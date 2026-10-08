package create

import (
	"context"
	"strconv"
	"testing"

	"github.com/hanifsyahsn/go_boilerplate_vs/internal/features/note/create/sqlc"
	notesharedsqlc "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/note/shared/sqlc"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/testhelper"
	"github.com/stretchr/testify/require"
)

func TestService_Create_I(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	testCases := []struct {
		name      string
		req       Request
		wantTitle string
		wantBody  string
	}{
		{name: "title and body stored", req: Request{Title: "Groceries", Body: "milk"}, wantTitle: "Groceries", wantBody: "milk"},
		{name: "title trimmed, empty body", req: Request{Title: "  Ideas  "}, wantTitle: "Ideas", wantBody: ""},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			defer testhelper.Truncate(t, testDB)
			userID := testhelper.SeedUser(t, testDB, testhelper.SeedUserParams{Name: "Jane"})

			svc := NewService(notesharedsqlc.New(testDB), sqlc.New(testDB))
			got, err := svc.Create(context.Background(), tc.req, userID)
			require.NoError(t, err)
			require.Equal(t, tc.wantTitle, got.Title)
			require.Equal(t, "Jane", got.CreatedBy.Name)

			id, err := strconv.ParseInt(got.ID, 10, 64)
			require.NoError(t, err)
			stored := testhelper.GetNote(t, testDB, id)
			require.Equal(t, tc.wantTitle, stored.Title)
			require.Equal(t, tc.wantBody, stored.Body)
			require.Equal(t, userID, stored.CreatedBy)
			require.False(t, stored.DeletedAt.Valid)
		})
	}
}
