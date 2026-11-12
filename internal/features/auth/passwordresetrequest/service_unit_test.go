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
	authsharedsqlc "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/shared/sqlc"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/logging"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/mailer"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/ratelimit"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/requestid"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/testhelper"
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
			name:    "a lookup error: nothing sent, logged without the address (the caller already had its 202)",
			limiter: &stubLimiter{allowed: true},
			stub: func(q *sqlc.MockQuerier, _ *MockQuerier) {
				q.EXPECT().GetUserForPasswordReset(gomock.Any(), gomock.Any()).Return(sqlc.GetUserForPasswordResetRow{}, sql.ErrConnDone)
			},
			wantLog: "password reset: lookup failed",
		},
		{
			name:    "an issuing error: nothing sent, logged with the recipient's id, never the address",
			limiter: &stubLimiter{allowed: true},
			stub: func(q *sqlc.MockQuerier, tq *MockQuerier) {
				q.EXPECT().GetUserForPasswordReset(gomock.Any(), gomock.Any()).Return(active, nil)
				tq.EXPECT().SpendUnusedAuthTokens(gomock.Any(), gomock.Any()).Return(sql.ErrConnDone)
			},
			wantLog: "password reset: issue token failed",
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

			svc := NewService(q, testhelper.FoldingQuerier(ctrl), store, tc.limiter, emailer)
			svc.(*service).now = func() time.Time { return time.Date(2026, 11, 11, 9, 0, 0, 0, time.UTC) }
			svc.Request(context.Background(), Request{Email: "  Jane@Example.com "})

			require.Equal(t, []string{"reset:jane@example.com"}, tc.limiter.keys, "the limit is taken first, per trimmed lowercase email")
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

// TestService_Request_AnswersBeforeWork_U: Request returns before the limit,
// the lookup or the token write run, so a known and an unknown email take the
// same time to answer. The work runs later, on a context detached from the
// request (it keeps the request id, outlives its cancellation).
func TestService_Request_AnswersBeforeWork_U(t *testing.T) {
	active := sqlc.GetUserForPasswordResetRow{ID: 7, Email: "jane@example.com", IsActive: true}

	testCases := []struct {
		name      string
		stub      func(q *sqlc.MockQuerier, tq *MockQuerier)
		wantEmail bool
	}{
		{
			name: "a known email: nothing is touched before the answer; the link goes out afterwards",
			stub: func(q *sqlc.MockQuerier, tq *MockQuerier) {
				q.EXPECT().GetUserForPasswordReset(gomock.Any(), "jane@example.com").Return(active, nil)
				tq.EXPECT().SpendUnusedAuthTokens(gomock.Any(), gomock.Any()).Return(nil)
				tq.EXPECT().InsertAuthToken(gomock.Any(), gomock.Any()).Return(nil)
			},
			wantEmail: true,
		},
		{
			name: "an unknown email: the same: nothing touched before the answer, nothing sent afterwards",
			stub: func(q *sqlc.MockQuerier, _ *MockQuerier) {
				q.EXPECT().GetUserForPasswordReset(gomock.Any(), "jane@example.com").Return(sqlc.GetUserForPasswordResetRow{}, sql.ErrNoRows)
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			q := sqlc.NewMockQuerier(ctrl)
			tq := NewMockQuerier(ctrl)
			store := NewMockStoreI(ctrl)
			limiter := &stubLimiter{allowed: true}
			fake := &mailer.Fake{}

			// Go holds the work instead of running it, like a busy background group.
			var held []func()
			var workCtx context.Context
			emailer := authshared.Emailer{Mailer: fake, AppBaseURL: "https://app.example.com",
				Go: func(ctx context.Context, fn func(context.Context)) {
					if workCtx == nil {
						workCtx = ctx
					}
					held = append(held, func() { fn(ctx) })
				}}

			reqCtx, cancel := context.WithCancel(requestid.With(context.Background(), "req-1"))
			NewService(q, testhelper.FoldingQuerier(ctrl), store, limiter, emailer).Request(reqCtx, Request{Email: "jane@example.com"})
			cancel() // the request is over

			// Answered: no limit taken, no lookup, no write, nothing sent (the
			// mocks have no expectations yet, so any call would fail the test).
			require.Empty(t, limiter.keys)
			require.Empty(t, fake.Sent())
			require.Len(t, held, 1, "the work was handed to the background group")
			require.Equal(t, "req-1", requestid.FromContext(workCtx), "its logs keep the request id")
			require.NoError(t, workCtx.Err(), "it outlives the request's cancellation")

			tc.stub(q, tq)
			store.EXPECT().RunInTx(gomock.Any(), gomock.Any()).DoAndReturn(
				func(ctx context.Context, fn func(Querier) error) error { return fn(tq) }).AnyTimes()
			for len(held) > 0 { // the work, then the send it starts
				next := held[0]
				held = held[1:]
				next()
			}
			require.Equal(t, []string{"reset:jane@example.com"}, limiter.keys)
			require.Len(t, fake.Sent(), map[bool]int{true: 1, false: 0}[tc.wantEmail])
		})
	}
}

func TestService_Request_FoldEmail_U(t *testing.T) {
	testCases := []struct {
		name    string
		folded  string
		foldErr error
		wantKey []string
		wantLog string
	}{
		{name: "the recipient key is the email as Postgres folds it (the lookup's fold)",
			folded: "ƛjane@example.com", wantKey: []string{"reset:ƛjane@example.com"}},
		{name: "a fold error: nothing counted, looked up or sent, logged without the address",
			foldErr: sql.ErrConnDone, wantLog: "password reset: fold email failed"},
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
			sq := authsharedsqlc.NewMockQuerier(ctrl)
			sq.EXPECT().FoldEmail(gomock.Any(), "ꟜJane@example.com").Return(tc.folded, tc.foldErr)
			if tc.foldErr == nil {
				q.EXPECT().GetUserForPasswordReset(gomock.Any(), gomock.Any()).Return(sqlc.GetUserForPasswordResetRow{}, sql.ErrNoRows)
			}
			limiter := &stubLimiter{allowed: true}
			fake := &mailer.Fake{}
			emailer := authshared.Emailer{Mailer: fake, Go: func(ctx context.Context, fn func(context.Context)) { fn(ctx) }}

			NewService(q, sq, NewMockStoreI(ctrl), limiter, emailer).Request(context.Background(), Request{Email: " ꟜJane@example.com "})

			require.Equal(t, tc.wantKey, limiter.keys)
			require.Empty(t, fake.Sent())
			if tc.wantLog != "" {
				require.Contains(t, buf.String(), `"msg":"`+tc.wantLog+`"`)
				require.NotContains(t, buf.String(), "example.com")
			}
		})
	}
}
