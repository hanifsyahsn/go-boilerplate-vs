package passwordchange

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/passwordchange/sqlc"
	authshared "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/shared"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/mailer"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/ratelimit"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/apperror"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/password"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/testhelper"
	"github.com/stretchr/testify/require"
)

func TestService_Change_I(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	testCases := []struct {
		name     string
		current  string
		wantCode apperror.Code
	}{
		{name: "the right current password: the new one works, the other devices are signed out, this one stays", current: testhelper.DefaultPassword},
		{name: "a wrong current password: 422 and nothing changes", current: "nope", wantCode: CodeWrongPassword},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			defer testhelper.Truncate(t, testDB)
			userID := testhelper.SeedUser(t, testDB, testhelper.SeedUserParams{Email: "jane@example.com"})
			mine := testhelper.SeedRefreshToken(t, testDB, testhelper.SeedRefreshTokenParams{Jti: uuid.New(), UserID: userID})
			other := testhelper.SeedRefreshToken(t, testDB, testhelper.SeedRefreshTokenParams{Jti: uuid.New(), UserID: userID})
			fake := &mailer.Fake{}
			svc := NewService(sqlc.New(testDB), NewStore(testDB), ratelimit.NewMemory(ratelimit.Rate{Burst: 5, Per: 15 * time.Minute}),
				authshared.Emailer{Mailer: fake, Go: func(ctx context.Context, fn func(context.Context)) { fn(ctx) }})

			err := svc.Change(context.Background(), userID, mine, Request{CurrentPassword: tc.current, Password: "N3w-Passw0rd"})

			user := testhelper.GetUser(t, testDB, userID)
			if !tc.wantCode.IsZero() {
				var appErr *apperror.AppError
				require.ErrorAs(t, err, &appErr)
				require.Equal(t, tc.wantCode, appErr.Code)
				require.True(t, password.Check(user.PasswordHash, testhelper.DefaultPassword))
				require.False(t, testhelper.GetSession(t, testDB, other).RevokedAt.Valid)
				require.Empty(t, fake.Sent())
				return
			}
			require.NoError(t, err)
			require.True(t, password.Check(user.PasswordHash, "N3w-Passw0rd"))
			require.False(t, testhelper.GetSession(t, testDB, mine).RevokedAt.Valid, "the caller stays signed in")
			require.Equal(t, authshared.RevokeReasonPasswordChanged, testhelper.GetSession(t, testDB, other).RevokeReason.String)
			require.Equal(t, []mailer.Message{authshared.PasswordChangedEmail("jane@example.com")}, fake.Sent())
		})
	}
}
