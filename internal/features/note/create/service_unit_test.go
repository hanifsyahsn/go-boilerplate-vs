package create

import (
	"context"
	"database/sql"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/features/note/create/sqlc"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/features/note/shared"
	notesharedsqlc "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/note/shared/sqlc"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/apperror"
	"github.com/stretchr/testify/require"
)

func TestService_Create_U(t *testing.T) {
	const userID = int64(7)
	row := notesharedsqlc.GetNoteByIDRow{ID: 5, Title: "Groceries", Body: "milk", CreatedBy: userID, CreatorName: "Jane"}

	testCases := []struct {
		name     string
		req      Request
		stub     func(q *sqlc.MockQuerier, sq *notesharedsqlc.MockQuerier)
		want     shared.NoteResource
		wantCode apperror.Code
	}{
		{
			name: "stored with a trimmed title, then read back",
			req:  Request{Title: "  Groceries ", Body: "milk"},
			stub: func(q *sqlc.MockQuerier, sq *notesharedsqlc.MockQuerier) {
				q.EXPECT().CreateNote(gomock.Any(), sqlc.CreateNoteParams{Title: "Groceries", Body: "milk", CreatedBy: userID}).Return(int64(5), nil)
				sq.EXPECT().GetNoteByID(gomock.Any(), int64(5)).Return(row, nil)
			},
			want: shared.NewNoteResource(shared.FieldsFromRow(row)),
		},
		{
			name: "an insert error is a 500",
			req:  Request{Title: "Groceries"},
			stub: func(q *sqlc.MockQuerier, sq *notesharedsqlc.MockQuerier) {
				q.EXPECT().CreateNote(gomock.Any(), gomock.Any()).Return(int64(0), sql.ErrConnDone)
			},
			wantCode: apperror.CodeInternal,
		},
		{
			name: "a read-back error is a 500",
			req:  Request{Title: "Groceries"},
			stub: func(q *sqlc.MockQuerier, sq *notesharedsqlc.MockQuerier) {
				q.EXPECT().CreateNote(gomock.Any(), gomock.Any()).Return(int64(5), nil)
				sq.EXPECT().GetNoteByID(gomock.Any(), int64(5)).Return(notesharedsqlc.GetNoteByIDRow{}, sql.ErrConnDone)
			},
			wantCode: apperror.CodeInternal,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			q := sqlc.NewMockQuerier(ctrl)
			sq := notesharedsqlc.NewMockQuerier(ctrl)
			tc.stub(q, sq)

			got, err := NewService(sq, q).Create(context.Background(), tc.req, userID)
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
