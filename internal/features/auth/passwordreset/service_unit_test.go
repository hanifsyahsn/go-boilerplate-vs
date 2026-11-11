package passwordreset

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/passwordreset/sqlc"
	authshared "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/shared"
	authsharedsqlc "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/shared/sqlc"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/mailer"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/apperror"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/onetimetoken"
	"github.com/stretchr/testify/require"
)

func TestService_Reset_U(t *testing.T) {
	consume := authsharedsqlc.ConsumeAuthTokenParams{TokenHash: onetimetoken.Hash("tok"), Purpose: authshared.PurposeResetPassword}

	testCases := []struct {
		name        string
		hashErr     error
		stub        func(q *MockQuerier)
		wantCode    apperror.Code
		wantPointer string
		wantNotice  bool
	}{
		{
			name: "a valid token: consumed, the new password set (email verified), every session ended, then the notice sent",
			stub: func(q *MockQuerier) {
				gomock.InOrder(
					q.EXPECT().ConsumeAuthToken(gomock.Any(), consume).Return(int64(7), nil),
					q.EXPECT().SetPasswordAfterReset(gomock.Any(), sqlc.SetPasswordAfterResetParams{ID: 7, PasswordHash: "hashed"}).
						Return("jane@example.com", nil),
					q.EXPECT().RevokeUserSessions(gomock.Any(), authsharedsqlc.RevokeUserSessionsParams{
						UserID: 7, Reason: authshared.RevokeReasonPasswordReset,
					}).Return(int64(2), nil),
				)
			},
			wantNotice: true,
		},
		{
			name: "an unknown, used or expired token: 422 invalid_token on the token, nothing changed",
			stub: func(q *MockQuerier) {
				q.EXPECT().ConsumeAuthToken(gomock.Any(), consume).Return(int64(0), sql.ErrNoRows)
			},
			wantCode: authshared.CodeInvalidToken, wantPointer: "/data/attributes/token",
		},
		{name: "a hashing error is a 500 before the token is touched", hashErr: errors.New("bcrypt"),
			stub: func(*MockQuerier) {}, wantCode: apperror.CodeInternal},
		{
			name: "a consume error is a 500",
			stub: func(q *MockQuerier) {
				q.EXPECT().ConsumeAuthToken(gomock.Any(), gomock.Any()).Return(int64(0), sql.ErrConnDone)
			},
			wantCode: apperror.CodeInternal,
		},
		{
			name: "a password write error is a 500 (the token stays unused: the transaction rolls back)",
			stub: func(q *MockQuerier) {
				q.EXPECT().ConsumeAuthToken(gomock.Any(), gomock.Any()).Return(int64(7), nil)
				q.EXPECT().SetPasswordAfterReset(gomock.Any(), gomock.Any()).Return("", sql.ErrConnDone)
			},
			wantCode: apperror.CodeInternal,
		},
		{
			name: "a session revoke error is a 500",
			stub: func(q *MockQuerier) {
				q.EXPECT().ConsumeAuthToken(gomock.Any(), gomock.Any()).Return(int64(7), nil)
				q.EXPECT().SetPasswordAfterReset(gomock.Any(), gomock.Any()).Return("jane@example.com", nil)
				q.EXPECT().RevokeUserSessions(gomock.Any(), gomock.Any()).Return(int64(0), sql.ErrConnDone)
			},
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
			svc := NewService(store, authshared.Emailer{Mailer: fake, Go: func(ctx context.Context, fn func(context.Context)) { fn(ctx) }})
			svc.(*service).hash = func(string) (string, error) { return "hashed", tc.hashErr }

			err := svc.Reset(context.Background(), Request{Token: "tok", Password: "N3w-Passw0rd"})
			if !tc.wantCode.IsZero() {
				var appErr *apperror.AppError
				require.ErrorAs(t, err, &appErr)
				require.Equal(t, tc.wantCode, appErr.Code)
				if tc.wantPointer != "" {
					require.Equal(t, authshared.InvalidTokenMessage, appErr.Message)
					require.Equal(t, tc.wantPointer, appErr.Source.Pointer)
				}
				require.Empty(t, fake.Sent(), "no notice when nothing changed")
				return
			}
			require.NoError(t, err)
			require.Equal(t, []mailer.Message{authshared.PasswordChangedEmail("jane@example.com")}, fake.Sent())
		})
	}
}
