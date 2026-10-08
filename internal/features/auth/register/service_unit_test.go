package register

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/register/sqlc"
	authshared "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/shared"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/apperror"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

func TestService_Register_U(t *testing.T) {
	req := Request{Name: " Jane ", Email: " jane@example.com ", Password: "Passw0rd!"}

	testCases := []struct {
		name     string
		hashErr  error
		stub     func(q *sqlc.MockQuerier)
		want     authshared.UserResource
		wantCode apperror.Code
	}{
		{
			name: "trimmed name / email, hashed password",
			stub: func(q *sqlc.MockQuerier) {
				q.EXPECT().CreateUser(gomock.Any(), sqlc.CreateUserParams{Name: "Jane", Email: "jane@example.com", PasswordHash: "hashed"}).
					Return(sqlc.CreateUserRow{ID: 1, Name: "Jane", Email: "jane@example.com", Role: "user", IsActive: true}, nil)
			},
			want: authshared.UserResource{ID: "1", Name: "Jane", Email: "jane@example.com", Role: "user"},
		},
		{
			name: "a taken email is a 409",
			stub: func(q *sqlc.MockQuerier) {
				q.EXPECT().CreateUser(gomock.Any(), gomock.Any()).Return(sqlc.CreateUserRow{}, &pq.Error{Code: "23505"})
			},
			wantCode: apperror.CodeConflict,
		},
		{
			name: "an insert error is a 500",
			stub: func(q *sqlc.MockQuerier) {
				q.EXPECT().CreateUser(gomock.Any(), gomock.Any()).Return(sqlc.CreateUserRow{}, sql.ErrConnDone)
			},
			wantCode: apperror.CodeInternal,
		},
		{
			name:     "a hash error is a 500, nothing written",
			hashErr:  errors.New("bcrypt"),
			stub:     func(q *sqlc.MockQuerier) {},
			wantCode: apperror.CodeInternal,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			q := sqlc.NewMockQuerier(ctrl)
			tc.stub(q)

			svc := &service{querier: q, hash: func(string) (string, error) { return "hashed", tc.hashErr }}
			got, err := svc.Register(context.Background(), req)
			if tc.wantCode != "" {
				var appErr *apperror.AppError
				require.ErrorAs(t, err, &appErr)
				require.Equal(t, tc.wantCode, appErr.Code)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}
