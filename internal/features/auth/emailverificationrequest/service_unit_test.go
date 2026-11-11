package emailverificationrequest

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/emailverificationrequest/sqlc"
	authshared "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/shared"
	authsharedsqlc "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/shared/sqlc"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/mailer"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/ratelimit"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/apperror"
	"github.com/stretchr/testify/require"
)

type errLimiter struct{}

func (errLimiter) Take(context.Context, string) (ratelimit.Decision, error) {
	return ratelimit.Decision{}, errors.New("store down")
}
func (errLimiter) Reset(context.Context, string) error  { return nil }
func (errLimiter) Return(context.Context, string) error { return nil }

func TestService_Request_U(t *testing.T) {
	unverified := sqlc.GetUserForVerificationRow{Email: "jane@example.com"}

	testCases := []struct {
		name     string
		limiter  ratelimit.Limiter
		stub     func(q *sqlc.MockQuerier, tq *MockQuerier)
		calls    int // Request calls in a row; the last one is checked
		wantCode apperror.Code
		wantMsg  string
		wantSent int
	}{
		{
			name: "an unverified email: a new 24 h link issued (earlier ones spent) and emailed",
			stub: func(q *sqlc.MockQuerier, tq *MockQuerier) {
				q.EXPECT().GetUserForVerification(gomock.Any(), int64(7)).Return(unverified, nil)
				tq.EXPECT().SpendUnusedAuthTokens(gomock.Any(), authsharedsqlc.SpendUnusedAuthTokensParams{UserID: 7, Purpose: authshared.PurposeVerifyEmail}).Return(nil)
				tq.EXPECT().InsertAuthToken(gomock.Any(), gomock.Any()).Return(nil)
			},
			calls: 1, wantSent: 1,
		},
		{
			name: "a verified email: 409 email_already_verified, nothing sent",
			stub: func(q *sqlc.MockQuerier, _ *MockQuerier) {
				q.EXPECT().GetUserForVerification(gomock.Any(), int64(7)).Return(sqlc.GetUserForVerificationRow{EmailVerified: true}, nil)
			},
			calls: 1, wantCode: CodeEmailAlreadyVerified, wantMsg: "Your email is already verified.",
		},
		{
			name: "the 4th link in an hour is a 429",
			stub: func(q *sqlc.MockQuerier, tq *MockQuerier) {
				q.EXPECT().GetUserForVerification(gomock.Any(), int64(7)).Return(unverified, nil).Times(4)
				tq.EXPECT().SpendUnusedAuthTokens(gomock.Any(), gomock.Any()).Return(nil).Times(3)
				tq.EXPECT().InsertAuthToken(gomock.Any(), gomock.Any()).Return(nil).Times(3)
			},
			calls: 4, wantCode: apperror.CodeTooManyRequests, wantMsg: "Too many attempts. Please wait a few minutes and try again.", wantSent: 3,
		},
		{
			name: "a limiter error is a 500", limiter: errLimiter{},
			stub: func(q *sqlc.MockQuerier, _ *MockQuerier) {
				q.EXPECT().GetUserForVerification(gomock.Any(), int64(7)).Return(unverified, nil)
			},
			calls: 1, wantCode: apperror.CodeInternal,
		},
		{
			name: "a read error is a 500",
			stub: func(q *sqlc.MockQuerier, _ *MockQuerier) {
				q.EXPECT().GetUserForVerification(gomock.Any(), int64(7)).Return(sqlc.GetUserForVerificationRow{}, sql.ErrConnDone)
			},
			calls: 1, wantCode: apperror.CodeInternal,
		},
		{
			name: "an issuing error is a 500 and nothing is sent",
			stub: func(q *sqlc.MockQuerier, tq *MockQuerier) {
				q.EXPECT().GetUserForVerification(gomock.Any(), int64(7)).Return(unverified, nil)
				tq.EXPECT().SpendUnusedAuthTokens(gomock.Any(), gomock.Any()).Return(sql.ErrConnDone)
			},
			calls: 1, wantCode: apperror.CodeInternal,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			q := sqlc.NewMockQuerier(ctrl)
			tq := NewMockQuerier(ctrl)
			tc.stub(q, tq)
			store := NewMockStoreI(ctrl)
			store.EXPECT().RunInTx(gomock.Any(), gomock.Any()).DoAndReturn(
				func(ctx context.Context, fn func(Querier) error) error { return fn(tq) }).AnyTimes()
			limiter := tc.limiter
			if limiter == nil {
				limiter = ratelimit.NewMemory(ratelimit.Rate{Burst: 3, Per: time.Hour})
			}
			fake := &mailer.Fake{}
			svc := NewService(q, store, limiter, authshared.Emailer{Mailer: fake, AppBaseURL: "https://app.example.com",
				Go: func(ctx context.Context, fn func(context.Context)) { fn(ctx) }})

			var err error
			for range tc.calls {
				err = svc.Request(context.Background(), 7)
			}
			require.Len(t, fake.Sent(), tc.wantSent)
			if !tc.wantCode.IsZero() {
				var appErr *apperror.AppError
				require.ErrorAs(t, err, &appErr)
				require.Equal(t, tc.wantCode, appErr.Code)
				if tc.wantMsg != "" {
					require.Equal(t, tc.wantMsg, appErr.Message)
				}
				return
			}
			require.NoError(t, err)
			require.Equal(t, "Verify your email", fake.Sent()[0].Subject)
			require.Equal(t, "jane@example.com", fake.Sent()[0].To)
			require.Contains(t, fake.Sent()[0].Text, "https://app.example.com/verify-email?token=")
		})
	}
}
