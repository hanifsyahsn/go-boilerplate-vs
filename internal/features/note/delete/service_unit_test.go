package delete

import (
	"context"
	"database/sql"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/features/note/delete/sqlc"
	notesharedsqlc "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/note/shared/sqlc"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/apperror"
	"github.com/stretchr/testify/require"
)

func TestService_Delete_U(t *testing.T) {
	const (
		noteID = int64(5)
		userID = int64(7)
	)

	testCases := []struct {
		name     string
		stub     func(q *MockQuerier)
		wantCode apperror.Code
	}{
		{
			name: "locked, then soft-deleted by the caller",
			stub: func(q *MockQuerier) {
				gomock.InOrder(
					q.EXPECT().LockNoteForUpdate(gomock.Any(), noteID).Return(notesharedsqlc.LockNoteForUpdateRow{ID: noteID, CreatedBy: userID}, nil),
					q.EXPECT().SoftDeleteNote(gomock.Any(), sqlc.SoftDeleteNoteParams{ID: noteID, DeletedBy: sql.NullInt64{Int64: userID, Valid: true}}).Return(nil),
				)
			},
		},
		{
			name: "someone else's note is a 404",
			stub: func(q *MockQuerier) {
				q.EXPECT().LockNoteForUpdate(gomock.Any(), noteID).Return(notesharedsqlc.LockNoteForUpdateRow{ID: noteID, CreatedBy: 99}, nil)
			},
			wantCode: apperror.CodeNotFound,
		},
		{
			name: "missing / already deleted is a 404",
			stub: func(q *MockQuerier) {
				q.EXPECT().LockNoteForUpdate(gomock.Any(), noteID).Return(notesharedsqlc.LockNoteForUpdateRow{}, sql.ErrNoRows)
			},
			wantCode: apperror.CodeNotFound,
		},
		{
			name: "a delete error is a 500",
			stub: func(q *MockQuerier) {
				q.EXPECT().LockNoteForUpdate(gomock.Any(), noteID).Return(notesharedsqlc.LockNoteForUpdateRow{ID: noteID, CreatedBy: userID}, nil)
				q.EXPECT().SoftDeleteNote(gomock.Any(), gomock.Any()).Return(sql.ErrConnDone)
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

			err := NewService(store).Delete(context.Background(), noteID, userID)
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
