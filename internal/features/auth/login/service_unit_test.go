package login

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
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
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/testhelper"
	"github.com/stretchr/testify/require"
)

// testClient is the caller of every login that isn't about the IP limit.
var testClient = Client{IP: "203.0.113.7", UserAgent: "test"}

// wideLimits never refuse in these tests (5 per email, 1000 per IP).
func wideLimits() Limiters {
	return Limiters{
		Email: ratelimit.NewMemory(ratelimit.Rate{Burst: 5, Per: time.Minute}),
		IP:    ratelimit.NewMemory(ratelimit.Rate{Burst: 1000, Per: time.Minute}),
	}
}

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
	insert := authsharedsqlc.StartSessionParams{Jti: pair.RefreshTokenID, UserID: 1, FamilyID: familyID, ExpiresAt: refreshExp,
		Trusted: true, UserAgent: "test", ClientIp: "203.0.113.7"}
	capped := insert
	capped.Trusted = false
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
			name: "right password, trusted device: a token pair with the DB role and the session as sid, a trusted session with the client's user agent and IP started with no cap",
			req:  Request{Email: " jane@example.com ", Password: "Passw0rd!", TrustDevice: true},
			stub: func(q *sqlc.MockQuerier, sq *authsharedsqlc.MockQuerier, m *jwttoken.MockM) {
				q.EXPECT().GetUserForLogin(gomock.Any(), "jane@example.com").Return(active, nil)
				m.EXPECT().CreateToken(internalshareddto.User{ID: 1, Name: "Jane", Email: "jane@example.com", Role: enum.RoleAdmin},
					familyID, 15*time.Minute, 24*time.Hour).Return(pair, nil)
				sq.EXPECT().StartSession(gomock.Any(), insert).Return(nil)
			},
			want: Result{User: authshared.UserResource{ID: "1", Name: "Jane", Email: "jane@example.com", Role: "admin"}, Tokens: pair},
		},
		{
			name: "untrusted device: an untrusted session capped at the untrusted session duration, the refresh token no longer than it",
			req:  Request{Email: "jane@example.com", Password: "Passw0rd!"},
			stub: func(q *sqlc.MockQuerier, sq *authsharedsqlc.MockQuerier, m *jwttoken.MockM) {
				q.EXPECT().GetUserForLogin(gomock.Any(), "jane@example.com").Return(active, nil)
				m.EXPECT().CreateToken(gomock.Any(), familyID, 15*time.Minute, 12*time.Hour).Return(pair, nil)
				sq.EXPECT().StartSession(gomock.Any(), capped).Return(nil)
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
				m.EXPECT().CreateToken(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(jwttoken.Pair{}, errors.New("sign"))
			},
			wantCode: apperror.CodeInternal,
		},
		{
			name: "a refresh-token insert error is a 500",
			req:  Request{Email: "jane@example.com", Password: "Passw0rd!"},
			stub: func(q *sqlc.MockQuerier, sq *authsharedsqlc.MockQuerier, m *jwttoken.MockM) {
				q.EXPECT().GetUserForLogin(gomock.Any(), gomock.Any()).Return(active, nil)
				m.EXPECT().CreateToken(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(pair, nil)
				sq.EXPECT().StartSession(gomock.Any(), gomock.Any()).Return(sql.ErrConnDone)
			},
			wantCode: apperror.CodeInternal,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			q := sqlc.NewMockQuerier(ctrl)
			sq := testhelper.FoldingQuerier(ctrl)
			m := jwttoken.NewMockM(ctrl)
			tc.stub(q, sq, m)

			svc := NewService(q, sq, m, 15*time.Minute, 24*time.Hour, 12*time.Hour, wideLimits())
			svc.(*service).newFamilyID = func() uuid.UUID { return familyID }
			svc.(*service).now = func() time.Time { return now }

			got, err := svc.Login(context.Background(), tc.req, testClient)
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
			sq := testhelper.FoldingQuerier(ctrl)
			m := jwttoken.NewMockM(ctrl)
			svc := NewService(q, sq, m, 15*time.Minute, 24*time.Hour, 12*time.Hour,
				Limiters{Email: ratelimit.NewMemory(ratelimit.Rate{Burst: 2, Per: time.Minute}), IP: wideLimits().IP})

			for i, a := range tc.attempts {
				if a.wantCode != apperror.CodeTooManyRequests {
					row := sqlc.GetUserForLoginRow{}
					if a.user != nil {
						row = *a.user
					}
					q.EXPECT().GetUserForLogin(gomock.Any(), gomock.Any()).Return(row, a.readErr)
				}
				if a.wantCode.IsZero() {
					m.EXPECT().CreateToken(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(jwttoken.Pair{}, nil)
					sq.EXPECT().StartSession(gomock.Any(), gomock.Any()).Return(nil)
				}

				_, err := svc.Login(context.Background(), Request{Email: a.email, Password: a.password}, testClient)
				if a.wantCode.IsZero() {
					require.NoError(t, err, "attempt %d", i)
					continue
				}
				var appErr *apperror.AppError
				require.ErrorAs(t, err, &appErr, "attempt %d", i)
				require.Equal(t, a.wantCode, appErr.Code, "attempt %d", i)
				if a.wantCode == apperror.CodeTooManyRequests {
					require.Equal(t, tooManyFailuresMessage, appErr.Message)
					require.Positive(t, appErr.RetryAfter, "attempt %d: the 429 says how long to wait", i)
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
			svc := NewService(q, testhelper.FoldingQuerier(ctrl), jwttoken.NewMockM(ctrl),
				15*time.Minute, 24*time.Hour, 12*time.Hour, wideLimits())

			var calls [][]string
			svc.(*service).checkPassword = func(hash, plain string) bool {
				calls = append(calls, []string{hash, plain})
				return false
			}

			_, err := svc.Login(context.Background(), Request{Email: "jane@example.com", Password: "guess"}, testClient)
			var appErr *apperror.AppError
			require.ErrorAs(t, err, &appErr)
			require.Equal(t, apperror.CodeUnauthorized, appErr.Code)
			require.Equal(t, invalidCredentialsMessage, appErr.Message, "same answer either way")
			require.Equal(t, [][]string{tc.wantCheck}, calls)
		})
	}
}

// stubLimiter answers Take with takeErr, or with a refusal when refuse is set,
// fails Reset / Return with giveBackErr, and records the calls.
type stubLimiter struct {
	takeErr, giveBackErr error
	refuse               bool
	calls                []string
	giveBackCtxErrs      []error // ctx.Err() as each Reset / Return saw it
}

func (l *stubLimiter) Take(_ context.Context, key string) (ratelimit.Decision, error) {
	l.calls = append(l.calls, "Take "+key)
	if l.refuse {
		return ratelimit.Decision{RetryAfter: time.Minute}, nil
	}
	return ratelimit.Decision{Allowed: l.takeErr == nil}, l.takeErr
}

func (l *stubLimiter) Reset(ctx context.Context, key string) error {
	l.calls = append(l.calls, "Reset "+key)
	l.giveBackCtxErrs = append(l.giveBackCtxErrs, ctx.Err())
	return l.giveBackErr
}

func (l *stubLimiter) Return(ctx context.Context, key string) error {
	l.calls = append(l.calls, "Return "+key)
	l.giveBackCtxErrs = append(l.giveBackCtxErrs, ctx.Err())
	return l.giveBackErr
}

// TestService_Login_GiveBackOutlivesRequest_U: a client that hangs up cancels
// the request ctx, and a shared store (Redis) refuses a call on a cancelled
// ctx; a give-back on it would leave the count taken although no password
// was tried (or the right one was). Every give-back runs on a ctx that
// outlives the request.
func TestService_Login_GiveBackOutlivesRequest_U(t *testing.T) {
	hash, err := password.Hash("Passw0rd!")
	require.NoError(t, err)
	active := sqlc.GetUserForLoginRow{ID: 1, Name: "Jane", Email: "jane@example.com", PasswordHash: hash, Role: "user", IsActive: true}
	testCases := []struct {
		name        string
		email       *stubLimiter
		foldErr     error
		readsUser   bool
		wantIPGives int
		wantEmGives int
	}{
		{name: "a fold that fails because the client went away still gives the IP its token back",
			email: &stubLimiter{}, foldErr: context.Canceled, wantIPGives: 1},
		{name: "an email that refuses after the client went away still gives the IP its token back",
			email: &stubLimiter{refuse: true}, wantIPGives: 1},
		{name: "the right password still resets the email and returns the IP's token after the client went away",
			email: &stubLimiter{}, readsUser: true, wantIPGives: 1, wantEmGives: 1},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			q := sqlc.NewMockQuerier(ctrl)
			sq := authsharedsqlc.NewMockQuerier(ctrl)
			sq.EXPECT().FoldEmail(gomock.Any(), gomock.Any()).Return("jane@example.com", tc.foldErr)
			if tc.readsUser {
				q.EXPECT().GetUserForLogin(gomock.Any(), gomock.Any()).Return(active, nil)
			}
			tm := jwttoken.NewMockM(ctrl)
			tm.EXPECT().CreateToken(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(jwttoken.Pair{}, nil).AnyTimes()
			sq.EXPECT().StartSession(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
			ip := &stubLimiter{}
			svc := NewService(q, sq, tm, 15*time.Minute, 24*time.Hour, 12*time.Hour, Limiters{Email: tc.email, IP: ip})

			ctx, cancel := context.WithCancel(context.Background())
			cancel() // the client hung up
			_, _ = svc.Login(ctx, Request{Email: "jane@example.com", Password: "Passw0rd!"}, testClient)

			require.Len(t, ip.giveBackCtxErrs, tc.wantIPGives)
			require.Len(t, tc.email.giveBackCtxErrs, tc.wantEmGives)
			for _, e := range append(ip.giveBackCtxErrs, tc.email.giveBackCtxErrs...) {
				require.NoError(t, e, "a give-back ran on the cancelled request ctx")
			}
		})
	}
}

// TestService_Login_LimiterOutcomes_U pins what the two limiters' answers do:
// the IP's token is taken first, then the email's, both before the DB read;
// an error is a 500 and a refusal (an empty bucket, or a full bucket map
// refusing a new key) a 429, both before the DB read — never an uncounted
// guess; a refusal by the IP leaves the email's budget alone, and one by the
// email gives the IP's token back (no password was tried); the right password
// refills the email's bucket and returns one IP token; a failed give-back
// still logs in, logged.
func TestService_Login_LimiterOutcomes_U(t *testing.T) {
	hash, err := password.Hash("Passw0rd!")
	require.NoError(t, err)
	active := sqlc.GetUserForLoginRow{ID: 1, Name: "Jane", Email: "jane@example.com", PasswordHash: hash, Role: "user", IsActive: true}
	storeDown := errors.New("store down")
	const emailKey = "login:jane@example.com"
	const ipKey = "loginip:203.0.113.7"

	testCases := []struct {
		name          string
		email, ip     *stubLimiter
		password      string
		readsUser     bool
		wantCode      apperror.Code // zero = success
		wantErr       error
		wantEmail     []string
		wantIP        []string
		wantLog       string
		wantLimiterAt string // the "limiter" attribute of the logged give-back failure
	}{
		{name: "an IP limiter that fails is a 500 before the email is counted or the DB read",
			email: &stubLimiter{}, ip: &stubLimiter{takeErr: storeDown}, password: "wrong",
			wantCode: apperror.CodeInternal, wantErr: storeDown, wantIP: []string{"Take " + ipKey}},
		{name: "an IP limiter that refuses is a 429 with the limiter's wait that leaves the email's budget alone, the right password too",
			email: &stubLimiter{}, ip: &stubLimiter{refuse: true}, password: "Passw0rd!",
			wantCode: apperror.CodeTooManyRequests, wantIP: []string{"Take " + ipKey}},
		{name: "an email limiter that fails is a 500 before the DB read, and the IP gets its token back",
			email: &stubLimiter{takeErr: storeDown}, ip: &stubLimiter{}, password: "wrong",
			wantCode: apperror.CodeInternal, wantErr: storeDown,
			wantEmail: []string{"Take " + emailKey}, wantIP: []string{"Take " + ipKey, "Return " + ipKey}},
		{name: "an email limiter that refuses is a 429 with the limiter's wait before the DB read, the right password too, and the IP gets its token back",
			email: &stubLimiter{refuse: true}, ip: &stubLimiter{}, password: "Passw0rd!",
			wantCode:  apperror.CodeTooManyRequests,
			wantEmail: []string{"Take " + emailKey}, wantIP: []string{"Take " + ipKey, "Return " + ipKey}},
		{name: "a wrong password keeps both tokens",
			email: &stubLimiter{}, ip: &stubLimiter{}, password: "wrong", readsUser: true,
			wantCode:  apperror.CodeUnauthorized,
			wantEmail: []string{"Take " + emailKey}, wantIP: []string{"Take " + ipKey}},
		{name: "the right password refills the email's bucket and returns only this attempt's IP token",
			email: &stubLimiter{}, ip: &stubLimiter{}, password: "Passw0rd!", readsUser: true,
			wantEmail: []string{"Take " + emailKey, "Reset " + emailKey}, wantIP: []string{"Take " + ipKey, "Return " + ipKey}},
		{name: "the right password still logs in when the email's bucket can't be refilled, and it's logged",
			email: &stubLimiter{giveBackErr: storeDown}, ip: &stubLimiter{}, password: "Passw0rd!", readsUser: true,
			wantEmail: []string{"Take " + emailKey, "Reset " + emailKey}, wantIP: []string{"Take " + ipKey, "Return " + ipKey},
			wantLog: `"msg":"login: give back failure count failed"`, wantLimiterAt: `"limiter":"email"`},
		{name: "the right password still logs in when the IP token can't be returned, and it's logged",
			email: &stubLimiter{}, ip: &stubLimiter{giveBackErr: storeDown}, password: "Passw0rd!", readsUser: true,
			wantEmail: []string{"Take " + emailKey, "Reset " + emailKey}, wantIP: []string{"Take " + ipKey, "Return " + ipKey},
			wantLog: `"msg":"login: give back failure count failed"`, wantLimiterAt: `"limiter":"ip"`},
		{name: "an email refusal whose IP give-back fails is still a 429, logged",
			email: &stubLimiter{refuse: true}, ip: &stubLimiter{giveBackErr: storeDown}, password: "wrong",
			wantCode:  apperror.CodeTooManyRequests,
			wantEmail: []string{"Take " + emailKey}, wantIP: []string{"Take " + ipKey, "Return " + ipKey},
			wantLog: `"msg":"login: give back failure count failed"`, wantLimiterAt: `"limiter":"ip"`},
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
			sq := testhelper.FoldingQuerier(ctrl)
			m := jwttoken.NewMockM(ctrl)
			if tc.readsUser {
				q.EXPECT().GetUserForLogin(gomock.Any(), gomock.Any()).Return(active, nil)
			}
			if tc.wantCode.IsZero() {
				m.EXPECT().CreateToken(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(jwttoken.Pair{}, nil)
				sq.EXPECT().StartSession(gomock.Any(), gomock.Any()).Return(nil)
			}
			svc := NewService(q, sq, m, 15*time.Minute, 24*time.Hour, 12*time.Hour, Limiters{Email: tc.email, IP: tc.ip})

			_, err = svc.Login(context.Background(), Request{Email: "jane@example.com", Password: tc.password}, testClient)
			require.Equal(t, tc.wantEmail, tc.email.calls, "email limiter calls")
			require.Equal(t, tc.wantIP, tc.ip.calls, "IP limiter calls")
			if tc.wantLog != "" {
				require.Contains(t, buf.String(), tc.wantLog)
				require.Contains(t, buf.String(), tc.wantLimiterAt)
			} else {
				require.NotContains(t, buf.String(), "give back failure count failed")
			}
			if tc.wantCode.IsZero() {
				require.NoError(t, err)
				return
			}
			var appErr *apperror.AppError
			require.ErrorAs(t, err, &appErr)
			require.Equal(t, tc.wantCode, appErr.Code)
			if tc.wantCode == apperror.CodeTooManyRequests {
				require.Equal(t, tooManyFailuresMessage, appErr.Message)
				require.Equal(t, time.Minute, appErr.RetryAfter, "the 429 carries the limiter's wait (Retry-After)")
			} else {
				require.Zero(t, appErr.RetryAfter, "only a 429 carries a wait")
			}
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
			}
		})
	}
}

// TestService_Login_IPFailureLimit_U pins the per-IP cap on failed logins
// across emails (credential stuffing tries each email once), against an IP
// limiter of 3 failures per window.
func TestService_Login_IPFailureLimit_U(t *testing.T) {
	hash, err := password.Hash("Passw0rd!")
	require.NoError(t, err)
	jane := sqlc.GetUserForLoginRow{ID: 1, Name: "Jane", Email: "jane@example.com", PasswordHash: hash, Role: "user", IsActive: true}

	const attacker, office = "198.51.100.9", "192.0.2.50"
	// attempt is one login; a 429 attempt must not reach the DB.
	type attempt struct {
		ip, email, password string
		user                *sqlc.GetUserForLoginRow
		wantCode            apperror.Code // zero = success
	}
	wrong := func(ip, email string) attempt {
		return attempt{ip: ip, email: email, password: "wrong", user: &jane, wantCode: apperror.CodeUnauthorized}
	}
	unknown := func(ip, email string) attempt {
		return attempt{ip: ip, email: email, password: "x", wantCode: apperror.CodeUnauthorized}
	}
	right := func(ip string) attempt {
		return attempt{ip: ip, email: "jane@example.com", password: "Passw0rd!", user: &jane}
	}
	inactive := jane
	inactive.IsActive = false
	inactiveRight := func(ip string) attempt {
		return attempt{ip: ip, email: "jane@example.com", password: "Passw0rd!", user: &inactive, wantCode: apperror.CodeUnauthorized}
	}
	blocked := func(ip, email string) attempt {
		return attempt{ip: ip, email: email, password: "Passw0rd!", wantCode: apperror.CodeTooManyRequests}
	}

	testCases := []struct {
		name       string
		emailBurst int
		attempts   []attempt
	}{
		{name: "failures for different emails from one IP add up; past the cap every login from it is a 429, the right password too",
			emailBurst: 5,
			attempts: []attempt{unknown(attacker, "a@example.com"), unknown(attacker, "b@example.com"),
				wrong(attacker, "jane@example.com"), blocked(attacker, "jane@example.com"), blocked(attacker, "c@example.com")}},
		{name: "a right password gives its own IP token back, so successes never fill the IP's budget",
			emailBurst: 5,
			attempts: []attempt{right(office), right(office), right(office), right(office), right(office),
				wrong(office, "jane@example.com"), wrong(office, "jane@example.com"), unknown(office, "a@example.com"),
				blocked(office, "jane@example.com")}},
		{name: "a right password doesn't wipe the IP's failures (one real account can't reset an attacker's count)",
			emailBurst: 5,
			attempts: []attempt{unknown(attacker, "a@example.com"), unknown(attacker, "b@example.com"), right(attacker),
				unknown(attacker, "c@example.com"), blocked(attacker, "d@example.com")}},
		{name: "a deactivated account with the right password gives its IP token back too (it isn't a guess)",
			emailBurst: 5,
			attempts: []attempt{inactiveRight(office), inactiveRight(office), inactiveRight(office), inactiveRight(office),
				unknown(office, "a@example.com"), unknown(office, "b@example.com"), unknown(office, "c@example.com"),
				blocked(office, "d@example.com")}},
		{name: "another IP keeps its own count",
			emailBurst: 5,
			attempts: []attempt{unknown(attacker, "a@example.com"), unknown(attacker, "b@example.com"),
				unknown(attacker, "c@example.com"), blocked(attacker, "d@example.com"), right(office)}},
		{name: "hammering an email that is already locked doesn't spend the IP's budget",
			emailBurst: 1,
			attempts: []attempt{wrong(office, "jane@example.com"), blocked(office, "jane@example.com"),
				blocked(office, "jane@example.com"), blocked(office, "jane@example.com"), blocked(office, "jane@example.com"),
				unknown(office, "a@example.com"), unknown(office, "b@example.com"), blocked(office, "c@example.com")}},
		{name: "an IP past its cap doesn't spend the email's budget: the owner logs in from elsewhere",
			emailBurst: 1,
			attempts: []attempt{unknown(attacker, "a@example.com"), unknown(attacker, "b@example.com"),
				unknown(attacker, "c@example.com"), blocked(attacker, "jane@example.com"), blocked(attacker, "jane@example.com"),
				right(office)}},
		{name: "an IPv6 client is counted per /64, not per address",
			emailBurst: 5,
			attempts: []attempt{unknown("2001:db8:1:1::1", "a@example.com"), unknown("2001:db8:1:1::2", "b@example.com"),
				unknown("2001:db8:1:1:ffff::3", "c@example.com"), blocked("2001:db8:1:1::4", "d@example.com")}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			q := sqlc.NewMockQuerier(ctrl)
			sq := testhelper.FoldingQuerier(ctrl)
			m := jwttoken.NewMockM(ctrl)
			svc := NewService(q, sq, m, 15*time.Minute, 24*time.Hour, 12*time.Hour, Limiters{
				Email: ratelimit.NewMemory(ratelimit.Rate{Burst: tc.emailBurst, Per: time.Hour}),
				IP:    ratelimit.NewMemory(ratelimit.Rate{Burst: 3, Per: time.Hour}),
			})

			for i, a := range tc.attempts {
				if a.wantCode != apperror.CodeTooManyRequests {
					if a.user != nil {
						q.EXPECT().GetUserForLogin(gomock.Any(), gomock.Any()).Return(*a.user, nil)
					} else {
						q.EXPECT().GetUserForLogin(gomock.Any(), gomock.Any()).Return(sqlc.GetUserForLoginRow{}, sql.ErrNoRows)
					}
				}
				if a.wantCode.IsZero() {
					m.EXPECT().CreateToken(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(jwttoken.Pair{}, nil)
					sq.EXPECT().StartSession(gomock.Any(), gomock.Any()).Return(nil)
				}

				_, err := svc.Login(context.Background(), Request{Email: a.email, Password: a.password}, Client{IP: a.ip})
				if a.wantCode.IsZero() {
					require.NoError(t, err, "attempt %d", i)
					continue
				}
				var appErr *apperror.AppError
				require.ErrorAs(t, err, &appErr, "attempt %d", i)
				require.Equal(t, a.wantCode, appErr.Code, "attempt %d", i)
				if a.wantCode == apperror.CodeTooManyRequests {
					require.Equal(t, tooManyFailuresMessage, appErr.Message, "attempt %d", i)
					require.Positive(t, appErr.RetryAfter, "attempt %d: the 429 says how long to wait", i)
				}
			}
		})
	}
}

// TestService_Login_ParallelGuesses_U pins that parallel wrong guesses can't
// outrun either count — for one email, or for many emails from one IP: while
// every guess that got through is still in its (slow) bcrypt comparison, the
// rest are already 429s, so exactly the burst reach the comparison.
func TestService_Login_ParallelGuesses_U(t *testing.T) {
	active := sqlc.GetUserForLoginRow{ID: 1, Email: "jane@example.com", PasswordHash: "$2a$10$stored", IsActive: true}

	testCases := []struct {
		name         string
		emailBurst   int
		ipBurst      int
		manyEmails   bool // each guess for its own email, all from one IP
		parallel     int
		wantCompared int32
	}{
		{name: "20 parallel guesses for one email against 2 tries: 2 reach bcrypt, 18 are 429s",
			emailBurst: 2, ipBurst: 1000, parallel: 20, wantCompared: 2},
		{name: "3 parallel guesses for one email against 5 tries: all 3 reach bcrypt",
			emailBurst: 5, ipBurst: 1000, parallel: 3, wantCompared: 3},
		{name: "20 parallel guesses for 20 emails from one IP against 2 tries: 2 reach bcrypt, 18 are 429s",
			emailBurst: 5, ipBurst: 2, manyEmails: true, parallel: 20, wantCompared: 2},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			q := sqlc.NewMockQuerier(ctrl)
			q.EXPECT().GetUserForLogin(gomock.Any(), gomock.Any()).Return(active, nil).AnyTimes()
			svc := NewService(q, testhelper.FoldingQuerier(ctrl), jwttoken.NewMockM(ctrl),
				15*time.Minute, 24*time.Hour, 12*time.Hour, Limiters{
					Email: ratelimit.NewMemory(ratelimit.Rate{Burst: tc.emailBurst, Per: time.Hour}),
					IP:    ratelimit.NewMemory(ratelimit.Rate{Burst: tc.ipBurst, Per: time.Hour}),
				})

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
			for i := range tc.parallel {
				email := "jane@example.com"
				if tc.manyEmails {
					email = fmt.Sprintf("user%d@example.com", i)
				}
				wg.Go(func() {
					_, err := svc.Login(context.Background(), Request{Email: email, Password: "guess"}, testClient)
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

func TestService_Login_FoldEmail_U(t *testing.T) {
	ipKey := ipFailureKey(testClient.IP)
	testCases := []struct {
		name           string
		ipRefuses      bool
		folded         string
		foldErr        error
		wantFold       bool
		wantCode       apperror.Code
		wantEmailCalls []string
		wantIPCalls    []string
	}{
		{name: "the per-email key is the email as Postgres folds it (the lookup's fold), not Go's ToLower",
			folded: "ƛjane@example.com", wantFold: true, wantCode: apperror.CodeUnauthorized,
			wantEmailCalls: []string{"Take login:ƛjane@example.com"}, wantIPCalls: []string{"Take " + ipKey}},
		{name: "a login the IP cap refuses never reaches the database, not even the fold",
			ipRefuses: true, wantCode: apperror.CodeTooManyRequests, wantIPCalls: []string{"Take " + ipKey}},
		{name: "a fold error is a 500 and gives the IP its token back (no password was tried)",
			foldErr: sql.ErrConnDone, wantFold: true, wantCode: apperror.CodeInternal,
			wantIPCalls: []string{"Take " + ipKey, "Return " + ipKey}},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			q := sqlc.NewMockQuerier(ctrl)
			sq := authsharedsqlc.NewMockQuerier(ctrl) // no FoldEmail expected unless wantFold
			if tc.wantFold {
				sq.EXPECT().FoldEmail(gomock.Any(), "ꟜJane@example.com").Return(tc.folded, tc.foldErr) // trimmed first
			}
			if tc.wantFold && tc.foldErr == nil {
				// the lookup gets the same trimmed email the fold got
				q.EXPECT().GetUserForLogin(gomock.Any(), "ꟜJane@example.com").Return(sqlc.GetUserForLoginRow{}, sql.ErrNoRows)
			}
			email, ip := &stubLimiter{}, &stubLimiter{refuse: tc.ipRefuses}
			svc := NewService(q, sq, jwttoken.NewMockM(ctrl), 15*time.Minute, 24*time.Hour, 12*time.Hour, Limiters{Email: email, IP: ip})

			_, err := svc.Login(context.Background(), Request{Email: "  ꟜJane@example.com ", Password: "Passw0rd!"}, testClient)
			var appErr *apperror.AppError
			require.ErrorAs(t, err, &appErr)
			require.Equal(t, tc.wantCode, appErr.Code)
			require.Equal(t, tc.wantEmailCalls, email.calls)
			require.Equal(t, tc.wantIPCalls, ip.calls)
		})
	}
}
