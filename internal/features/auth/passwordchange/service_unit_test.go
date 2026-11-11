package passwordchange

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/passwordchange/sqlc"
	authshared "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/shared"
	authsharedsqlc "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/shared/sqlc"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/mailer"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/ratelimit"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/apperror"
	"github.com/stretchr/testify/require"
)

// brokenLimiter is a failure limiter whose store is down (Take) or whose
// give-back fails (Reset).
type brokenLimiter struct{ takeErr, resetErr error }

func (l brokenLimiter) Take(context.Context, string) (ratelimit.Decision, error) {
	return ratelimit.Decision{Allowed: true}, l.takeErr
}
func (l brokenLimiter) Reset(context.Context, string) error { return l.resetErr }
func (brokenLimiter) Return(context.Context, string) error  { return nil }

func TestService_Change_U(t *testing.T) {
	current := uuid.New()
	right := Request{CurrentPassword: "Passw0rd!", Password: "N3w-Passw0rd"}
	wrong := Request{CurrentPassword: "nope", Password: "N3w-Passw0rd"}

	testCases := []struct {
		name        string
		limiter     ratelimit.Limiter
		req         Request
		stub        func(q *sqlc.MockQuerier, tq *MockQuerier)
		hashErr     error
		wantCode    apperror.Code
		wantMsg     string
		wantPointer string
		wantNotice  bool
	}{
		{
			name: "the right current password: the new one set and every other session ended, the caller's kept, a notice sent",
			req:  right,
			stub: func(q *sqlc.MockQuerier, tq *MockQuerier) {
				q.EXPECT().GetUserPassword(gomock.Any(), int64(7)).Return("hash-of:Passw0rd!", nil)
				gomock.InOrder(
					tq.EXPECT().SetPassword(gomock.Any(), sqlc.SetPasswordParams{ID: 7, PasswordHash: "hash-of:N3w-Passw0rd"}).
						Return("jane@example.com", nil),
					tq.EXPECT().RevokeUserSessions(gomock.Any(), authsharedsqlc.RevokeUserSessionsParams{
						UserID: 7, Reason: authshared.RevokeReasonPasswordChanged, KeepFamilyID: uuid.NullUUID{UUID: current, Valid: true},
					}).Return(int64(2), nil),
				)
			},
			wantNotice: true,
		},
		{
			name: "a wrong current password: 422 wrong_password on current_password, nothing changed",
			req:  wrong,
			stub: func(q *sqlc.MockQuerier, _ *MockQuerier) {
				q.EXPECT().GetUserPassword(gomock.Any(), int64(7)).Return("hash-of:Passw0rd!", nil)
			},
			wantCode: CodeWrongPassword, wantMsg: "The current password is incorrect.", wantPointer: "/data/attributes/current_password",
		},
		{
			name: "a failed refill of the failure count is only logged", req: right, limiter: brokenLimiter{resetErr: errors.New("down")},
			stub: func(q *sqlc.MockQuerier, tq *MockQuerier) {
				q.EXPECT().GetUserPassword(gomock.Any(), gomock.Any()).Return("hash-of:Passw0rd!", nil)
				tq.EXPECT().SetPassword(gomock.Any(), gomock.Any()).Return("jane@example.com", nil)
				tq.EXPECT().RevokeUserSessions(gomock.Any(), gomock.Any()).Return(int64(0), nil)
			},
			wantNotice: true,
		},
		{
			name: "a limiter error is a 500 before the password is read", req: right, limiter: brokenLimiter{takeErr: errors.New("down")},
			stub: func(*sqlc.MockQuerier, *MockQuerier) {}, wantCode: apperror.CodeInternal,
		},
		{
			name: "a read error is a 500", req: right,
			stub: func(q *sqlc.MockQuerier, _ *MockQuerier) {
				q.EXPECT().GetUserPassword(gomock.Any(), gomock.Any()).Return("", sql.ErrConnDone)
			},
			wantCode: apperror.CodeInternal,
		},
		{
			name: "a hashing error is a 500 and nothing is written", req: right, hashErr: errors.New("bcrypt"),
			stub: func(q *sqlc.MockQuerier, _ *MockQuerier) {
				q.EXPECT().GetUserPassword(gomock.Any(), gomock.Any()).Return("hash-of:Passw0rd!", nil)
			},
			wantCode: apperror.CodeInternal,
		},
		{
			name: "a write error is a 500", req: right,
			stub: func(q *sqlc.MockQuerier, tq *MockQuerier) {
				q.EXPECT().GetUserPassword(gomock.Any(), gomock.Any()).Return("hash-of:Passw0rd!", nil)
				tq.EXPECT().SetPassword(gomock.Any(), gomock.Any()).Return("", sql.ErrConnDone)
			},
			wantCode: apperror.CodeInternal,
		},
		{
			name: "a session revoke error is a 500 (the new password rolls back with it)", req: right,
			stub: func(q *sqlc.MockQuerier, tq *MockQuerier) {
				q.EXPECT().GetUserPassword(gomock.Any(), gomock.Any()).Return("hash-of:Passw0rd!", nil)
				tq.EXPECT().SetPassword(gomock.Any(), gomock.Any()).Return("jane@example.com", nil)
				tq.EXPECT().RevokeUserSessions(gomock.Any(), gomock.Any()).Return(int64(0), sql.ErrConnDone)
			},
			wantCode: apperror.CodeInternal,
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
				limiter = ratelimit.NewMemory(ratelimit.Rate{Burst: 5, Per: 15 * time.Minute})
			}
			fake := &mailer.Fake{}
			svc := NewService(q, store, limiter, authshared.Emailer{Mailer: fake, Go: func(ctx context.Context, fn func(context.Context)) { fn(ctx) }})
			svc.(*service).check = func(hash, plain string) bool { return hash == "hash-of:"+plain }
			svc.(*service).hash = func(plain string) (string, error) { return "hash-of:" + plain, tc.hashErr }

			err := svc.Change(context.Background(), 7, current, tc.req)
			if !tc.wantCode.IsZero() {
				var appErr *apperror.AppError
				require.ErrorAs(t, err, &appErr)
				require.Equal(t, tc.wantCode, appErr.Code)
				if tc.wantMsg != "" {
					require.Equal(t, tc.wantMsg, appErr.Message)
					require.Equal(t, tc.wantPointer, appErr.Source.Pointer)
				}
				require.Empty(t, fake.Sent())
				return
			}
			require.NoError(t, err)
			require.Equal(t, []mailer.Message{authshared.PasswordChangedEmail("jane@example.com")}, fake.Sent())
		})
	}
}

// TestService_Change_FailureLimit_U: wrong current passwords are capped per
// user; past the cap even the right one is a 429 until a token comes back;
// the right one refills the count.
func TestService_Change_FailureLimit_U(t *testing.T) {
	right := Request{CurrentPassword: "Passw0rd!", Password: "N3w-Passw0rd"}
	wrong := Request{CurrentPassword: "nope", Password: "N3w-Passw0rd"}

	testCases := []struct {
		name string
		// attempts run in order; want is each one's code (zero = success).
		attempts []Request
		want     []apperror.Code
	}{
		{name: "the 6th attempt in 15 min after 5 wrong ones is a 429, even with the right password",
			attempts: []Request{wrong, wrong, wrong, wrong, wrong, right},
			want: []apperror.Code{CodeWrongPassword, CodeWrongPassword, CodeWrongPassword, CodeWrongPassword, CodeWrongPassword,
				apperror.CodeTooManyRequests}},
		{name: "the right password refills the count",
			attempts: []Request{wrong, wrong, wrong, wrong, right, wrong, wrong, wrong, wrong, wrong},
			want: []apperror.Code{CodeWrongPassword, CodeWrongPassword, CodeWrongPassword, CodeWrongPassword, {},
				CodeWrongPassword, CodeWrongPassword, CodeWrongPassword, CodeWrongPassword, CodeWrongPassword}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			q := sqlc.NewMockQuerier(ctrl)
			q.EXPECT().GetUserPassword(gomock.Any(), gomock.Any()).Return("hash-of:Passw0rd!", nil).AnyTimes()
			tq := NewMockQuerier(ctrl)
			tq.EXPECT().SetPassword(gomock.Any(), gomock.Any()).Return("jane@example.com", nil).AnyTimes()
			tq.EXPECT().RevokeUserSessions(gomock.Any(), gomock.Any()).Return(int64(0), nil).AnyTimes()
			store := NewMockStoreI(ctrl)
			store.EXPECT().RunInTx(gomock.Any(), gomock.Any()).DoAndReturn(
				func(ctx context.Context, fn func(Querier) error) error { return fn(tq) }).AnyTimes()
			svc := NewService(q, store, ratelimit.NewMemory(ratelimit.Rate{Burst: 5, Per: 15 * time.Minute}),
				authshared.Emailer{Mailer: &mailer.Fake{}, Go: func(ctx context.Context, fn func(context.Context)) { fn(ctx) }})
			svc.(*service).check = func(hash, plain string) bool { return hash == "hash-of:"+plain }
			svc.(*service).hash = func(plain string) (string, error) { return "hash-of:" + plain, nil }

			for i, req := range tc.attempts {
				err := svc.Change(context.Background(), 7, uuid.New(), req)
				if tc.want[i].IsZero() {
					require.NoError(t, err, "attempt %d", i+1)
					continue
				}
				var appErr *apperror.AppError
				require.ErrorAs(t, err, &appErr, "attempt %d", i+1)
				require.Equal(t, tc.want[i], appErr.Code, "attempt %d", i+1)
			}
		})
	}
}

// TestService_Change_ParallelGuesses_U: wrong guesses sent at once can't get
// past the burst — each takes its token before the password is compared.
func TestService_Change_ParallelGuesses_U(t *testing.T) {
	testCases := []struct {
		name         string
		guesses      int
		wantCompared int
	}{
		{name: "20 parallel guesses with a burst of 5: 5 compared, 15 refused", guesses: 20, wantCompared: 5},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			q := sqlc.NewMockQuerier(ctrl)
			q.EXPECT().GetUserPassword(gomock.Any(), gomock.Any()).Return("hash-of:Passw0rd!", nil).AnyTimes()
			svc := NewService(q, NewMockStoreI(ctrl), ratelimit.NewMemory(ratelimit.Rate{Burst: 5, Per: 15 * time.Minute}),
				authshared.Emailer{Mailer: &mailer.Fake{}, Go: func(ctx context.Context, fn func(context.Context)) { fn(ctx) }})
			var mu sync.Mutex
			compared := 0
			svc.(*service).check = func(string, string) bool {
				mu.Lock()
				compared++
				mu.Unlock()
				time.Sleep(10 * time.Millisecond) // a slow compare, like bcrypt
				return false
			}

			var wg sync.WaitGroup
			refused := make(chan struct{}, tc.guesses)
			for range tc.guesses {
				wg.Add(1)
				go func() {
					defer wg.Done()
					err := svc.Change(context.Background(), 7, uuid.New(), Request{CurrentPassword: "guess", Password: "N3w-Passw0rd"})
					var appErr *apperror.AppError
					if errors.As(err, &appErr) && appErr.Code == apperror.CodeTooManyRequests {
						refused <- struct{}{}
					}
				}()
			}
			wg.Wait()
			close(refused)

			require.Equal(t, tc.wantCompared, compared)
			require.Len(t, refused, tc.guesses-tc.wantCompared)
		})
	}
}
