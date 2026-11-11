package emailverification

import (
	"context"
	"database/sql"
	"testing"

	"github.com/golang/mock/gomock"
	authshared "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/shared"
	authsharedsqlc "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/shared/sqlc"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/apperror"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/onetimetoken"
	"github.com/stretchr/testify/require"
)

func TestService_Verify_U(t *testing.T) {
	consume := authsharedsqlc.ConsumeAuthTokenParams{TokenHash: onetimetoken.Hash("tok"), Purpose: authshared.PurposeVerifyEmail}

	testCases := []struct {
		name        string
		stub        func(q *MockQuerier)
		wantCode    apperror.Code
		wantPointer string
	}{
		{
			name: "a valid token: consumed, then the user's email marked verified",
			stub: func(q *MockQuerier) {
				gomock.InOrder(
					q.EXPECT().ConsumeAuthToken(gomock.Any(), consume).Return(int64(7), nil),
					q.EXPECT().MarkEmailVerified(gomock.Any(), int64(7)).Return(nil),
				)
			},
		},
		{
			name: "an unknown, used or expired token: the same 422 invalid_token as a reset, on the token",
			stub: func(q *MockQuerier) {
				q.EXPECT().ConsumeAuthToken(gomock.Any(), consume).Return(int64(0), sql.ErrNoRows)
			},
			wantCode: authshared.CodeInvalidToken, wantPointer: "/data/attributes/token",
		},
		{
			name: "a consume error is a 500",
			stub: func(q *MockQuerier) {
				q.EXPECT().ConsumeAuthToken(gomock.Any(), gomock.Any()).Return(int64(0), sql.ErrConnDone)
			},
			wantCode: apperror.CodeInternal,
		},
		{
			name: "a write error is a 500 (the token stays unused: the transaction rolls back)",
			stub: func(q *MockQuerier) {
				q.EXPECT().ConsumeAuthToken(gomock.Any(), gomock.Any()).Return(int64(7), nil)
				q.EXPECT().MarkEmailVerified(gomock.Any(), int64(7)).Return(sql.ErrConnDone)
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
				func(ctx context.Context, fn func(Querier) error) error { return fn(q) })

			err := NewService(store).Verify(context.Background(), Request{Token: "tok"})
			if !tc.wantCode.IsZero() {
				var appErr *apperror.AppError
				require.ErrorAs(t, err, &appErr)
				require.Equal(t, tc.wantCode, appErr.Code)
				if tc.wantPointer != "" {
					require.Equal(t, authshared.InvalidTokenMessage, appErr.Message)
					require.Equal(t, tc.wantPointer, appErr.Source.Pointer)
				}
				return
			}
			require.NoError(t, err)
		})
	}
}
