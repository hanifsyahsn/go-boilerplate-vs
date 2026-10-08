package update

import (
	"context"
	"database/sql"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/features/note/shared"
	notesharedsqlc "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/note/shared/sqlc"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/features/note/update/sqlc"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/apperror"
	"github.com/stretchr/testify/require"
)

func TestService_Update_U(t *testing.T) {
	const (
		noteID = int64(5)
		userID = int64(7)
	)
	row := notesharedsqlc.GetNoteByIDRow{ID: noteID, Title: "New", Body: "text", CreatedBy: userID, CreatorName: "Jane"}
	title, body, empty := "  New ", "text", ""
	both := Request{Title: &title, Body: &body}
	owned := notesharedsqlc.LockNoteForUpdateRow{ID: noteID, CreatedBy: userID}

	testCases := []struct {
		name     string
		req      Request
		stub     func(q *MockQuerier)
		want     shared.NoteResource
		wantCode apperror.Code
	}{
		{
			name: "locked, both attributes set (title trimmed), read back",
			req:  both,
			stub: func(q *MockQuerier) {
				gomock.InOrder(
					q.EXPECT().LockNoteForUpdate(gomock.Any(), noteID).Return(owned, nil),
					q.EXPECT().UpdateNote(gomock.Any(), sqlc.UpdateNoteParams{
						ID:    noteID,
						Title: sql.NullString{String: "New", Valid: true},
						Body:  sql.NullString{String: "text", Valid: true},
					}).Return(nil),
					q.EXPECT().GetNoteByID(gomock.Any(), noteID).Return(row, nil),
				)
			},
			want: shared.NewNoteResource(shared.FieldsFromRow(row)),
		},
		{
			name: "a partial update: the title is left NULL (kept), an empty body is set",
			req:  Request{Body: &empty},
			stub: func(q *MockQuerier) {
				gomock.InOrder(
					q.EXPECT().LockNoteForUpdate(gomock.Any(), noteID).Return(owned, nil),
					q.EXPECT().UpdateNote(gomock.Any(), sqlc.UpdateNoteParams{ID: noteID, Body: sql.NullString{Valid: true}}).Return(nil),
					q.EXPECT().GetNoteByID(gomock.Any(), noteID).Return(row, nil),
				)
			},
			want: shared.NewNoteResource(shared.FieldsFromRow(row)),
		},
		{
			name: "someone else's note is a 404, nothing written",
			req:  both,
			stub: func(q *MockQuerier) {
				q.EXPECT().LockNoteForUpdate(gomock.Any(), noteID).Return(notesharedsqlc.LockNoteForUpdateRow{ID: noteID, CreatedBy: 99}, nil)
			},
			wantCode: apperror.CodeNotFound,
		},
		{
			name: "a missing note is a 404",
			req:  both,
			stub: func(q *MockQuerier) {
				q.EXPECT().LockNoteForUpdate(gomock.Any(), noteID).Return(notesharedsqlc.LockNoteForUpdateRow{}, sql.ErrNoRows)
			},
			wantCode: apperror.CodeNotFound,
		},
		{
			name: "an update error is a 500",
			req:  both,
			stub: func(q *MockQuerier) {
				q.EXPECT().LockNoteForUpdate(gomock.Any(), noteID).Return(notesharedsqlc.LockNoteForUpdateRow{ID: noteID, CreatedBy: userID}, nil)
				q.EXPECT().UpdateNote(gomock.Any(), gomock.Any()).Return(sql.ErrConnDone)
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

			got, err := NewService(store).Update(context.Background(), noteID, tc.req, userID)
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
