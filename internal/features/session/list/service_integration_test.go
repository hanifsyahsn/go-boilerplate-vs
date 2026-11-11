package list

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/features/session/list/sqlc"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/testhelper"
	"github.com/stretchr/testify/require"
)

func TestService_List_I(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	now := time.Now()
	hourAgo, dayAgo := now.Add(-time.Hour), now.Add(-24*time.Hour)
	inAnHour := now.Add(time.Hour)

	testCases := []struct {
		name string
		sort string
		// want is the expected order, by the name given to each seeded session.
		want []string
	}{
		{name: "only live sessions, most recently used first", sort: "-last_used_at", want: []string{"phone", "laptop"}},
		{name: "sorted by last use, oldest first", sort: "last_used_at", want: []string{"laptop", "phone"}},
		{name: "sorted by creation, newest first", sort: "-created_at", want: []string{"laptop", "phone"}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			defer testhelper.Truncate(t, testDB)
			me := testhelper.SeedUser(t, testDB, testhelper.SeedUserParams{})
			other := testhelper.SeedUser(t, testDB, testhelper.SeedUserParams{})

			names := map[uuid.UUID]string{}
			seed := func(name string, user int64, p testhelper.SeedSessionParams, token testhelper.SeedRefreshTokenParams) uuid.UUID {
				p.UserID, token.UserID = user, user
				family := testhelper.SeedSession(t, testDB, p)
				token.Jti, token.FamilyID = uuid.New(), family
				testhelper.SeedRefreshToken(t, testDB, token)
				names[family] = name
				return family
			}
			// laptop: trusted, created late, used long ago. phone: untrusted
			// (capped in an hour), created early, used recently.
			seed("laptop", me, testhelper.SeedSessionParams{Trusted: true, UserAgent: "Laptop", CreatedAt: hourAgo, LastUsedAt: dayAgo},
				testhelper.SeedRefreshTokenParams{})
			current := seed("phone", me, testhelper.SeedSessionParams{UserAgent: "Phone", CreatedAt: dayAgo, LastUsedAt: hourAgo},
				testhelper.SeedRefreshTokenParams{SessionExpiresAt: &inAnHour})
			// Not live: signed out; its only token expired; past its cap; someone else's.
			seed("revoked", me, testhelper.SeedSessionParams{RevokeReason: "logout"}, testhelper.SeedRefreshTokenParams{})
			seed("expired", me, testhelper.SeedSessionParams{}, testhelper.SeedRefreshTokenParams{ExpiresAt: hourAgo})
			seed("capped", me, testhelper.SeedSessionParams{}, testhelper.SeedRefreshTokenParams{SessionExpiresAt: &hourAgo})
			seed("other user", other, testhelper.SeedSessionParams{}, testhelper.SeedRefreshTokenParams{})

			got, err := NewService(sqlc.New(testDB)).List(context.Background(), me, current, tc.sort)
			require.NoError(t, err)

			var order []string
			for _, s := range got {
				order = append(order, names[uuid.MustParse(s.ID)])
				switch names[uuid.MustParse(s.ID)] {
				case "phone":
					require.True(t, s.Current)
					require.False(t, s.Trusted)
					require.NotNil(t, s.ExpiresAt)
					require.WithinDuration(t, inAnHour, *s.ExpiresAt, time.Millisecond)
				case "laptop":
					require.False(t, s.Current)
					require.True(t, s.Trusted)
					require.Nil(t, s.ExpiresAt, "a trusted session has no cap")
					require.Equal(t, "Laptop", s.UserAgent)
				}
			}
			require.Equal(t, tc.want, order)
		})
	}
}
