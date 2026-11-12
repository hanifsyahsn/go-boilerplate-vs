package passwordresetrequest

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/passwordresetrequest/sqlc"
	authshared "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/shared"
	authsharedsqlc "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/shared/sqlc"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/mailer"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/ratelimit"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/onetimetoken"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/testhelper"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/utility"
	"github.com/stretchr/testify/require"
)

func TestService_Request_I(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	testCases := []struct {
		name string
		run  func(t *testing.T, svc S, fake *mailer.Fake, userID int64)
	}{
		{name: "a known user gets one email whose link carries a token ConsumeAuthToken accepts", run: func(t *testing.T, svc S, fake *mailer.Fake, userID int64) {
			svc.Request(context.Background(), Request{Email: "JANE@example.com"})
			sent := fake.Sent()
			require.Len(t, sent, 1)
			plain := sent[0].Text[strings.Index(sent[0].Text, "token=")+len("token="):]
			plain = plain[:strings.Index(plain, "\n")]

			got, err := authsharedsqlc.New(testDB).ConsumeAuthToken(context.Background(), authsharedsqlc.ConsumeAuthTokenParams{
				TokenHash: onetimetoken.Hash(plain), Purpose: authshared.PurposeResetPassword,
			})
			require.NoError(t, err)
			require.Equal(t, userID, got)
			tokens := testhelper.GetAuthTokens(t, testDB, userID)
			require.Len(t, tokens, 1)
			require.WithinDuration(t, time.Now().Add(time.Hour), tokens[0].ExpiresAt, 5*time.Second, "a reset link lives 1 h")
		}},
		{name: "a second request spends the first token", run: func(t *testing.T, svc S, _ *mailer.Fake, userID int64) {
			svc.Request(context.Background(), Request{Email: "jane@example.com"})
			svc.Request(context.Background(), Request{Email: "jane@example.com"})
			tokens := testhelper.GetAuthTokens(t, testDB, userID)
			require.Len(t, tokens, 2)
			require.True(t, tokens[0].UsedAt.Valid, "the first link no longer works")
			require.False(t, tokens[1].UsedAt.Valid)
		}},
		{name: "the 4th request in an hour sends nothing", run: func(t *testing.T, svc S, fake *mailer.Fake, userID int64) {
			for range 4 {
				svc.Request(context.Background(), Request{Email: "jane@example.com"})
			}
			require.Len(t, fake.Sent(), 3)
			require.Len(t, testhelper.GetAuthTokens(t, testDB, userID), 3)
		}},
		{name: "an unknown email: no token, no email", run: func(t *testing.T, svc S, fake *mailer.Fake, userID int64) {
			svc.Request(context.Background(), Request{Email: "nobody@example.com"})
			require.Empty(t, fake.Sent())
			require.Empty(t, testhelper.GetAuthTokens(t, testDB, userID))
		}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			defer testhelper.Truncate(t, testDB)
			userID := testhelper.SeedUser(t, testDB, testhelper.SeedUserParams{Email: "jane@example.com"})
			fake := &mailer.Fake{}
			emailer := authshared.Emailer{Mailer: fake, AppBaseURL: "https://app.example.com",
				Go: func(ctx context.Context, fn func(context.Context)) { fn(ctx) }}
			svc := NewService(sqlc.New(testDB), NewStore(testDB), ratelimit.NewMemory(ratelimit.Rate{Burst: 3, Per: time.Hour}), emailer)
			tc.run(t, svc, fake, userID)
		})
	}
}

// TestService_Request_BackgroundGroup_I: with main's real background group,
// the request returns before the work, and waiting for the group (what main
// does at shutdown, after the server drained) finishes it: the token is
// stored and the email sent.
func TestService_Request_BackgroundGroup_I(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	testCases := []struct {
		name string
	}{
		{name: "the work runs in the background group and is done once the group is waited for (shutdown)"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			defer testhelper.Truncate(t, testDB)
			userID := testhelper.SeedUser(t, testDB, testhelper.SeedUserParams{Email: "jane@example.com"})
			fake := &mailer.Fake{}
			var bg utility.Group
			emailer := authshared.Emailer{Mailer: fake, AppBaseURL: "https://app.example.com", Go: bg.Go}
			svc := NewService(sqlc.New(testDB), NewStore(testDB), ratelimit.NewMemory(ratelimit.Rate{Burst: 3, Per: time.Hour}), emailer)

			svc.Request(context.Background(), Request{Email: "jane@example.com"})
			bg.Wait()

			require.Len(t, testhelper.GetAuthTokens(t, testDB, userID), 1)
			require.Len(t, fake.Sent(), 1)
		})
	}
}
