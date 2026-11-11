package me

import (
	"context"
	"strconv"
	"testing"

	"github.com/hanifsyahsn/go_boilerplate_vs/internal/features/me/sqlc"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/testhelper"
	"github.com/stretchr/testify/require"
)

func TestService_Get_I(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	testCases := []struct {
		name string
		seed testhelper.SeedUserParams
		// verified marks the email verified after seeding.
		verified bool
	}{
		{name: "a user", seed: testhelper.SeedUserParams{Name: "Jane", Email: "jane@example.com", Role: "user"}},
		{name: "an admin", seed: testhelper.SeedUserParams{Name: "Root", Email: "root@example.com", Role: "admin"}},
		{name: "a verified email reads email_verified true", seed: testhelper.SeedUserParams{Name: "Jane", Email: "jane@example.com", Role: "user"},
			verified: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			defer testhelper.Truncate(t, testDB)
			userID := testhelper.SeedUser(t, testDB, tc.seed)
			if tc.verified {
				_, err := testDB.Exec(`UPDATE users SET email_verified_at = now() WHERE id = $1`, userID)
				require.NoError(t, err)
			}

			got, err := NewService(sqlc.New(testDB)).Get(context.Background(), userID)
			require.NoError(t, err)
			require.Equal(t, strconv.FormatInt(userID, 10), got.ID)
			require.Equal(t, tc.seed.Name, got.Name)
			require.Equal(t, tc.seed.Email, got.Email)
			require.Equal(t, tc.seed.Role, got.Role)
			require.Equal(t, tc.verified, got.EmailVerified)
		})
	}
}
