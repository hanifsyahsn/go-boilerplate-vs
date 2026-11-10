package login

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/login/sqlc"
	authshared "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/shared"
	authsharedsqlc "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/shared/sqlc"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/jwttoken"
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

			svc := NewService(q, sq, m, 15*time.Minute, 24*time.Hour, 12*time.Hour, ratelimit.New(5, time.Minute))
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
		{name: "wrong passwords count; once blocked even the right password is a 429 until the window ends",
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
			svc := NewService(q, sq, m, 15*time.Minute, 24*time.Hour, 12*time.Hour, ratelimit.New(2, time.Minute))

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
				15*time.Minute, 24*time.Hour, 12*time.Hour, ratelimit.New(5, time.Minute))

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
