package user

import (
	"context"
	"testing"

	"github.com/hanifsyahsn/go_boilerplate_vs/internal/features/admin/user/sqlc"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/testhelper"
	"github.com/stretchr/testify/require"
)

func TestService_List_I(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	active, inactive := true, false

	testCases := []struct {
		name      string
		req       ListRequest
		wantNames []string
	}{
		{name: "everyone, newest first", req: ListRequest{Sort: "-created_at"}, wantNames: []string{"Old Bob", "Jane Doe", "Admin"}},
		{name: "by name", req: ListRequest{Sort: "name"}, wantNames: []string{"Admin", "Jane Doe", "Old Bob"}},
		{name: "by email, descending", req: ListRequest{Sort: "-email"}, wantNames: []string{"Jane Doe", "Old Bob", "Admin"}},
		{name: "search on email", req: ListRequest{Q: "JANE@", Sort: "-created_at"}, wantNames: []string{"Jane Doe"}},
		{name: "only inactive", req: ListRequest{IsActive: &inactive, Sort: "-created_at"}, wantNames: []string{"Old Bob"}},
		{name: "only active", req: ListRequest{IsActive: &active, Sort: "-created_at"}, wantNames: []string{"Jane Doe", "Admin"}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			defer testhelper.Truncate(t, testDB)
			testhelper.SeedUser(t, testDB, testhelper.SeedUserParams{Name: "Admin", Email: "admin@example.com", Role: "admin"})
			testhelper.SeedUser(t, testDB, testhelper.SeedUserParams{Name: "Jane Doe", Email: "jane@example.com"})
			testhelper.SeedUser(t, testDB, testhelper.SeedUserParams{Name: "Old Bob", Email: "bob@example.com", IsActive: testhelper.BoolPtr(false)})

			items, meta, err := NewService(sqlc.New(testDB)).List(context.Background(), tc.req)
			require.NoError(t, err)

			names := make([]string, 0, len(items))
			for _, it := range items {
				names = append(names, it.Name)
			}
			require.Equal(t, tc.wantNames, names)
			require.Equal(t, int64(len(tc.wantNames)), meta.Total)
		})
	}
}

func TestService_Update_I(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	testCases := []struct {
		name     string
		before   bool  // the target's is_active before
		isActive *bool // the PATCH attribute
		want     bool
	}{
		{name: "deactivate", before: true, isActive: testhelper.BoolPtr(false), want: false},
		{name: "activate", before: false, isActive: testhelper.BoolPtr(true), want: true},
		{name: "no attribute: unchanged", before: false, isActive: nil, want: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			defer testhelper.Truncate(t, testDB)
			admin := testhelper.SeedUser(t, testDB, testhelper.SeedUserParams{Role: "admin"})
			target := testhelper.SeedUser(t, testDB, testhelper.SeedUserParams{IsActive: testhelper.BoolPtr(tc.before)})

			got, err := NewService(sqlc.New(testDB)).Update(context.Background(), target, UpdateRequest{IsActive: tc.isActive}, admin)
			require.NoError(t, err)
			require.Equal(t, tc.want, got.IsActive)
			require.Equal(t, tc.want, testhelper.GetUser(t, testDB, target).IsActive)
		})
	}
}
