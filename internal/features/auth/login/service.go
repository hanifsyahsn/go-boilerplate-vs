package login

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/login/sqlc"
	authshared "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/shared"
	authsharedsqlc "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/shared/sqlc"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/jwttoken"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/ratelimit"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/apperror"
	internalshareddto "github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/dto"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/enum"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/password"
)

const feature = "Login service"

// invalidCredentialsMessage is the one 401 for an unknown email and a wrong
// password alike, so a login never tells which emails exist.
const invalidCredentialsMessage = "The email or password is incorrect." // #nosec G101 -- a user-facing message, not a credential

// tooManyFailuresMessage is the 429 once an email used up its failed logins.
const tooManyFailuresMessage = "Too many wrong attempts. Please wait a few minutes and try again."

type S interface {
	// Login checks the credentials of an active account and issues a token
	// pair. The refresh token starts a new family in refresh_tokens (one per
	// login, i.e. per device). An untrusted device's family is capped at
	// untrustedSessionDuration from now; a trusted one slides with no cap.
	//
	// Failed logins are capped per email (the failures limiter, a token
	// bucket): every attempt takes a token *first*, atomically, so parallel
	// guesses can't all pass a check before any is counted; the right password
	// refills the bucket, so in effect only failures count. Once it's empty
	// every attempt for that email is a 429 without touching the DB — the
	// right password included, until a token comes back. A limiter that
	// errors or refuses the key is a 500 / 429 (fail closed); only a failed
	// refill after a success is just logged.
	Login(ctx context.Context, req Request) (Result, error)
}

type service struct {
	querier              sqlc.Querier
	authSharedQuerier    authsharedsqlc.Querier
	tokenMaker           jwttoken.M
	accessTokenDuration  time.Duration
	refreshTokenDuration time.Duration
	// untrustedSessionDuration caps a login without trust_device.
	untrustedSessionDuration time.Duration
	// failures counts wrong credentials per email (failureKey).
	failures ratelimit.Limiter
	// checkPassword is password.CheckOrWaste; a seam for the test that pins
	// the unknown-email path still spends a bcrypt comparison.
	checkPassword func(hash, plain string) bool
	newFamilyID   func() uuid.UUID
	now           func() time.Time
}

func NewService(
	querier sqlc.Querier,
	authSharedQuerier authsharedsqlc.Querier,
	tokenMaker jwttoken.M,
	accessTokenDuration, refreshTokenDuration, untrustedSessionDuration time.Duration,
	failures ratelimit.Limiter,
) S {
	return &service{
		querier:              querier,
		authSharedQuerier:    authSharedQuerier,
		tokenMaker:           tokenMaker,
		accessTokenDuration:  accessTokenDuration,
		refreshTokenDuration: refreshTokenDuration,

		untrustedSessionDuration: untrustedSessionDuration,
		failures:                 failures,
		checkPassword:            password.CheckOrWaste,
		newFamilyID:              uuid.New,
		now:                      time.Now,
	}
}

// failureKey is the failures limiter's key for an email: trimmed and
// lowercased, the way GetUserForLogin matches it, so "Jane@x.com " and
// "jane@x.com" share one count.
func failureKey(email string) string {
	return "login:" + strings.ToLower(strings.TrimSpace(email))
}

func (s *service) Login(ctx context.Context, req Request) (Result, error) {
	key := failureKey(req.Email)
	// Take before anything else (not peek now, count later): the bcrypt
	// comparison takes ~100ms, and a check-then-count would let N parallel
	// requests all pass the check before the first one is counted.
	d, err := s.failures.Take(ctx, key)
	if err != nil {
		return Result{}, apperror.New(apperror.CodeInternal, apperror.CodeInternal.SafeToThrow(),
			fmt.Errorf("failures.Take() (%s): %w", feature, err))
	}
	if !d.Allowed {
		return Result{}, apperror.New(apperror.CodeTooManyRequests, tooManyFailuresMessage,
			fmt.Errorf("(%s): login failures exceeded for an email", feature))
	}

	user, err := s.querier.GetUserForLogin(ctx, strings.TrimSpace(req.Email))
	if errors.Is(err, sql.ErrNoRows) {
		// Spend a bcrypt comparison anyway, so an unknown email answers in
		// about the time a wrong password does and timing doesn't reveal
		// which emails have an account.
		s.checkPassword("", req.Password)
		return Result{}, apperror.New(apperror.CodeUnauthorized, invalidCredentialsMessage,
			fmt.Errorf("GetUserForLogin() (%s): unknown email: %w", feature, err))
	}
	if err != nil {
		return Result{}, apperror.New(apperror.CodeInternal, apperror.CodeInternal.SafeToThrow(),
			fmt.Errorf("GetUserForLogin() (%s): %w", feature, err))
	}

	if !s.checkPassword(user.PasswordHash, req.Password) {
		return Result{}, apperror.New(apperror.CodeUnauthorized, invalidCredentialsMessage,
			fmt.Errorf("(%s): wrong password for user %d", feature, user.ID))
	}

	// The credentials are right: the email's failures start over (this
	// attempt's token included). A failed refill only leaves the count as it
	// was, so it doesn't fail the login.
	if err := s.failures.Reset(ctx, key); err != nil {
		slog.WarnContext(ctx, "login: reset failure count failed", "feature", feature, "err", err)
	}

	// Checked after the password, so only the account holder learns it's
	// deactivated.
	if !user.IsActive {
		return Result{}, apperror.New(apperror.CodeUnauthorized, "Your account is no longer active.",
			fmt.Errorf("(%s): user %d is not active", feature, user.ID))
	}

	refreshTokenDuration := s.refreshTokenDuration
	var sessionExpiresAt sql.NullTime
	if !req.TrustDevice {
		sessionExpiresAt = sql.NullTime{Time: s.now().Add(s.untrustedSessionDuration), Valid: true}
		refreshTokenDuration = min(refreshTokenDuration, s.untrustedSessionDuration)
	}

	tokens, err := s.tokenMaker.CreateToken(internalshareddto.User{
		ID:    user.ID,
		Name:  user.Name,
		Email: user.Email,
		Role:  enum.Role(user.Role),
	}, s.accessTokenDuration, refreshTokenDuration)
	if err != nil {
		return Result{}, apperror.New(apperror.CodeInternal, apperror.CodeInternal.SafeToThrow(),
			fmt.Errorf("tokenMaker.CreateToken() (%s): %w", feature, err))
	}

	if err = s.authSharedQuerier.InsertRefreshToken(ctx, authsharedsqlc.InsertRefreshTokenParams{
		Jti:       tokens.RefreshTokenID,
		UserID:    user.ID,
		FamilyID:  s.newFamilyID(),
		ExpiresAt: tokens.RefreshTokenExpiresAt,

		SessionExpiresAt: sessionExpiresAt,
	}); err != nil {
		return Result{}, apperror.New(apperror.CodeInternal, apperror.CodeInternal.SafeToThrow(),
			fmt.Errorf("InsertRefreshToken() (%s): %w", feature, err))
	}

	return Result{
		User:   authshared.NewUserResource(user.ID, user.Name, user.Email, user.Role),
		Tokens: tokens,
	}, nil
}
