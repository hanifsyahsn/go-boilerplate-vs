package shared

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/features/note/shared/sqlc"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/apperror"
	"github.com/stretchr/testify/require"
)

func TestGetOwnedNote_U(t *testing.T) {
	owned := sqlc.GetNoteByIDRow{ID: 5, Title: "T", CreatedBy: 7}

	testCases := []struct {
		name     string
		row      sqlc.GetNoteByIDRow
		readErr  error
		wantCode apperror.Code
	}{
		{name: "the caller's own note", row: owned},
		{name: "someone else's note is a 404", row: sqlc.GetNoteByIDRow{ID: 5, CreatedBy: 8}, wantCode: apperror.CodeNotFound},
		{name: "missing / deleted is a 404", readErr: sql.ErrNoRows, wantCode: apperror.CodeNotFound},
		{name: "a read error is a 500", readErr: sql.ErrConnDone, wantCode: apperror.CodeInternal},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			q := sqlc.NewMockQuerier(ctrl)
			q.EXPECT().GetNoteByID(gomock.Any(), int64(5)).Return(tc.row, tc.readErr)

			got, err := GetOwnedNote(context.Background(), q, 5, 7, "test")
			if tc.wantCode != "" {
				var appErr *apperror.AppError
				require.ErrorAs(t, err, &appErr)
				require.Equal(t, tc.wantCode, appErr.Code)
				return
			}
			require.NoError(t, err)
			require.Equal(t, owned, got)
		})
	}
}

func TestLockOwnedNote_U(t *testing.T) {
	testCases := []struct {
		name     string
		row      sqlc.LockNoteForUpdateRow
		lockErr  error
		wantCode apperror.Code
	}{
		{name: "the caller's own note", row: sqlc.LockNoteForUpdateRow{ID: 5, CreatedBy: 7}},
		{name: "someone else's note is a 404", row: sqlc.LockNoteForUpdateRow{ID: 5, CreatedBy: 8}, wantCode: apperror.CodeNotFound},
		{name: "missing / deleted is a 404", lockErr: sql.ErrNoRows, wantCode: apperror.CodeNotFound},
		{name: "a lock error is a 500", lockErr: sql.ErrConnDone, wantCode: apperror.CodeInternal},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			q := sqlc.NewMockQuerier(ctrl)
			q.EXPECT().LockNoteForUpdate(gomock.Any(), int64(5)).Return(tc.row, tc.lockErr)

			err := LockOwnedNote(context.Background(), q, 5, 7, "test")
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

func TestNewNoteResource_U(t *testing.T) {
	now := time.Date(2026, 10, 7, 9, 0, 0, 0, time.UTC)

	testCases := []struct {
		name string
		row  sqlc.GetNoteByIDRow
		want NoteResource
	}{
		{
			name: "ids become strings, the creator a relationship",
			row: sqlc.GetNoteByIDRow{ID: 5, Title: "T", Body: "B", CreatedBy: 7, CreatedAt: now, UpdatedAt: now,
				CreatorName: "Jane", CreatorEmail: "jane@example.com"},
			want: NoteResource{ID: "5", Title: "T", Body: "B", CreatedAt: now, UpdatedAt: now,
				CreatedBy: UserResource{ID: "7", Name: "Jane", Email: "jane@example.com"}},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, NewNoteResource(FieldsFromRow(tc.row)))
		})
	}
}
