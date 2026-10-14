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

			svc := NewService(q, sq, m, 15*time.Minute, 24*time.Hour, 12*time.Hour)
			svc.(*service).newFamilyID = func() uuid.UUID { return familyID }
			svc.(*service).now = func() time.Time { return now }

			got, err := svc.Login(context.Background(), tc.req)
			if tc.wantCode != "" {
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
