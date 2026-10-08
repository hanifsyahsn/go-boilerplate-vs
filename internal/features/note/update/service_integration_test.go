package update

import (
	"context"
	"database/sql"
	"testing"

	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/apperror"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/testhelper"
	"github.com/stretchr/testify/require"
)

func TestService_Update_I(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	title, body := "New", "text"
	both := Request{Title: &title, Body: &body}

	testCases := []struct {
		name      string
		req       Request
		arrange   func(t *testing.T, db *sql.DB, me, other int64) int64
		wantCode  apperror.Code
		wantTitle string // the stored title afterwards
		wantBody  string // the stored body afterwards
	}{
		{
			name: "my note: both attributes changed",
			req:  both,
			arrange: func(t *testing.T, db *sql.DB, me, other int64) int64 {
				return testhelper.SeedNote(t, db, testhelper.SeedNoteParams{Title: "Old", Body: "old", CreatedBy: me})
			},
			wantTitle: "New",
			wantBody:  "text",
		},
		{
			name: "a partial update keeps what wasn't sent",
			req:  Request{Body: &body},
			arrange: func(t *testing.T, db *sql.DB, me, other int64) int64 {
				return testhelper.SeedNote(t, db, testhelper.SeedNoteParams{Title: "Old", Body: "old", CreatedBy: me})
			},
			wantTitle: "Old",
			wantBody:  "text",
		},
		{
			name: "someone else's note is a 404 and untouched",
			req:  both,
			arrange: func(t *testing.T, db *sql.DB, me, other int64) int64 {
				return testhelper.SeedNote(t, db, testhelper.SeedNoteParams{Title: "Old", Body: "old", CreatedBy: other})
			},
			wantCode:  apperror.CodeNotFound,
			wantTitle: "Old",
			wantBody:  "old",
		},
		{
			name: "a deleted note is a 404 and untouched",
			req:  both,
			arrange: func(t *testing.T, db *sql.DB, me, other int64) int64 {
				return testhelper.SeedNote(t, db, testhelper.SeedNoteParams{Title: "Old", Body: "old", CreatedBy: me, Deleted: true})
			},
			wantCode:  apperror.CodeNotFound,
			wantTitle: "Old",
			wantBody:  "old",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			defer testhelper.Truncate(t, testDB)
			me := testhelper.SeedUser(t, testDB, testhelper.SeedUserParams{})
			other := testhelper.SeedUser(t, testDB, testhelper.SeedUserParams{})
			id := tc.arrange(t, testDB, me, other)

			got, err := NewService(NewStore(testDB)).Update(context.Background(), id, tc.req, me)
			if tc.wantCode != "" {
				var appErr *apperror.AppError
				require.ErrorAs(t, err, &appErr)
				require.Equal(t, tc.wantCode, appErr.Code)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.wantTitle, got.Title)
				require.Equal(t, tc.wantBody, got.Body)
			}

			stored := testhelper.GetNote(t, testDB, id)
			require.Equal(t, tc.wantTitle, stored.Title)
			require.Equal(t, tc.wantBody, stored.Body)
		})
	}
}
