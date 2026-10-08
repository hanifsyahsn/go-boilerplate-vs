package delete

import (
	"context"
	"database/sql"
	"testing"

	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/apperror"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/testhelper"
	"github.com/stretchr/testify/require"
)

func TestService_Delete_I(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	testCases := []struct {
		name        string
		arrange     func(t *testing.T, db *sql.DB, me, other int64) int64
		wantCode    apperror.Code
		wantDeleted bool
	}{
		{
			name: "my note is soft-deleted, the row kept",
			arrange: func(t *testing.T, db *sql.DB, me, other int64) int64 {
				return testhelper.SeedNote(t, db, testhelper.SeedNoteParams{CreatedBy: me})
			},
			wantDeleted: true,
		},
		{
			name: "someone else's note is a 404 and kept",
			arrange: func(t *testing.T, db *sql.DB, me, other int64) int64 {
				return testhelper.SeedNote(t, db, testhelper.SeedNoteParams{CreatedBy: other})
			},
			wantCode: apperror.CodeNotFound,
		},
		{
			name: "an already deleted note is a 404",
			arrange: func(t *testing.T, db *sql.DB, me, other int64) int64 {
				return testhelper.SeedNote(t, db, testhelper.SeedNoteParams{CreatedBy: me, Deleted: true})
			},
			wantCode:    apperror.CodeNotFound,
			wantDeleted: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			defer testhelper.Truncate(t, testDB)
			me := testhelper.SeedUser(t, testDB, testhelper.SeedUserParams{})
			other := testhelper.SeedUser(t, testDB, testhelper.SeedUserParams{})
			id := tc.arrange(t, testDB, me, other)

			err := NewService(NewStore(testDB)).Delete(context.Background(), id, me)
			if tc.wantCode != "" {
				var appErr *apperror.AppError
				require.ErrorAs(t, err, &appErr)
				require.Equal(t, tc.wantCode, appErr.Code)
			} else {
				require.NoError(t, err)
			}

			stored := testhelper.GetNote(t, testDB, id)
			require.Equal(t, tc.wantDeleted, stored.DeletedAt.Valid)
			if tc.wantDeleted && tc.wantCode == "" {
				require.Equal(t, me, stored.DeletedBy.Int64)
			}
		})
	}
}
