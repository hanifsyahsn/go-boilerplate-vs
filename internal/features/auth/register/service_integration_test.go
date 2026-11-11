package register

import (
	"context"
	"strconv"
	"testing"
	"time"

	authshared "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/shared"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/mailer"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/apperror"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/password"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/testhelper"
	"github.com/stretchr/testify/require"
)

func TestService_Register_I(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	testCases := []struct {
		name          string
		existingEmail string
		email         string
		wantCode      apperror.Code
	}{
		{name: "a new account", email: "jane@example.com"},
		{name: "a taken email is a 409", existingEmail: "jane@example.com", email: "jane@example.com", wantCode: apperror.CodeConflict},
		{name: "the email is unique ignoring case", existingEmail: "jane@example.com", email: "JANE@example.com", wantCode: apperror.CodeConflict},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			defer testhelper.Truncate(t, testDB)
			if tc.existingEmail != "" {
				testhelper.SeedUser(t, testDB, testhelper.SeedUserParams{Email: tc.existingEmail})
			}

			fake := &mailer.Fake{}
			svc := NewService(NewStore(testDB), authshared.Emailer{Mailer: fake, AppBaseURL: "https://app.example.com",
				Go: func(ctx context.Context, fn func(context.Context)) { fn(ctx) }})
			got, err := svc.Register(context.Background(), Request{Name: "Jane", Email: tc.email, Password: "Passw0rd!"})
			if !tc.wantCode.IsZero() {
				var appErr *apperror.AppError
				require.ErrorAs(t, err, &appErr)
				require.Equal(t, tc.wantCode, appErr.Code)
				require.Empty(t, fake.Sent(), "no verification email for a refused registration")
				return
			}
			require.NoError(t, err)
			require.Equal(t, "user", got.Role)

			id, err := strconv.ParseInt(got.ID, 10, 64)
			require.NoError(t, err)
			stored := testhelper.GetUser(t, testDB, id)
			require.True(t, stored.IsActive)
			require.True(t, password.Check(stored.PasswordHash, "Passw0rd!"))
			require.False(t, stored.EmailVerifiedAt.Valid, "not verified until the link is opened")
			tokens := testhelper.GetAuthTokens(t, testDB, id)
			require.Len(t, tokens, 1)
			require.Equal(t, authshared.PurposeVerifyEmail, tokens[0].Purpose)
			require.WithinDuration(t, time.Now().Add(24*time.Hour), tokens[0].ExpiresAt, 5*time.Second)
			require.Len(t, fake.Sent(), 1)
			require.Equal(t, "Verify your email", fake.Sent()[0].Subject)
		})
	}
}
