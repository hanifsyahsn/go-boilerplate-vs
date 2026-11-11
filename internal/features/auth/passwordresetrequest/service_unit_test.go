package passwordresetrequest

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/passwordresetrequest/sqlc"
	authshared "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/shared"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/logging"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/mailer"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/ratelimit"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/apperror"
	"github.com/stretchr/testify/require"
)

// stubLimiter answers Take with a fixed decision / error and records keys.
type stubLimiter struct {
	allowed bool
	err     error
	keys    []string
}

func (l *stubLimiter) Take(_ context.Context, key string) (ratelimit.Decision, error) {
	l.keys = append(l.keys, key)
	return ratelimit.Decision{Allowed: l.allowed}, l.err
}
func (*stubLimiter) Reset(context.Context, string) error  { return nil }
func (*stubLimiter) Return(context.Context, string) error { return nil }

func TestService_Request_U(t *testing.T) {
	active := sqlc.GetUserForPasswordResetRow{ID: 7, Email: "Jane@example.com", IsActive: true}

	testCases := []struct {
		name      string
		limiter   *stubLimiter
		stub      func(q *sqlc.MockQuerier, tq *MockQuerier)
		wantCode  apperror.Code
		wantEmail bool
		wantLog   string // a log record with this message
	}{
		{
			name:    "an active account: a reset token issued (earlier ones spent) and one email to its address with the link",
			limiter: &stubLimiter{allowed: true},
			stub: func(q *sqlc.MockQuerier, tq *MockQuerier) {
				q.EXPECT().GetUserForPasswordReset(gomock.Any(), "Jane@Example.com").Return(active, nil) // trimmed; SQL ignores case
				tq.EXPECT().SpendUnusedAuthTokens(gomock.Any(), gomock.Any()).Return(nil)
				tq.EXPECT().InsertAuthToken(gomock.Any(), gomock.Any()).Return(nil)
			},
			wantEmail: true,
		},
		{
			name:    "an unknown email: nothing sent, the same answer",
			limiter: &stubLimiter{allowed: true},
			stub: func(q *sqlc.MockQuerier, _ *MockQuerier) {
				q.EXPECT().GetUserForPasswordReset(gomock.Any(), gomock.Any()).Return(sqlc.GetUserForPasswordResetRow{}, sql.ErrNoRows)
			},
		},
		{
			name:    "an inactive account: nothing sent, the same answer",
			limiter: &stubLimiter{allowed: true},
			stub: func(q *sqlc.MockQuerier, _ *MockQuerier) {
				q.EXPECT().GetUserForPasswordReset(gomock.Any(), gomock.Any()).
					Return(sqlc.GetUserForPasswordResetRow{ID: 7, Email: "jane@example.com"}, nil)
			},
		},
		{
			name:    "the recipient's limit reached: nothing looked up or sent, a warning without the address",
			limiter: &stubLimiter{allowed: false},
			stub:    func(*sqlc.MockQuerier, *MockQuerier) {},
			wantLog: "password reset: recipient limit reached",
		},
		{
			name:    "a limiter error: nothing sent (never an uncounted email), logged, the same answer",
			limiter: &stubLimiter{err: errors.New("store down")},
			stub:    func(*sqlc.MockQuerier, *MockQuerier) {},
			wantLog: "password reset: recipient limit failed",
		},
		{
			name:    "a lookup error is a 500",
			limiter: &stubLimiter{allowed: true},
			stub: func(q *sqlc.MockQuerier, _ *MockQuerier) {
				q.EXPECT().GetUserForPasswordReset(gomock.Any(), gomock.Any()).Return(sqlc.GetUserForPasswordResetRow{}, sql.ErrConnDone)
			},
			wantCode: apperror.CodeInternal,
		},
		{
			name:    "an issuing error is a 500 and nothing is sent",
			limiter: &stubLimiter{allowed: true},
			stub: func(q *sqlc.MockQuerier, tq *MockQuerier) {
				q.EXPECT().GetUserForPasswordReset(gomock.Any(), gomock.Any()).Return(active, nil)
				tq.EXPECT().SpendUnusedAuthTokens(gomock.Any(), gomock.Any()).Return(sql.ErrConnDone)
			},
			wantCode: apperror.CodeInternal,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			l, err := logging.New(&buf, logging.FormatJSON, "debug")
			require.NoError(t, err)
			prev := slog.Default()
			slog.SetDefault(l)
			defer slog.SetDefault(prev)

			ctrl := gomock.NewController(t)
			q := sqlc.NewMockQuerier(ctrl)
			tq := NewMockQuerier(ctrl)
			tc.stub(q, tq)
			store := NewMockStoreI(ctrl)
			store.EXPECT().RunInTx(gomock.Any(), gomock.Any()).DoAndReturn(
				func(ctx context.Context, fn func(Querier) error) error { return fn(tq) }).AnyTimes()
			fake := &mailer.Fake{}
			emailer := authshared.Emailer{Mailer: fake, AppBaseURL: "https://app.example.com",
				Go: func(ctx context.Context, fn func(context.Context)) { fn(ctx) }}

			svc := NewService(q, store, tc.limiter, emailer)
			svc.(*service).now = func() time.Time { return time.Date(2026, 11, 11, 9, 0, 0, 0, time.UTC) }
			err = svc.Request(context.Background(), Request{Email: "  Jane@Example.com "})

			require.Equal(t, []string{"reset:jane@example.com"}, tc.limiter.keys, "the limit is taken first, per trimmed lowercase email")
			if !tc.wantCode.IsZero() {
				var appErr *apperror.AppError
				require.ErrorAs(t, err, &appErr)
				require.Equal(t, tc.wantCode, appErr.Code)
				require.Empty(t, fake.Sent())
				return
			}
			require.NoError(t, err)
			if !tc.wantEmail {
				require.Empty(t, fake.Sent(), "nothing sent")
			} else {
				sent := fake.Sent()
				require.Len(t, sent, 1)
				require.Equal(t, "Jane@example.com", sent[0].To)
				require.Equal(t, "Reset your password", sent[0].Subject)
				require.Contains(t, sent[0].Text, "https://app.example.com/reset-password?token=")
			}
			if tc.wantLog != "" {
				var rec map[string]any
				require.NoError(t, json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &rec), buf.String())
				require.Equal(t, tc.wantLog, rec["msg"])
				require.NotContains(t, buf.String(), "example.com", "the address is never logged")
			}
		})
	}
}
