package detail

import (
	"context"
	"database/sql"
	"strconv"
	"testing"

	notesharedsqlc "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/note/shared/sqlc"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/apperror"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/testhelper"
	"github.com/stretchr/testify/require"
)

func TestService_Get_I(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	testCases := []struct {
		name     string
		arrange  func(t *testing.T, db *sql.DB, me, other int64) int64
		wantCode apperror.Code
	}{
		{
			name: "my live note",
			arrange: func(t *testing.T, db *sql.DB, me, other int64) int64 {
				return testhelper.SeedNote(t, db, testhelper.SeedNoteParams{Title: "Mine", CreatedBy: me})
			},
		},
		{
			name: "someone else's note is a 404",
			arrange: func(t *testing.T, db *sql.DB, me, other int64) int64 {
				return testhelper.SeedNote(t, db, testhelper.SeedNoteParams{CreatedBy: other})
			},
			wantCode: apperror.CodeNotFound,
		},
		{
			name: "a deleted note is a 404",
			arrange: func(t *testing.T, db *sql.DB, me, other int64) int64 {
				return testhelper.SeedNote(t, db, testhelper.SeedNoteParams{CreatedBy: me, Deleted: true})
			},
			wantCode: apperror.CodeNotFound,
		},
		{
			name:     "a missing note is a 404",
			arrange:  func(t *testing.T, db *sql.DB, me, other int64) int64 { return 999 },
			wantCode: apperror.CodeNotFound,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			defer testhelper.Truncate(t, testDB)
			me := testhelper.SeedUser(t, testDB, testhelper.SeedUserParams{Name: "Jane"})
			other := testhelper.SeedUser(t, testDB, testhelper.SeedUserParams{})
			id := tc.arrange(t, testDB, me, other)

			got, err := NewService(notesharedsqlc.New(testDB)).Get(context.Background(), id, me)
			if tc.wantCode != "" {
				var appErr *apperror.AppError
				require.ErrorAs(t, err, &appErr)
				require.Equal(t, tc.wantCode, appErr.Code)
				return
			}
			require.NoError(t, err)
			require.Equal(t, strconv.FormatInt(id, 10), got.ID)
			require.Equal(t, "Mine", got.Title)
			require.Equal(t, "Jane", got.CreatedBy.Name)
		})
	}
}
