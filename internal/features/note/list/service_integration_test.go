package list

import (
	"context"
	"testing"

	"github.com/hanifsyahsn/go_boilerplate_vs/internal/features/note/list/sqlc"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/pagination"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/testhelper"
	"github.com/stretchr/testify/require"
)

func TestService_List_I(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	testCases := []struct {
		name       string
		req        Request
		wantTitles []string
		wantTotal  int64
	}{
		{name: "only the caller's live notes, newest first", req: Request{Sort: "-created_at"}, wantTitles: []string{"Ideas", "Groceries list"}, wantTotal: 2},
		{name: "oldest first", req: Request{Sort: "created_at"}, wantTitles: []string{"Groceries list", "Ideas"}, wantTotal: 2},
		{name: "by title", req: Request{Sort: "title"}, wantTitles: []string{"Groceries list", "Ideas"}, wantTotal: 2},
		{name: "by title, descending", req: Request{Sort: "-title"}, wantTitles: []string{"Ideas", "Groceries list"}, wantTotal: 2},
		{name: "search matches title or body, ignoring case", req: Request{Q: "MILK", Sort: "-created_at"}, wantTitles: []string{"Groceries list"}, wantTotal: 1},
		{name: "page 2 of size 1", req: Request{Sort: "-created_at", Request: pagination.Request{Page: 2, PageSize: 1}}, wantTitles: []string{"Groceries list"}, wantTotal: 2},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			defer testhelper.Truncate(t, testDB)
			me := testhelper.SeedUser(t, testDB, testhelper.SeedUserParams{})
			other := testhelper.SeedUser(t, testDB, testhelper.SeedUserParams{})

			testhelper.SeedNote(t, testDB, testhelper.SeedNoteParams{Title: "Groceries list", Body: "milk", CreatedBy: me})
			testhelper.SeedNote(t, testDB, testhelper.SeedNoteParams{Title: "Ideas", CreatedBy: me})
			testhelper.SeedNote(t, testDB, testhelper.SeedNoteParams{Title: "Deleted", CreatedBy: me, Deleted: true})
			testhelper.SeedNote(t, testDB, testhelper.SeedNoteParams{Title: "Someone else's milk", CreatedBy: other})

			notes, meta, err := NewService(sqlc.New(testDB)).List(context.Background(), tc.req, me)
			require.NoError(t, err)

			titles := make([]string, 0, len(notes))
			for _, n := range notes {
				titles = append(titles, n.Title)
			}
			require.Equal(t, tc.wantTitles, titles)
			require.Equal(t, tc.wantTotal, meta.Total)
		})
	}
}
