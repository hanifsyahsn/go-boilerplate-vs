package logout

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	authsharedsqlc "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/shared/sqlc"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/jwttoken"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/apperror"
	"github.com/stretchr/testify/require"
)

func TestService_Logout_U(t *testing.T) {
	jti := uuid.New()
	claims := jwt.MapClaims{"sub": float64(1), "jti": jti.String()}

	testCases := []struct {
		name     string
		token    string
		stub     func(q *authsharedsqlc.MockQuerier, m *jwttoken.MockM)
		wantCode apperror.Code
	}{
		{
			name:  "a valid token: its family revoked",
			token: "ref",
			stub: func(q *authsharedsqlc.MockQuerier, m *jwttoken.MockM) {
				m.EXPECT().VerifyRefreshToken("ref").Return(claims, nil)
				q.EXPECT().RevokeRefreshTokenFamily(gomock.Any(), jti).Return(nil)
			},
		},
		{name: "no token: nothing to do", token: "", stub: func(q *authsharedsqlc.MockQuerier, m *jwttoken.MockM) {}},
		{
			name:  "an invalid or expired token: nothing to do",
			token: "bad",
			stub: func(q *authsharedsqlc.MockQuerier, m *jwttoken.MockM) {
				m.EXPECT().VerifyRefreshToken("bad").Return(nil, errors.New("expired"))
			},
		},
		{
			name:  "a jti that isn't a uuid: nothing to do",
			token: "ref",
			stub: func(q *authsharedsqlc.MockQuerier, m *jwttoken.MockM) {
				m.EXPECT().VerifyRefreshToken("ref").Return(jwt.MapClaims{"sub": float64(1), "jti": "abc"}, nil)
			},
		},
		{
			name:  "a revoke error is a 500",
			token: "ref",
			stub: func(q *authsharedsqlc.MockQuerier, m *jwttoken.MockM) {
				m.EXPECT().VerifyRefreshToken("ref").Return(claims, nil)
				q.EXPECT().RevokeRefreshTokenFamily(gomock.Any(), jti).Return(sql.ErrConnDone)
			},
			wantCode: apperror.CodeInternal,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			q := authsharedsqlc.NewMockQuerier(ctrl)
			m := jwttoken.NewMockM(ctrl)
			tc.stub(q, m)

			err := NewService(q, m).Logout(context.Background(), tc.token)
			if tc.wantCode != "" {
				var appErr *apperror.AppError
				require.ErrorAs(t, err, &appErr)
				require.Equal(t, tc.wantCode, appErr.Code)
				return
			}
			require.NoError(t, err)
		})
	}
}
