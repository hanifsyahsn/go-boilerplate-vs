package login

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/login/sqlc"
	authshared "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/shared"
	authsharedsqlc "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/shared/sqlc"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/jwttoken"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/logging"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/ratelimit"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/apperror"
	internalshareddto "github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/dto"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/enum"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/password"
	"github.com/stretchr/testify/require"
)

func TestService_Login_U(t *testing.T) {
	hash, err := password.Hash("Passw0rd!")
	require.NoError(t, err)

	active := sqlc.GetUserForLoginRow{ID: 1, Name: "Jane", Email: "jane@example.com", PasswordHash: hash, Role: "admin", IsActive: true}
	inactive := active
	inactive.IsActive = false
	refreshExp := time.Now().Add(24 * time.Hour)
	pair := jwttoken.Pair{AccessToken: "acc", RefreshToken: "ref", RefreshTokenID: uuid.New(), RefreshTokenExpiresAt: refreshExp}
	familyID := uuid.New()
	now := time.Now()
	insert := authsharedsqlc.InsertRefreshTokenParams{Jti: pair.RefreshTokenID, UserID: 1, FamilyID: familyID, ExpiresAt: refreshExp}
	capped := insert
	capped.SessionExpiresAt = sql.NullTime{Time: now.Add(12 * time.Hour), Valid: true}

	testCases := []struct {
		name     string
		req      Request
		stub     func(q *sqlc.MockQuerier, sq *authsharedsqlc.MockQuerier, m *jwttoken.MockM)
		want     Result
		wantCode apperror.Code
		wantMsg  string
	}{
		{
			name: "right password, trusted device: a token pair with the DB role, the refresh token recorded in a new family with no cap",
			req:  Request{Email: " jane@example.com ", Password: "Passw0rd!", TrustDevice: true},
			stub: func(q *sqlc.MockQuerier, sq *authsharedsqlc.MockQuerier, m *jwttoken.MockM) {
				q.EXPECT().GetUserForLogin(gomock.Any(), "jane@example.com").Return(active, nil)
				m.EXPECT().CreateToken(internalshareddto.User{ID: 1, Name: "Jane", Email: "jane@example.com", Role: enum.RoleAdmin},
					15*time.Minute, 24*time.Hour).Return(pair, nil)
				sq.EXPECT().InsertRefreshToken(gomock.Any(), insert).Return(nil)
			},
			want: Result{User: authshared.UserResource{ID: "1", Name: "Jane", Email: "jane@example.com", Role: "admin"}, Tokens: pair},
		},
		{
			name: "untrusted device: the family capped at the untrusted session duration, the refresh token no longer than it",
			req:  Request{Email: "jane@example.com", Password: "Passw0rd!"},
			stub: func(q *sqlc.MockQuerier, sq *authsharedsqlc.MockQuerier, m *jwttoken.MockM) {
				q.EXPECT().GetUserForLogin(gomock.Any(), "jane@example.com").Return(active, nil)
				m.EXPECT().CreateToken(gomock.Any(), 15*time.Minute, 12*time.Hour).Return(pair, nil)
				sq.EXPECT().InsertRefreshToken(gomock.Any(), capped).Return(nil)
			},
			want: Result{User: authshared.UserResource{ID: "1", Name: "Jane", Email: "jane@example.com", Role: "admin"}, Tokens: pair},
		},
		{
			name: "unknown email",
			req:  Request{Email: "nobody@example.com", Password: "x"},
			stub: func(q *sqlc.MockQuerier, sq *authsharedsqlc.MockQuerier, m *jwttoken.MockM) {
				q.EXPECT().GetUserForLogin(gomock.Any(), gomock.Any()).Return(sqlc.GetUserForLoginRow{}, sql.ErrNoRows)
			},
			wantCode: apperror.CodeUnauthorized, wantMsg: invalidCredentialsMessage,
		},
		{
			name: "wrong password: the same message",
			req:  Request{Email: "jane@example.com", Password: "wrong"},
			stub: func(q *sqlc.MockQuerier, sq *authsharedsqlc.MockQuerier, m *jwttoken.MockM) {
				q.EXPECT().GetUserForLogin(gomock.Any(), gomock.Any()).Return(active, nil)
			},
			wantCode: apperror.CodeUnauthorized, wantMsg: invalidCredentialsMessage,
		},
		{
			name: "a deactivated account",
			req:  Request{Email: "jane@example.com", Password: "Passw0rd!"},
			stub: func(q *sqlc.MockQuerier, sq *authsharedsqlc.MockQuerier, m *jwttoken.MockM) {
				q.EXPECT().GetUserForLogin(gomock.Any(), gomock.Any()).Return(inactive, nil)
			},
			wantCode: apperror.CodeUnauthorized, wantMsg: "Your account is no longer active.",
		},
		{
			name: "a read error is a 500",
			req:  Request{Email: "jane@example.com", Password: "x"},
			stub: func(q *sqlc.MockQuerier, sq *authsharedsqlc.MockQuerier, m *jwttoken.MockM) {
				q.EXPECT().GetUserForLogin(gomock.Any(), gomock.Any()).Return(sqlc.GetUserForLoginRow{}, sql.ErrConnDone)
			},
			wantCode: apperror.CodeInternal,
		},
		{
			name: "a signing error is a 500",
			req:  Request{Email: "jane@example.com", Password: "Passw0rd!"},
			stub: func(q *sqlc.MockQuerier, sq *authsharedsqlc.MockQuerier, m *jwttoken.MockM) {
				q.EXPECT().GetUserForLogin(gomock.Any(), gomock.Any()).Return(active, nil)
				m.EXPECT().CreateToken(gomock.Any(), gomock.Any(), gomock.Any()).Return(jwttoken.Pair{}, errors.New("sign"))
			},
			wantCode: apperror.CodeInternal,
		},
		{
			name: "a refresh-token insert error is a 500",
			req:  Request{Email: "jane@example.com", Password: "Passw0rd!"},
			stub: func(q *sqlc.MockQuerier, sq *authsharedsqlc.MockQuerier, m *jwttoken.MockM) {
				q.EXPECT().GetUserForLogin(gomock.Any(), gomock.Any()).Return(active, nil)
				m.EXPECT().CreateToken(gomock.Any(), gomock.Any(), gomock.Any()).Return(pair, nil)
				sq.EXPECT().InsertRefreshToken(gomock.Any(), gomock.Any()).Return(sql.ErrConnDone)
			},
			wantCode: apperror.CodeInternal,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			q := sqlc.NewMockQuerier(ctrl)
			sq := authsharedsqlc.NewMockQuerier(ctrl)
			m := jwttoken.NewMockM(ctrl)
			tc.stub(q, sq, m)

			svc := NewService(q, sq, m, 15*time.Minute, 24*time.Hour, 12*time.Hour, ratelimit.NewMemory(ratelimit.Rate{Burst: 5, Per: time.Minute}))
			svc.(*service).newFamilyID = func() uuid.UUID { return familyID }
			svc.(*service).now = func() time.Time { return now }

			got, err := svc.Login(context.Background(), tc.req)
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
			require.Equal(t, tc.want, got)
		})
	}
}

// TestService_Login_FailureLimit_U pins the per-email cap on failed logins,
// against a limiter of 2 failures per window.
func TestService_Login_FailureLimit_U(t *testing.T) {
	hash, err := password.Hash("Passw0rd!")
	require.NoError(t, err)
	active := sqlc.GetUserForLoginRow{ID: 1, Name: "Jane", Email: "jane@example.com", PasswordHash: hash, Role: "user", IsActive: true}
	inactive := active
	inactive.IsActive = false

	// attempt is one login; user / readErr is what GetUserForLogin returns.
	// A 429 attempt sets neither: it must not reach the DB.
	type attempt struct {
		email, password string
		user            *sqlc.GetUserForLoginRow
		readErr         error
		wantCode        apperror.Code // zero = success
	}
	unknown := func(email string) attempt {
		return attempt{email: email, password: "x", readErr: sql.ErrNoRows, wantCode: apperror.CodeUnauthorized}
	}
	wrong := func(email string) attempt {
		return attempt{email: email, password: "wrong", user: &active, wantCode: apperror.CodeUnauthorized}
	}
	right := attempt{email: "jane@example.com", password: "Passw0rd!", user: &active}
	blocked := func(email string) attempt {
		return attempt{email: email, password: "Passw0rd!", wantCode: apperror.CodeTooManyRequests}
	}

	testCases := []struct {
		name     string
		attempts []attempt
	}{
		{name: "unknown emails count as failures; past the limit the email is a 429 without a DB read",
			attempts: []attempt{unknown("ghost@example.com"), unknown("ghost@example.com"), blocked("ghost@example.com")}},
		{name: "wrong passwords count; once the bucket is empty even the right password is a 429",
			attempts: []attempt{wrong("jane@example.com"), wrong("jane@example.com"), blocked("jane@example.com")}},
		{name: "a successful login clears the email's failures",
			attempts: []attempt{wrong("jane@example.com"), right, wrong("jane@example.com"), wrong("jane@example.com"),
				blocked("jane@example.com")}},
		{name: "a deactivated account with the right password is not a failure",
			attempts: []attempt{
				{email: "jane@example.com", password: "Passw0rd!", user: &inactive, wantCode: apperror.CodeUnauthorized},
				{email: "jane@example.com", password: "Passw0rd!", user: &inactive, wantCode: apperror.CodeUnauthorized},
				{email: "jane@example.com", password: "Passw0rd!", user: &inactive, wantCode: apperror.CodeUnauthorized},
			}},
		{name: "case and surrounding spaces don't give an email a fresh count",
			attempts: []attempt{wrong("Jane@Example.com"), wrong(" jane@example.com "), blocked("JANE@EXAMPLE.COM")}},
		{name: "another email keeps its own count",
			attempts: []attempt{wrong("jane@example.com"), wrong("jane@example.com"), unknown("ghost@example.com")}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			q := sqlc.NewMockQuerier(ctrl)
			sq := authsharedsqlc.NewMockQuerier(ctrl)
			m := jwttoken.NewMockM(ctrl)
			svc := NewService(q, sq, m, 15*time.Minute, 24*time.Hour, 12*time.Hour, ratelimit.NewMemory(ratelimit.Rate{Burst: 2, Per: time.Minute}))

			for i, a := range tc.attempts {
				if a.wantCode != apperror.CodeTooManyRequests {
					row := sqlc.GetUserForLoginRow{}
					if a.user != nil {
						row = *a.user
					}
					q.EXPECT().GetUserForLogin(gomock.Any(), gomock.Any()).Return(row, a.readErr)
				}
				if a.wantCode.IsZero() {
					m.EXPECT().CreateToken(gomock.Any(), gomock.Any(), gomock.Any()).Return(jwttoken.Pair{}, nil)
					sq.EXPECT().InsertRefreshToken(gomock.Any(), gomock.Any()).Return(nil)
				}

				_, err := svc.Login(context.Background(), Request{Email: a.email, Password: a.password})
				if a.wantCode.IsZero() {
					require.NoError(t, err, "attempt %d", i)
					continue
				}
				var appErr *apperror.AppError
				require.ErrorAs(t, err, &appErr, "attempt %d", i)
				require.Equal(t, a.wantCode, appErr.Code, "attempt %d", i)
				if a.wantCode == apperror.CodeTooManyRequests {
					require.Equal(t, tooManyFailuresMessage, appErr.Message)
				}
			}
		})
	}
}

// TestService_Login_TimingEqualized_U pins that every login with a known-format
// request spends one bcrypt comparison, an unknown email included (against a
// dummy hash via password.CheckOrWaste), so response time doesn't reveal which
// emails have an account.
func TestService_Login_TimingEqualized_U(t *testing.T) {
	user := sqlc.GetUserForLoginRow{ID: 1, Email: "jane@example.com", PasswordHash: "$2a$10$stored", IsActive: true}

	testCases := []struct {
		name      string
		row       sqlc.GetUserForLoginRow
		readErr   error
		wantCheck []string // the (hash, plain) the one comparison got
	}{
		{name: "an unknown email still runs one comparison, with no hash (CheckOrWaste spends it on the dummy)",
			readErr: sql.ErrNoRows, wantCheck: []string{"", "guess"}},
		{name: "a known email compares against its stored hash",
			row: user, wantCheck: []string{"$2a$10$stored", "guess"}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			q := sqlc.NewMockQuerier(ctrl)
			q.EXPECT().GetUserForLogin(gomock.Any(), gomock.Any()).Return(tc.row, tc.readErr)
			svc := NewService(q, authsharedsqlc.NewMockQuerier(ctrl), jwttoken.NewMockM(ctrl),
				15*time.Minute, 24*time.Hour, 12*time.Hour, ratelimit.NewMemory(ratelimit.Rate{Burst: 5, Per: time.Minute}))

			var calls [][]string
			svc.(*service).checkPassword = func(hash, plain string) bool {
				calls = append(calls, []string{hash, plain})
				return false
			}

			_, err := svc.Login(context.Background(), Request{Email: "jane@example.com", Password: "guess"})
			var appErr *apperror.AppError
			require.ErrorAs(t, err, &appErr)
			require.Equal(t, apperror.CodeUnauthorized, appErr.Code)
			require.Equal(t, invalidCredentialsMessage, appErr.Message, "same answer either way")
			require.Equal(t, [][]string{tc.wantCheck}, calls)
		})
	}
}

// stubLimiter answers Take with takeErr, or with a refusal when refuse is set,
// and records the calls.
type stubLimiter struct {
	takeErr, resetErr error
	refuse            bool
	calls             []string
}

func (l *stubLimiter) Take(_ context.Context, key string) (ratelimit.Decision, error) {
	l.calls = append(l.calls, "Take "+key)
	if l.refuse {
		return ratelimit.Decision{RetryAfter: time.Minute}, nil
	}
	return ratelimit.Decision{Allowed: l.takeErr == nil}, l.takeErr
}

func (l *stubLimiter) Reset(_ context.Context, key string) error {
	l.calls = append(l.calls, "Reset "+key)
	return l.resetErr
}

// TestService_Login_LimiterOutcomes_U pins what the failures limiter's answers
// do: a token is taken before the DB read; an error is a 500 and a refusal (an
// empty bucket, or a full bucket map refusing a new key) a 429, both before
// the DB read — never an uncounted guess; a failed refill after the right
// password still logs in, logged.
func TestService_Login_LimiterOutcomes_U(t *testing.T) {
	hash, err := password.Hash("Passw0rd!")
	require.NoError(t, err)
	active := sqlc.GetUserForLoginRow{ID: 1, Name: "Jane", Email: "jane@example.com", PasswordHash: hash, Role: "user", IsActive: true}
	storeDown := errors.New("store down")
	const key = "login:jane@example.com"

	testCases := []struct {
		name      string
		limiter   *stubLimiter
		password  string
		readsUser bool
		wantCode  apperror.Code // zero = success
		wantErr   error
		wantCalls []string
		wantLog   string
	}{
		{name: "a limiter that fails is a 500 before the DB read",
			limiter: &stubLimiter{takeErr: storeDown}, password: "wrong",
			wantCode: apperror.CodeInternal, wantErr: storeDown, wantCalls: []string{"Take " + key}},
		{name: "a limiter that refuses the key is a 429 before the DB read, the right password too",
			limiter: &stubLimiter{refuse: true}, password: "Passw0rd!",
			wantCode: apperror.CodeTooManyRequests, wantCalls: []string{"Take " + key}},
		{name: "a wrong password keeps the token it took",
			limiter: &stubLimiter{}, password: "wrong", readsUser: true,
			wantCode: apperror.CodeUnauthorized, wantCalls: []string{"Take " + key}},
		{name: "the right password gives the token back by refilling the bucket",
			limiter: &stubLimiter{}, password: "Passw0rd!", readsUser: true,
			wantCalls: []string{"Take " + key, "Reset " + key}},
		{name: "the right password still logs in when the bucket can't be refilled, and it's logged",
			limiter: &stubLimiter{resetErr: storeDown}, password: "Passw0rd!", readsUser: true,
			wantCalls: []string{"Take " + key, "Reset " + key},
			wantLog:   `"msg":"login: reset failure count failed"`},
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
			m := jwttoken.NewMockM(ctrl)
			if tc.readsUser {
				q.EXPECT().GetUserForLogin(gomock.Any(), gomock.Any()).Return(active, nil)
			}
			if tc.wantCode.IsZero() {
				m.EXPECT().CreateToken(gomock.Any(), gomock.Any(), gomock.Any()).Return(jwttoken.Pair{}, nil)
				sq.EXPECT().InsertRefreshToken(gomock.Any(), gomock.Any()).Return(nil)
			}
			svc := NewService(q, sq, m, 15*time.Minute, 24*time.Hour, 12*time.Hour, tc.limiter)

			_, err = svc.Login(context.Background(), Request{Email: "jane@example.com", Password: tc.password})
			require.Equal(t, tc.wantCalls, tc.limiter.calls)
			if tc.wantCode.IsZero() {
				require.NoError(t, err)
				require.Contains(t, buf.String(), tc.wantLog)
				return
			}
			var appErr *apperror.AppError
			require.ErrorAs(t, err, &appErr)
			require.Equal(t, tc.wantCode, appErr.Code)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
			}
		})
	}
}

// TestService_Login_ParallelGuesses_U pins that parallel wrong guesses for one
// email can't outrun the count: while every guess that got through is still
// in its (slow) bcrypt comparison, the rest are already 429s, so exactly the
// burst reach the comparison.
func TestService_Login_ParallelGuesses_U(t *testing.T) {
	active := sqlc.GetUserForLoginRow{ID: 1, Email: "jane@example.com", PasswordHash: "$2a$10$stored", IsActive: true}

	testCases := []struct {
		name         string
		burst        int
		parallel     int
		wantCompared int32
	}{
		{name: "20 parallel guesses against 2 tries: 2 reach bcrypt, 18 are 429s", burst: 2, parallel: 20, wantCompared: 2},
		{name: "3 parallel guesses against 5 tries: all 3 reach bcrypt", burst: 5, parallel: 3, wantCompared: 3},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			q := sqlc.NewMockQuerier(ctrl)
			q.EXPECT().GetUserForLogin(gomock.Any(), gomock.Any()).Return(active, nil).AnyTimes()
			svc := NewService(q, authsharedsqlc.NewMockQuerier(ctrl), jwttoken.NewMockM(ctrl),
				15*time.Minute, 24*time.Hour, 12*time.Hour,
				ratelimit.NewMemory(ratelimit.Rate{Burst: tc.burst, Per: time.Hour}))

			// Every comparison blocks until release: the guesses that got
			// through are all "in bcrypt" while the others decide.
			var compared, tooMany, unauthorized atomic.Int32
			release := make(chan struct{})
			svc.(*service).checkPassword = func(string, string) bool {
				compared.Add(1)
				<-release
				return false
			}

			var wg sync.WaitGroup
			for range tc.parallel {
				wg.Go(func() {
					_, err := svc.Login(context.Background(), Request{Email: "jane@example.com", Password: "guess"})
					var appErr *apperror.AppError
					if errors.As(err, &appErr) {
						switch appErr.Code {
						case apperror.CodeTooManyRequests:
							tooMany.Add(1)
						case apperror.CodeUnauthorized:
							unauthorized.Add(1)
						}
					}
				})
			}
			require.Eventually(t, func() bool { return compared.Load()+tooMany.Load() == int32(tc.parallel) },
				5*time.Second, time.Millisecond, "every guess is either in bcrypt or refused")
			close(release)
			wg.Wait()

			require.Equal(t, tc.wantCompared, compared.Load())
			require.Equal(t, tc.wantCompared, unauthorized.Load())
			require.Equal(t, int32(tc.parallel)-tc.wantCompared, tooMany.Load())
		})
	}
}
