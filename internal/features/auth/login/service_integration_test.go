package login

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"strings"
	"testing"
	"time"

	"github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/login/sqlc"
	authsharedsqlc "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/shared/sqlc"
	platformconstant "github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/constant"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/jwttoken"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/ratelimit"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/apperror"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/password"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/testhelper"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

func TestService_Login_I(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	maker := jwttoken.NewTokenMakerES256(key, &key.PublicKey, "client", "issuer")

	const untrustedTTL = 30 * time.Minute

	testCases := []struct {
		name     string
		seed     testhelper.SeedUserParams
		req      Request
		wantCode apperror.Code
	}{
		{
			name: "matching credentials, email case ignored, untrusted device: the session capped",
			seed: testhelper.SeedUserParams{Email: "jane@example.com", Role: "admin"},
			req:  Request{Email: "JANE@example.com", Password: testhelper.DefaultPassword},
		},
		{
			name: "trusted device: no cap",
			seed: testhelper.SeedUserParams{Email: "jane@example.com", Role: "admin"},
			req:  Request{Email: "jane@example.com", Password: testhelper.DefaultPassword, TrustDevice: true},
		},
		{
			name:     "wrong password",
			seed:     testhelper.SeedUserParams{Email: "jane@example.com"},
			req:      Request{Email: "jane@example.com", Password: "wrong"},
			wantCode: apperror.CodeUnauthorized,
		},
		{
			name:     "deactivated",
			seed:     testhelper.SeedUserParams{Email: "jane@example.com", IsActive: testhelper.BoolPtr(false)},
			req:      Request{Email: "jane@example.com", Password: testhelper.DefaultPassword},
			wantCode: apperror.CodeUnauthorized,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			defer testhelper.Truncate(t, testDB)
			userID := testhelper.SeedUser(t, testDB, tc.seed)

			got, err := NewService(sqlc.New(testDB), authsharedsqlc.New(testDB), maker, time.Minute, time.Hour, untrustedTTL,
				Limiters{
					Email: ratelimit.NewMemory(ratelimit.Rate{Burst: 5, Per: time.Minute}),
					IP:    ratelimit.NewMemory(ratelimit.Rate{Burst: 5, Per: time.Minute}),
				}).Login(context.Background(), tc.req, Client{IP: "203.0.113.7", UserAgent: "Mozilla/5.0 (test)"})
			if !tc.wantCode.IsZero() {
				var appErr *apperror.AppError
				require.ErrorAs(t, err, &appErr)
				require.Equal(t, tc.wantCode, appErr.Code)
				return
			}
			require.NoError(t, err)

			claims, err := maker.VerifyAccessToken(got.Tokens.AccessToken)
			require.NoError(t, err)
			require.Equal(t, float64(userID), claims[platformconstant.SubKey])
			require.Equal(t, "admin", claims[platformconstant.RoleKey])

			_, err = maker.VerifyRefreshToken(got.Tokens.RefreshToken)
			require.NoError(t, err)

			stored := testhelper.GetRefreshToken(t, testDB, got.Tokens.RefreshTokenID)
			require.Equal(t, userID, stored.UserID)
			require.False(t, stored.RevokedAt.Valid)

			// The login's session: the family, carried by the access token as sid.
			require.Equal(t, stored.FamilyID.String(), claims[platformconstant.SessionIDKey])
			session := testhelper.GetSession(t, testDB, stored.FamilyID)
			require.Equal(t, userID, session.UserID)
			require.Equal(t, tc.req.TrustDevice, session.Trusted)
			require.Equal(t, "Mozilla/5.0 (test)", session.UserAgent)
			require.Equal(t, "203.0.113.7", session.ClientIP)
			require.False(t, session.RevokedAt.Valid)
			if tc.req.TrustDevice {
				require.False(t, stored.SessionExpiresAt.Valid)
				require.WithinDuration(t, time.Now().Add(time.Hour), got.Tokens.RefreshTokenExpiresAt, 5*time.Second)
			} else {
				require.True(t, stored.SessionExpiresAt.Valid)
				require.WithinDuration(t, time.Now().Add(untrustedTTL), stored.SessionExpiresAt.Time, 5*time.Second)
				require.WithinDuration(t, time.Now().Add(untrustedTTL), got.Tokens.RefreshTokenExpiresAt, 5*time.Second)
			}
		})
	}
}

// TestService_Login_FoldedLikeTheLookup_I: Postgres (glibc, Unicode 16) lowers
// U+A7DC to U+019B ("ƛ"); Go's strings.ToLower leaves it. Both spellings reach
// the same account, so they must share one failure count, or an attacker gets
// twice the guesses by alternating them.
func TestService_Login_FoldedLikeTheLookup_I(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	maker := jwttoken.NewTokenMakerES256(key, &key.PublicKey, "client", "issuer")

	testCases := []struct {
		name      string
		spellings []string // used in turn for the wrong guesses
	}{
		{name: "two spellings the database folds to one account share one count: the 6th wrong guess is a 429",
			spellings: []string{"ƛjane@example.com", "Ƛjane@example.com"}},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			defer testhelper.Truncate(t, testDB)
			testhelper.SeedUser(t, testDB, testhelper.SeedUserParams{Email: "ƛjane@example.com"})

			var folded string
			require.NoError(t, testDB.QueryRow(`SELECT lower($1::varchar)`, "Ƛjane@example.com").Scan(&folded))
			require.Equal(t, "ƛjane@example.com", folded, "precondition: this database folds U+A7DC to U+019B")

			svc := NewService(sqlc.New(testDB), authsharedsqlc.New(testDB), maker, time.Minute, time.Hour, time.Hour,
				Limiters{
					Email: ratelimit.NewMemory(ratelimit.Rate{Burst: 5, Per: 15 * time.Minute}),
					IP:    ratelimit.NewMemory(ratelimit.Rate{Burst: 100, Per: time.Hour}),
				})
			for i := range 6 {
				email := tc.spellings[i%len(tc.spellings)]
				_, err := svc.Login(context.Background(), Request{Email: email, Password: "wrong-password"}, Client{IP: "203.0.113.7"})
				var appErr *apperror.AppError
				require.ErrorAs(t, err, &appErr, "attempt %d", i+1)
				want := apperror.CodeUnauthorized
				if i == 5 {
					want = apperror.CodeTooManyRequests
				}
				require.Equal(t, want, appErr.Code, "attempt %d (%q)", i+1, email)
			}
		})
	}
}

// TestService_Login_NFC_I pins decision 0019 against Postgres: a password
// signs in whichever Unicode form the device sends, and a hash made before
// passwords were normalized (bcrypt of the bytes as sent) is upgraded on its
// first login, after which the other form works too. The upgrade never
// overwrites a hash that changed since it was read.
func TestService_Login_NFC_I(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	maker := jwttoken.NewTokenMakerES256(key, &key.PublicKey, "client", "issuer")
	login := func(plain string) error {
		_, err := NewService(sqlc.New(testDB), authsharedsqlc.New(testDB), maker, time.Minute, time.Hour, time.Hour,
			Limiters{
				Email: ratelimit.NewMemory(ratelimit.Rate{Burst: 5, Per: time.Minute}),
				IP:    ratelimit.NewMemory(ratelimit.Rate{Burst: 5, Per: time.Minute}),
			}).Login(context.Background(), Request{Email: "jane@example.com", Password: plain}, Client{IP: "203.0.113.7"})
		return err
	}
	const (
		composed   = "caf\u00e9-Passw0rd"
		decomposed = "cafe\u0301-Passw0rd"
	)

	// setLegacy stores bcrypt of plain's bytes as sent: a hash made before 0019.
	setLegacy := func(t *testing.T, userID int64, plain string) {
		t.Helper()
		b, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.MinCost)
		require.NoError(t, err)
		_, err = testDB.Exec(`UPDATE users SET password_hash = $1 WHERE id = $2`, string(b), userID)
		require.NoError(t, err)
	}

	testCases := []struct {
		name string
		run  func(t *testing.T, userID int64)
	}{
		{name: "a legacy hash of the decomposed form: signs in, is rehashed, then the composed form signs in too",
			run: func(t *testing.T, userID int64) {
				setLegacy(t, userID, decomposed)
				before := testhelper.GetUser(t, testDB, userID).PasswordHash
				var appErr *apperror.AppError
				require.ErrorAs(t, login(composed), &appErr, "before the upgrade the other form can't match")
				require.Equal(t, apperror.CodeUnauthorized, appErr.Code)
				require.NoError(t, login(decomposed))
				after := testhelper.GetUser(t, testDB, userID).PasswordHash
				require.NotEqual(t, before, after, "the hash was replaced")
				require.Equal(t, password.Match{OK: true}, password.Verify(after, composed))
				require.NoError(t, login(composed), "another device's form now signs in")
				require.Equal(t, after, testhelper.GetUser(t, testDB, userID).PasswordHash, "an NFC hash isn't rewritten")
			}},
		{name: "an NFC hash (made by password.Hash from the decomposed form) signs in from both forms and is never rewritten",
			run: func(t *testing.T, userID int64) {
				hash, err := password.Hash(decomposed)
				require.NoError(t, err)
				_, err = testDB.Exec(`UPDATE users SET password_hash = $1 WHERE id = $2`, hash, userID)
				require.NoError(t, err)
				require.NoError(t, login(composed))
				require.NoError(t, login(decomposed))
				require.Equal(t, hash, testhelper.GetUser(t, testDB, userID).PasswordHash)
			}},
		{name: "a legacy password NFC makes longer than 72 bytes (20 क़: 60 sent, 120 in NFC) still signs in, and is left as it is",
			run: func(t *testing.T, userID int64) {
				qa := strings.Repeat("\u0958", 20)
				setLegacy(t, userID, qa)
				before := testhelper.GetUser(t, testDB, userID).PasswordHash
				require.NoError(t, login(qa))
				require.Equal(t, before, testhelper.GetUser(t, testDB, userID).PasswordHash, "it can't be rehashed: bcrypt would refuse it")
			}},
		{name: "a legacy 72-byte password plus one more character is wrong (bcrypt would read only the first 72 bytes)",
			run: func(t *testing.T, userID int64) {
				p72 := strings.Repeat("e\u0301", 24) // 72 bytes sent, 48 in NFC
				setLegacy(t, userID, p72)
				var appErr *apperror.AppError
				require.ErrorAs(t, login(p72+"x"), &appErr)
				require.Equal(t, apperror.CodeUnauthorized, appErr.Code)
				require.NoError(t, login(p72), "the real password still works")
			}},
		{name: "RehashPassword doesn't overwrite a hash that changed since the login read it",
			run: func(t *testing.T, userID int64) {
				current := testhelper.GetUser(t, testDB, userID).PasswordHash
				n, err := sqlc.New(testDB).RehashPassword(context.Background(),
					sqlc.RehashPasswordParams{ID: userID, NewHash: "$2a$10$new", OldHash: "$2a$10$stale"})
				require.NoError(t, err)
				require.Zero(t, n)
				require.Equal(t, current, testhelper.GetUser(t, testDB, userID).PasswordHash)
			}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			defer testhelper.Truncate(t, testDB)
			userID := testhelper.SeedUser(t, testDB, testhelper.SeedUserParams{Email: "jane@example.com", Password: decomposed})
			tc.run(t, userID)
		})
	}
}
