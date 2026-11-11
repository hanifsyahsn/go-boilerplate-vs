package passwordreset

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	authshared "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/shared"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/mailer"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/apperror"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/password"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/testhelper"
	"github.com/stretchr/testify/require"
)

func TestService_Reset_I(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	const newPassword = "N3w-Passw0rd"
	hourAgo := time.Now().Add(-time.Hour)

	testCases := []struct {
		name string
		// token seeds the presented token for the user ("" = none seeded).
		token    testhelper.SeedAuthTokenParams
		wantCode apperror.Code
	}{
		{name: "a valid token: the new password works, the old doesn't, every session ended, the email verified, a notice sent",
			token: testhelper.SeedAuthTokenParams{Plain: "tok"}},
		{name: "a used token is a 422 and nothing changes",
			token: testhelper.SeedAuthTokenParams{Plain: "tok", Used: true}, wantCode: authshared.CodeInvalidToken},
		{name: "an expired token is a 422 and nothing changes",
			token: testhelper.SeedAuthTokenParams{Plain: "tok", ExpiresAt: hourAgo}, wantCode: authshared.CodeInvalidToken},
		{name: "a verification token can't reset a password",
			token: testhelper.SeedAuthTokenParams{Plain: "tok", Purpose: authshared.PurposeVerifyEmail}, wantCode: authshared.CodeInvalidToken},
		{name: "an unknown token is a 422", token: testhelper.SeedAuthTokenParams{Plain: "other"}, wantCode: authshared.CodeInvalidToken},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			defer testhelper.Truncate(t, testDB)
			userID := testhelper.SeedUser(t, testDB, testhelper.SeedUserParams{Email: "jane@example.com"})
			tc.token.UserID = userID
			testhelper.SeedAuthToken(t, testDB, tc.token)
			laptop := testhelper.SeedRefreshToken(t, testDB, testhelper.SeedRefreshTokenParams{Jti: uuid.New(), UserID: userID})
			phone := testhelper.SeedRefreshToken(t, testDB, testhelper.SeedRefreshTokenParams{Jti: uuid.New(), UserID: userID})
			fake := &mailer.Fake{}
			svc := NewService(NewStore(testDB), authshared.Emailer{Mailer: fake, Go: func(ctx context.Context, fn func(context.Context)) { fn(ctx) }})

			err := svc.Reset(context.Background(), Request{Token: "tok", Password: newPassword})

			user := testhelper.GetUser(t, testDB, userID)
			if !tc.wantCode.IsZero() {
				var appErr *apperror.AppError
				require.ErrorAs(t, err, &appErr)
				require.Equal(t, tc.wantCode, appErr.Code)
				require.True(t, password.Check(user.PasswordHash, testhelper.DefaultPassword), "the old password still works")
				require.False(t, user.EmailVerifiedAt.Valid)
				require.False(t, testhelper.GetSession(t, testDB, laptop).RevokedAt.Valid, "no session ended")
				require.Empty(t, fake.Sent())
				return
			}
			require.NoError(t, err)
			require.True(t, password.Check(user.PasswordHash, newPassword))
			require.False(t, password.Check(user.PasswordHash, testhelper.DefaultPassword))
			require.True(t, user.EmailVerifiedAt.Valid, "the link proved the address")
			for _, family := range []uuid.UUID{laptop, phone} {
				session := testhelper.GetSession(t, testDB, family)
				require.Equal(t, authshared.RevokeReasonPasswordReset, session.RevokeReason.String)
			}
			require.Equal(t, []mailer.Message{authshared.PasswordChangedEmail("jane@example.com")}, fake.Sent())
		})
	}
}

// TestService_Reset_ParallelOneToken_I: two resets racing with one link —
// exactly one wins, the other is a 422.
func TestService_Reset_ParallelOneToken_I(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	testCases := []struct {
		name     string
		parallel int
	}{
		{name: "two parallel resets with one token: exactly one 204", parallel: 2},
		{name: "eight parallel resets with one token: exactly one 204", parallel: 8},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			defer testhelper.Truncate(t, testDB)
			userID := testhelper.SeedUser(t, testDB, testhelper.SeedUserParams{})
			testhelper.SeedAuthToken(t, testDB, testhelper.SeedAuthTokenParams{UserID: userID, Plain: "tok"})
			svc := NewService(NewStore(testDB), authshared.Emailer{Mailer: &mailer.Fake{}, Go: func(ctx context.Context, fn func(context.Context)) { fn(ctx) }})

			var wg sync.WaitGroup
			errs := make([]error, tc.parallel)
			for i := range tc.parallel {
				wg.Add(1)
				go func() {
					defer wg.Done()
					errs[i] = svc.Reset(context.Background(), Request{Token: "tok", Password: "N3w-Passw0rd"})
				}()
			}
			wg.Wait()

			ok, invalid := 0, 0
			for _, err := range errs {
				var appErr *apperror.AppError
				switch {
				case err == nil:
					ok++
				case errors.As(err, &appErr) && appErr.Code == authshared.CodeInvalidToken:
					invalid++
				default:
					t.Fatalf("unexpected error: %v", err)
				}
			}
			require.Equal(t, 1, ok)
			require.Equal(t, tc.parallel-1, invalid)
		})
	}
}
