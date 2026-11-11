package register

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/register/sqlc"
	authshared "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/shared"
	authsharedsqlc "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/shared/sqlc"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/mailer"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/apperror"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

func TestService_Register_U(t *testing.T) {
	req := Request{Name: " Jane ", Email: " jane@example.com ", Password: "Passw0rd!"}
	now := time.Date(2026, 11, 11, 9, 0, 0, 0, time.UTC)

	testCases := []struct {
		name      string
		hashErr   error
		stub      func(q *MockQuerier)
		want      authshared.UserResource
		wantCode  apperror.Code
		wantEmail bool
	}{
		{
			name: "trimmed name / email, hashed password, a 24 h verification token issued in the same transaction, then the link emailed",
			stub: func(q *MockQuerier) {
				gomock.InOrder(
					q.EXPECT().CreateUser(gomock.Any(), sqlc.CreateUserParams{Name: "Jane", Email: "jane@example.com", PasswordHash: "hashed"}).
						Return(sqlc.CreateUserRow{ID: 1, Name: "Jane", Email: "jane@example.com", Role: "user", IsActive: true}, nil),
					q.EXPECT().SpendUnusedAuthTokens(gomock.Any(), authsharedsqlc.SpendUnusedAuthTokensParams{UserID: 1, Purpose: authshared.PurposeVerifyEmail}).Return(nil),
					q.EXPECT().InsertAuthToken(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, p authsharedsqlc.InsertAuthTokenParams) error {
						require.Equal(t, authshared.PurposeVerifyEmail, p.Purpose)
						require.Equal(t, now.Add(24*time.Hour), p.ExpiresAt)
						return nil
					}),
				)
			},
			want:      authshared.UserResource{ID: "1", Name: "Jane", Email: "jane@example.com", Role: "user"},
			wantEmail: true,
		},
		{
			name: "a token error is a 500 and no email (the account rolls back with it)",
			stub: func(q *MockQuerier) {
				q.EXPECT().CreateUser(gomock.Any(), gomock.Any()).Return(sqlc.CreateUserRow{ID: 1}, nil)
				q.EXPECT().SpendUnusedAuthTokens(gomock.Any(), gomock.Any()).Return(sql.ErrConnDone)
			},
			wantCode: apperror.CodeInternal,
		},
		{
			name: "a taken email is a 409",
			stub: func(q *MockQuerier) {
				q.EXPECT().CreateUser(gomock.Any(), gomock.Any()).Return(sqlc.CreateUserRow{}, &pq.Error{Code: "23505"})
			},
			wantCode: apperror.CodeConflict,
		},
		{
			name: "an insert error is a 500",
			stub: func(q *MockQuerier) {
				q.EXPECT().CreateUser(gomock.Any(), gomock.Any()).Return(sqlc.CreateUserRow{}, sql.ErrConnDone)
			},
			wantCode: apperror.CodeInternal,
		},
		{
			name:     "a hash error is a 500, nothing written",
			hashErr:  errors.New("bcrypt"),
			stub:     func(*MockQuerier) {},
			wantCode: apperror.CodeInternal,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			q := NewMockQuerier(ctrl)
			tc.stub(q)
			store := NewMockStoreI(ctrl)
			store.EXPECT().RunInTx(gomock.Any(), gomock.Any()).DoAndReturn(
				func(ctx context.Context, fn func(Querier) error) error { return fn(q) }).AnyTimes()
			fake := &mailer.Fake{}

			svc := &service{store: store, hash: func(string) (string, error) { return "hashed", tc.hashErr },
				now: func() time.Time { return now },
				emailer: authshared.Emailer{Mailer: fake, AppBaseURL: "https://app.example.com",
					Go: func(ctx context.Context, fn func(context.Context)) { fn(ctx) }}}
			got, err := svc.Register(context.Background(), req)
			if !tc.wantCode.IsZero() {
				var appErr *apperror.AppError
				require.ErrorAs(t, err, &appErr)
				require.Equal(t, tc.wantCode, appErr.Code)
				require.Empty(t, fake.Sent())
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
			sent := fake.Sent()
			require.Len(t, sent, 1)
			require.Equal(t, "Verify your email", sent[0].Subject)
			require.Equal(t, "jane@example.com", sent[0].To)
			require.Contains(t, sent[0].Text, "https://app.example.com/verify-email?token=")
		})
	}
}
