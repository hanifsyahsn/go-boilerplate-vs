package logout

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/logout/sqlc"
	authshared "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/shared"
	authsharedsqlc "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/shared/sqlc"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/jwttoken"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/apperror"
	"github.com/stretchr/testify/require"
)

func TestService_Logout_U(t *testing.T) {
	jti := uuid.New()
	familyID := uuid.New()
	claims := jwt.MapClaims{"sub": float64(1), "jti": jti.String()}
	revoke := authsharedsqlc.RevokeSessionParams{FamilyID: familyID, Reason: authshared.RevokeReasonLogout}

	testCases := []struct {
		name     string
		token    string
		stub     func(q *sqlc.MockQuerier, sq *authsharedsqlc.MockQuerier, m *jwttoken.MockM)
		wantCode apperror.Code
	}{
		{
			name:  "a valid token: its session revoked as logout",
			token: "ref",
			stub: func(q *sqlc.MockQuerier, sq *authsharedsqlc.MockQuerier, m *jwttoken.MockM) {
				m.EXPECT().VerifyRefreshToken("ref").Return(claims, nil)
				q.EXPECT().GetRefreshTokenFamily(gomock.Any(), jti).Return(familyID, nil)
				sq.EXPECT().RevokeSession(gomock.Any(), revoke).Return(int64(1), nil)
			},
		},
		{
			name:  "a session already revoked: still a success",
			token: "ref",
			stub: func(q *sqlc.MockQuerier, sq *authsharedsqlc.MockQuerier, m *jwttoken.MockM) {
				m.EXPECT().VerifyRefreshToken("ref").Return(claims, nil)
				q.EXPECT().GetRefreshTokenFamily(gomock.Any(), jti).Return(familyID, nil)
				sq.EXPECT().RevokeSession(gomock.Any(), revoke).Return(int64(0), nil)
			},
		},
		{name: "no token: nothing to do", token: "", stub: func(*sqlc.MockQuerier, *authsharedsqlc.MockQuerier, *jwttoken.MockM) {}},
		{
			name:  "an invalid or expired token: nothing to do",
			token: "bad",
			stub: func(q *sqlc.MockQuerier, sq *authsharedsqlc.MockQuerier, m *jwttoken.MockM) {
				m.EXPECT().VerifyRefreshToken("bad").Return(nil, errors.New("expired"))
			},
		},
		{
			name:  "a jti that isn't a uuid: nothing to do",
			token: "ref",
			stub: func(q *sqlc.MockQuerier, sq *authsharedsqlc.MockQuerier, m *jwttoken.MockM) {
				m.EXPECT().VerifyRefreshToken("ref").Return(jwt.MapClaims{"sub": float64(1), "jti": "abc"}, nil)
			},
		},
		{
			name:  "a token with no row: nothing to revoke",
			token: "ref",
			stub: func(q *sqlc.MockQuerier, sq *authsharedsqlc.MockQuerier, m *jwttoken.MockM) {
				m.EXPECT().VerifyRefreshToken("ref").Return(claims, nil)
				q.EXPECT().GetRefreshTokenFamily(gomock.Any(), jti).Return(uuid.Nil, sql.ErrNoRows)
			},
		},
		{
			name:  "a family read error is a 500",
			token: "ref",
			stub: func(q *sqlc.MockQuerier, sq *authsharedsqlc.MockQuerier, m *jwttoken.MockM) {
				m.EXPECT().VerifyRefreshToken("ref").Return(claims, nil)
				q.EXPECT().GetRefreshTokenFamily(gomock.Any(), jti).Return(uuid.Nil, sql.ErrConnDone)
			},
			wantCode: apperror.CodeInternal,
		},
		{
			name:  "a revoke error is a 500",
			token: "ref",
			stub: func(q *sqlc.MockQuerier, sq *authsharedsqlc.MockQuerier, m *jwttoken.MockM) {
				m.EXPECT().VerifyRefreshToken("ref").Return(claims, nil)
				q.EXPECT().GetRefreshTokenFamily(gomock.Any(), jti).Return(familyID, nil)
				sq.EXPECT().RevokeSession(gomock.Any(), revoke).Return(int64(0), sql.ErrConnDone)
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

			err := NewService(q, sq, m).Logout(context.Background(), tc.token)
			if !tc.wantCode.IsZero() {
				var appErr *apperror.AppError
				require.ErrorAs(t, err, &appErr)
				require.Equal(t, tc.wantCode, appErr.Code)
				return
			}
			require.NoError(t, err)
		})
	}
}
