package emailverificationrequest

import (
	"context"
	"testing"
	"time"

	"github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/emailverificationrequest/sqlc"
	authshared "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/shared"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/mailer"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/ratelimit"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/apperror"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/testhelper"
	"github.com/stretchr/testify/require"
)

func TestService_Request_I(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	testCases := []struct {
		name     string
		verified bool
		wantCode apperror.Code
	}{
		{name: "an unverified email: the earlier link spent, a new one stored and emailed"},
		{name: "a verified email is a 409 and nothing is issued", verified: true, wantCode: CodeEmailAlreadyVerified},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			defer testhelper.Truncate(t, testDB)
			userID := testhelper.SeedUser(t, testDB, testhelper.SeedUserParams{Email: "jane@example.com"})
			if tc.verified {
				_, err := testDB.Exec(`UPDATE users SET email_verified_at = now() WHERE id = $1`, userID)
				require.NoError(t, err)
			}
			testhelper.SeedAuthToken(t, testDB, testhelper.SeedAuthTokenParams{UserID: userID, Plain: "old", Purpose: authshared.PurposeVerifyEmail})
			fake := &mailer.Fake{}
			svc := NewService(sqlc.New(testDB), NewStore(testDB), ratelimit.NewMemory(ratelimit.Rate{Burst: 3, Per: time.Hour}),
				authshared.Emailer{Mailer: fake, AppBaseURL: "https://app.example.com", Go: func(ctx context.Context, fn func(context.Context)) { fn(ctx) }})

			err := svc.Request(context.Background(), userID)

			tokens := testhelper.GetAuthTokens(t, testDB, userID)
			if !tc.wantCode.IsZero() {
				var appErr *apperror.AppError
				require.ErrorAs(t, err, &appErr)
				require.Equal(t, tc.wantCode, appErr.Code)
				require.Len(t, tokens, 1)
				require.False(t, tokens[0].UsedAt.Valid)
				require.Empty(t, fake.Sent())
				return
			}
			require.NoError(t, err)
			require.Len(t, tokens, 2)
			require.True(t, tokens[0].UsedAt.Valid, "the earlier link stops working")
			require.False(t, tokens[1].UsedAt.Valid)
			require.Len(t, fake.Sent(), 1)
		})
	}
}
