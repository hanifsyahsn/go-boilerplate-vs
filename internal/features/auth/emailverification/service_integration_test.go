package emailverification

import (
	"context"
	"testing"
	"time"

	authshared "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/shared"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/apperror"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/testhelper"
	"github.com/stretchr/testify/require"
)

func TestService_Verify_I(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	testCases := []struct {
		name     string
		token    testhelper.SeedAuthTokenParams
		wantCode apperror.Code
	}{
		{name: "a valid verification token: the email is verified and the token spent",
			token: testhelper.SeedAuthTokenParams{Plain: "tok", Purpose: authshared.PurposeVerifyEmail, ExpiresAt: time.Now().Add(24 * time.Hour)}},
		{name: "a reset token can't verify an email",
			token: testhelper.SeedAuthTokenParams{Plain: "tok", Purpose: authshared.PurposeResetPassword}, wantCode: authshared.CodeInvalidToken},
		{name: "an expired token is a 422",
			token:    testhelper.SeedAuthTokenParams{Plain: "tok", Purpose: authshared.PurposeVerifyEmail, ExpiresAt: time.Now().Add(-time.Minute)},
			wantCode: authshared.CodeInvalidToken},
		{name: "a used token is a 422",
			token:    testhelper.SeedAuthTokenParams{Plain: "tok", Purpose: authshared.PurposeVerifyEmail, Used: true},
			wantCode: authshared.CodeInvalidToken},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			defer testhelper.Truncate(t, testDB)
			userID := testhelper.SeedUser(t, testDB, testhelper.SeedUserParams{})
			tc.token.UserID = userID
			testhelper.SeedAuthToken(t, testDB, tc.token)

			err := NewService(NewStore(testDB)).Verify(context.Background(), Request{Token: "tok"})

			verified := testhelper.GetUser(t, testDB, userID).EmailVerifiedAt.Valid
			if !tc.wantCode.IsZero() {
				var appErr *apperror.AppError
				require.ErrorAs(t, err, &appErr)
				require.Equal(t, tc.wantCode, appErr.Code)
				require.False(t, verified)
				return
			}
			require.NoError(t, err)
			require.True(t, verified)
			require.True(t, testhelper.GetAuthTokens(t, testDB, userID)[0].UsedAt.Valid, "the link works once")
		})
	}
}
