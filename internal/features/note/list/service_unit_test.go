package list

import (
	"context"
	"database/sql"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/features/note/list/sqlc"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/features/note/shared"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/apperror"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/pagination"
	"github.com/stretchr/testify/require"
)

func TestService_List_U(t *testing.T) {
	const userID = int64(7)
	row := sqlc.ListNotesRow{ID: 5, Title: "Groceries", CreatedBy: userID, CreatorName: "Jane"}

	testCases := []struct {
		name      string
		req       Request
		stub      func(q *sqlc.MockQuerier)
		wantNotes []shared.NoteResource
		wantMeta  pagination.Meta
		wantCode  apperror.Code
	}{
		{
			name: "trimmed search, page 2 of size 10",
			req:  Request{Q: "  groc ", Sort: "title", Request: pagination.Request{Page: 2, PageSize: 10}},
			stub: func(q *sqlc.MockQuerier) {
				search := sql.NullString{String: "groc", Valid: true}
				q.EXPECT().ListNotes(gomock.Any(), sqlc.ListNotesParams{UserID: userID, Q: search, Sort: "title", Limit: 10, Offset: 10}).
					Return([]sqlc.ListNotesRow{row}, nil)
				q.EXPECT().CountNotes(gomock.Any(), sqlc.CountNotesParams{UserID: userID, Q: search}).Return(int64(11), nil)
			},
			wantNotes: []shared.NoteResource{{ID: "5", Title: "Groceries", CreatedBy: shared.UserResource{ID: "7", Name: "Jane"}}},
			wantMeta:  pagination.Meta{Page: 2, PageSize: 10, Total: 11, TotalPage: 2},
		},
		{
			name: "no search: q is NULL, an empty page is a non-nil slice",
			req:  Request{Sort: "-created_at"},
			stub: func(q *sqlc.MockQuerier) {
				q.EXPECT().ListNotes(gomock.Any(), sqlc.ListNotesParams{UserID: userID, Sort: "-created_at", Limit: 20, Offset: 0}).
					Return([]sqlc.ListNotesRow{}, nil)
				q.EXPECT().CountNotes(gomock.Any(), sqlc.CountNotesParams{UserID: userID}).Return(int64(0), nil)
			},
			wantNotes: []shared.NoteResource{},
			wantMeta:  pagination.Meta{Page: 1, PageSize: 20},
		},
		{
			name: "a list error is a 500",
			stub: func(q *sqlc.MockQuerier) {
				q.EXPECT().ListNotes(gomock.Any(), gomock.Any()).Return(nil, sql.ErrConnDone)
			},
			wantCode: apperror.CodeInternal,
		},
		{
			name: "a count error is a 500",
			stub: func(q *sqlc.MockQuerier) {
				q.EXPECT().ListNotes(gomock.Any(), gomock.Any()).Return([]sqlc.ListNotesRow{}, nil)
				q.EXPECT().CountNotes(gomock.Any(), gomock.Any()).Return(int64(0), sql.ErrConnDone)
			},
			wantCode: apperror.CodeInternal,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			q := sqlc.NewMockQuerier(ctrl)
			tc.stub(q)

			notes, meta, err := NewService(q).List(context.Background(), tc.req, userID)
			if tc.wantCode != "" {
				var appErr *apperror.AppError
				require.ErrorAs(t, err, &appErr)
				require.Equal(t, tc.wantCode, appErr.Code)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.wantNotes, notes)
			require.Equal(t, tc.wantMeta, meta)
		})
	}
}
