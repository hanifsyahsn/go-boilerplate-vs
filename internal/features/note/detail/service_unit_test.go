package detail

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/features/note/shared"
	notesharedsqlc "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/note/shared/sqlc"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/apperror"
	"github.com/stretchr/testify/require"
)

func TestService_Get_U(t *testing.T) {
	const (
		noteID = int64(5)
		userID = int64(7)
	)
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	row := notesharedsqlc.GetNoteByIDRow{
		ID: noteID, Title: "Mine", Body: "text", CreatedBy: userID,
		CreatedAt: now, UpdatedAt: now, CreatorName: "Jane", CreatorEmail: "jane@example.com",
	}

	testCases := []struct {
		name     string
		stub     func(q *notesharedsqlc.MockQuerier)
		want     shared.NoteResource
		wantCode apperror.Code
	}{
		{
			name: "my note is returned as a resource",
			stub: func(q *notesharedsqlc.MockQuerier) {
				q.EXPECT().GetNoteByID(gomock.Any(), noteID).Return(row, nil)
			},
			want: shared.NewNoteResource(shared.FieldsFromRow(row)),
		},
		{
			name: "someone else's note is a 404",
			stub: func(q *notesharedsqlc.MockQuerier) {
				other := row
				other.CreatedBy = 99
				q.EXPECT().GetNoteByID(gomock.Any(), noteID).Return(other, nil)
			},
			wantCode: apperror.CodeNotFound,
		},
		{
			name: "a missing (or deleted) note is a 404",
			stub: func(q *notesharedsqlc.MockQuerier) {
				q.EXPECT().GetNoteByID(gomock.Any(), noteID).Return(notesharedsqlc.GetNoteByIDRow{}, sql.ErrNoRows)
			},
			wantCode: apperror.CodeNotFound,
		},
		{
			name: "a database error is a 500",
			stub: func(q *notesharedsqlc.MockQuerier) {
				q.EXPECT().GetNoteByID(gomock.Any(), noteID).Return(notesharedsqlc.GetNoteByIDRow{}, sql.ErrConnDone)
			},
			wantCode: apperror.CodeInternal,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			q := notesharedsqlc.NewMockQuerier(ctrl)
			tc.stub(q)

			got, err := NewService(q).Get(context.Background(), noteID, userID)
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
