package revoke

import (
	"context"
	"database/sql"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	authshared "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/shared"
	authsharedsqlc "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/shared/sqlc"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/features/session/revoke/sqlc"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/features/session/shared"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/apperror"
	"github.com/stretchr/testify/require"
)

func TestService_Revoke_U(t *testing.T) {
	family := uuid.New()
	lock := sqlc.LockLiveSessionParams{FamilyID: family, UserID: 7}

	testCases := []struct {
		name     string
		id       string
		stub     func(q *MockQuerier)
		wantCode apperror.Code
		wantMsg  string
	}{
		{
			name: "the caller's live session: locked, then revoked as signed_out_remotely",
			id:   family.String(),
			stub: func(q *MockQuerier) {
				gomock.InOrder(
					q.EXPECT().LockLiveSession(gomock.Any(), lock).Return(family, nil),
					q.EXPECT().RevokeSession(gomock.Any(), authsharedsqlc.RevokeSessionParams{
						FamilyID: family, Reason: authshared.RevokeReasonSignedOutRemotely,
					}).Return(int64(1), nil),
				)
			},
		},
		{
			name: "someone else's, a revoked or an expired session is a 404 and nothing is revoked",
			id:   family.String(),
			stub: func(q *MockQuerier) {
				q.EXPECT().LockLiveSession(gomock.Any(), lock).Return(uuid.Nil, sql.ErrNoRows)
			},
			wantCode: apperror.CodeNotFound, wantMsg: shared.NotFoundMessage,
		},
		{
			name: "an id that isn't a UUID is a 404 without touching the DB", id: "not-a-uuid",
			stub:     func(*MockQuerier) {},
			wantCode: apperror.CodeNotFound, wantMsg: shared.NotFoundMessage,
		},
		{
			name: "a lock error is a 500",
			id:   family.String(),
			stub: func(q *MockQuerier) {
				q.EXPECT().LockLiveSession(gomock.Any(), lock).Return(uuid.Nil, sql.ErrConnDone)
			},
			wantCode: apperror.CodeInternal,
		},
		{
			name: "a revoke error is a 500",
			id:   family.String(),
			stub: func(q *MockQuerier) {
				q.EXPECT().LockLiveSession(gomock.Any(), lock).Return(family, nil)
				q.EXPECT().RevokeSession(gomock.Any(), gomock.Any()).Return(int64(0), sql.ErrConnDone)
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

			err := NewService(store).Revoke(context.Background(), 7, tc.id)
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
		})
	}
}
