package login

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/login/sqlc"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/shared/authsharedtest"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/jwttoken"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/logging"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/apperror"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/password"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

const (
	composedPassword   = "caf\u00e9-Passw0rd"  // NFC, e.g. Windows
	decomposedPassword = "cafe\u0301-Passw0rd" // NFD, e.g. macOS
)

func bcryptOf(t *testing.T, plain string) string {
	t.Helper()
	b, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.MinCost)
	require.NoError(t, err)
	return string(b)
}

// TestService_Login_LegacyRehash_U pins decision 0019's upgrade path: a hash
// made before passwords were NFC-normalized (bcrypt of the bytes as sent)
// still signs its owner in and is replaced by the NFC hash, only if it is
// still the stored one; any other login writes nothing; a failed rehash is
// logged and never fails the login.
func TestService_Login_LegacyRehash_U(t *testing.T) {
	legacy := bcryptOf(t, decomposedPassword) // registered from a Mac before 0019
	current := bcryptOf(t, composedPassword)  // NFC: made by password.Hash, or already NFC bytes

	testCases := []struct {
		name       string
		stored     string
		plain      string
		hashErr    error
		rehashErr  error
		noRows     bool // the compare-and-set found the hash changed
		inactive   bool
		cancelled  bool // the client hung up: the request ctx is cancelled
		wantRehash bool
		wantErr    apperror.Code // zero = signed in
		wantLog    string
	}{
		{name: "a legacy hash signs in and is replaced by the NFC hash of the same password",
			stored: legacy, plain: decomposedPassword, wantRehash: true},
		{name: "an NFC hash signs in from either device and writes nothing",
			stored: current, plain: decomposedPassword},
		{name: "a wrong password writes nothing", stored: legacy, plain: "cafe\u0301-wrong",
			wantErr: apperror.CodeUnauthorized},
		{name: "a failed rehash write is logged and the login still succeeds",
			stored: legacy, plain: decomposedPassword, wantRehash: true, rehashErr: errors.New("db down"),
			wantLog: "login: rehash password failed"},
		{name: "a failed hash is logged, nothing is written, the login still succeeds",
			stored: legacy, plain: decomposedPassword, hashErr: errors.New("boom"),
			wantLog: "login: rehash password failed"},
		{name: "a hash changed meanwhile (0 rows) is left alone, logged at info",
			stored: legacy, plain: decomposedPassword, wantRehash: true, noRows: true,
			wantLog: "login: rehash skipped, stored hash no longer the one checked"},
		{name: "a deactivated account's legacy hash isn't rewritten (the login is refused anyway)",
			stored: legacy, plain: decomposedPassword, inactive: true, wantErr: apperror.CodeUnauthorized},
		{name: "a client that hung up after a right password still gets its hash upgraded (the write outlives the request)",
			stored: legacy, plain: decomposedPassword, cancelled: true, wantRehash: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var logs bytes.Buffer
			l, err := logging.New(&logs, logging.FormatJSON, "debug")
			require.NoError(t, err)
			prev := slog.Default()
			slog.SetDefault(l)
			defer slog.SetDefault(prev)

			ctrl := gomock.NewController(t)
			q := sqlc.NewMockQuerier(ctrl)
			sq := authsharedtest.FoldingQuerier(ctrl)
			m := jwttoken.NewMockM(ctrl)
			q.EXPECT().GetUserForLogin(gomock.Any(), "jane@example.com").Return(sqlc.GetUserForLoginRow{
				ID: 7, Name: "Jane", Email: "jane@example.com", PasswordHash: tc.stored, Role: "user", IsActive: !tc.inactive}, nil)
			if tc.wantErr == (apperror.Code{}) {
				m.EXPECT().CreateToken(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(jwttoken.Pair{}, nil)
				sq.EXPECT().StartSession(gomock.Any(), gomock.Any()).Return(nil)
			}
			var newHash string
			if tc.wantRehash {
				q.EXPECT().RehashPassword(gomock.Any(), gomock.Any()).DoAndReturn(
					func(ctx context.Context, arg sqlc.RehashPasswordParams) (int64, error) {
						newHash = arg.NewHash
						require.Equal(t, int64(7), arg.ID)
						require.Equal(t, tc.stored, arg.OldHash, "only if the stored hash is still the one checked")
						require.NoError(t, ctx.Err(), "the write runs on a ctx the client can't cancel")
						if tc.noRows {
							return 0, nil
						}
						return 1, tc.rehashErr
					})
			}

			svc := NewService(q, sq, m, 15*time.Minute, 24*time.Hour, 12*time.Hour, wideLimits())
			if tc.hashErr != nil {
				svc.(*service).hashPassword = func(string) (string, error) { return "", tc.hashErr }
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tc.cancelled {
				// Cancelled once the password matched: the stubs below the
				// limiter don't look at ctx, the rehash write must not depend on it.
				svc.(*service).verifyPassword = func(hash, plain string) password.Match {
					defer cancel()
					return password.VerifyOrWaste(hash, plain)
				}
			}
			_, err = svc.Login(ctx, Request{Email: "jane@example.com", Password: tc.plain}, testClient)
			if tc.wantErr != (apperror.Code{}) {
				var appErr *apperror.AppError
				require.ErrorAs(t, err, &appErr)
				require.Equal(t, tc.wantErr, appErr.Code)
				return
			}
			require.NoError(t, err)
			if tc.wantRehash && tc.rehashErr == nil && !tc.noRows {
				require.Equal(t, password.Match{OK: true}, password.Verify(newHash, composedPassword),
					"after the rehash the other device's form signs in, not as legacy")
				require.Equal(t, password.Match{OK: true}, password.Verify(newHash, decomposedPassword))
			}
			if tc.wantLog != "" {
				require.Contains(t, logs.String(), tc.wantLog)
				require.Contains(t, logs.String(), `"target_user_id":7`, "names the account it was about")
				require.NotContains(t, logs.String(), tc.plain, "the password never reaches a log")
			} else {
				require.NotContains(t, logs.String(), "rehash")
			}
		})
	}
}
