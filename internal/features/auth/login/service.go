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
	// login, i.e. per device) together with its auth_sessions row (trusted,
	// the client's User-Agent and IP), in one statement; the access token's
	// sid is the family id. An untrusted device's family is capped at
	// untrustedSessionDuration from now; a trusted one slides with no cap.
	//
	// Failed logins are capped twice (token buckets, Limiters): per email,
	// and per client IP across every email (credential stuffing tries each
	// email once, so the per-email cap alone never fires). Every attempt
	// takes a token from both *first*, atomically, so parallel guesses can't
	// all pass a check before any is counted — the IP first, so a caller the
	// IP cap already stopped can't drain a victim's email budget, and the IP
	// token is given back when the email refuses (no password was tried).
	// The right password refills the email bucket and gives the IP token
	// back (never refills the IP bucket: one real account would wipe an
	// attacker's count), so in effect only failures count. An empty bucket
	// is a 429 without touching the DB — the right password included, until
	// a token comes back. A limiter that errors or refuses the key is a
	// 500 / 429 (fail closed); only a failed give-back is just logged.
	Login(ctx context.Context, req Request, client Client) (Result, error)
}

// Limiters are the login's failure budgets.
type Limiters struct {
	// Email counts wrong credentials per email (failureKey).
	Email ratelimit.Limiter
	// IP counts wrong credentials per client IP, across emails (ipFailureKey).
	IP ratelimit.Limiter
}

// Client is who is logging in, from the request (not the document).
type Client struct {
	// IP is c.ClientIP() (TRUSTED_PROXIES decides whether a proxy's
	// X-Forwarded-For counts).
	IP string
	// UserAgent is the request's User-Agent (kept for the session list).
	UserAgent string
}

type service struct {
	querier              sqlc.Querier
	authSharedQuerier    authsharedsqlc.Querier
	tokenMaker           jwttoken.M
	accessTokenDuration  time.Duration
	refreshTokenDuration time.Duration
	// untrustedSessionDuration caps a login without trust_device.
	untrustedSessionDuration time.Duration
	limits                   Limiters
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
	limits Limiters,
) S {
	return &service{
		querier:              querier,
		authSharedQuerier:    authSharedQuerier,
		tokenMaker:           tokenMaker,
		accessTokenDuration:  accessTokenDuration,
		refreshTokenDuration: refreshTokenDuration,

		untrustedSessionDuration: untrustedSessionDuration,
		limits:                   limits,
		checkPassword:            password.CheckOrWaste,
		newFamilyID:              uuid.New,
		now:                      time.Now,
	}
}

// failureKey is the failures limiter's key for an email already folded by
// Postgres (FoldEmail), the way GetUserForLogin matches it, so "Jane@x.com "
// and "jane@x.com" share one count, and so does every spelling the database
// treats as the same account.
func failureKey(folded string) string {
	return "login:" + folded
}

// ipFailureKey is the IP limiter's key: an IPv6 client is counted per /64.
func ipFailureKey(ip string) string {
	return "loginip:" + ratelimit.IPKey(ip)
}

func (s *service) Login(ctx context.Context, req Request, client Client) (Result, error) {
	email := strings.TrimSpace(req.Email) // the fold and the lookup get the same value
	ipKey := ipFailureKey(client.IP)
	// Take before anything else (not peek now, count later): the bcrypt
	// comparison takes ~100ms, and a check-then-count would let N parallel
	// requests all pass the check before the first one is counted. The IP's
	// token needs no email, so a login the IP cap refuses costs no DB query.
	d, err := s.limits.IP.Take(ctx, ipKey)
	if err != nil {
		return Result{}, apperror.New(apperror.CodeInternal, apperror.CodeInternal.SafeToThrow(),
			fmt.Errorf("limits.IP.Take() (%s): %w", feature, err))
	}
	if !d.Allowed {
		return Result{}, apperror.New(apperror.CodeTooManyRequests, tooManyFailuresMessage,
			fmt.Errorf("(%s): login failures exceeded for an IP", feature)).WithRetryAfter(d.RetryAfter)
	}

	// Fold the email where the lookup folds it (Postgres), not with Go's
	// strings.ToLower: their Unicode tables differ for a few letters, and a
	// key that differs from the lookup would give one account two counts.
	folded, err := s.authSharedQuerier.FoldEmail(ctx, email)
	if err != nil {
		// No password is tried, so this attempt doesn't count against the IP.
		s.giveBack(ctx, "ip", s.limits.IP.Return(ctx, ipKey))
		return Result{}, apperror.New(apperror.CodeInternal, apperror.CodeInternal.SafeToThrow(),
			fmt.Errorf("FoldEmail() (%s): %w", feature, err))
	}
	emailKey := failureKey(folded)

	d, err = s.limits.Email.Take(ctx, emailKey)
	if err != nil || !d.Allowed {
		// No password is tried, so this attempt doesn't count against the IP.
		s.giveBack(ctx, "ip", s.limits.IP.Return(ctx, ipKey))
	}
	if err != nil {
		return Result{}, apperror.New(apperror.CodeInternal, apperror.CodeInternal.SafeToThrow(),
			fmt.Errorf("limits.Email.Take() (%s): %w", feature, err))
	}
	if !d.Allowed {
		return Result{}, apperror.New(apperror.CodeTooManyRequests, tooManyFailuresMessage,
			fmt.Errorf("(%s): login failures exceeded for an email", feature)).WithRetryAfter(d.RetryAfter)
	}

	user, err := s.querier.GetUserForLogin(ctx, email)
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
	// attempt's token included), and the IP gets this attempt's token back —
	// only that one, the IP's other failures stay counted. A failed give-back
	// only leaves a count as it was, so it doesn't fail the login.
	s.giveBack(ctx, "email", s.limits.Email.Reset(ctx, emailKey))
	s.giveBack(ctx, "ip", s.limits.IP.Return(ctx, ipKey))

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

	familyID := s.newFamilyID() // the session's id, carried by the access token as sid
	tokens, err := s.tokenMaker.CreateToken(internalshareddto.User{
		ID:    user.ID,
		Name:  user.Name,
		Email: user.Email,
		Role:  enum.Role(user.Role),
	}, familyID, s.accessTokenDuration, refreshTokenDuration)
	if err != nil {
		return Result{}, apperror.New(apperror.CodeInternal, apperror.CodeInternal.SafeToThrow(),
			fmt.Errorf("tokenMaker.CreateToken() (%s): %w", feature, err))
	}

	device := authshared.NewSessionClient(client.UserAgent, client.IP)
	if err = s.authSharedQuerier.StartSession(ctx, authsharedsqlc.StartSessionParams{
		Jti:       tokens.RefreshTokenID,
		UserID:    user.ID,
		FamilyID:  familyID,
		ExpiresAt: tokens.RefreshTokenExpiresAt,

		SessionExpiresAt: sessionExpiresAt,
		Trusted:          req.TrustDevice,
		UserAgent:        device.UserAgent,
		ClientIp:         device.IP,
	}); err != nil {
		return Result{}, apperror.New(apperror.CodeInternal, apperror.CodeInternal.SafeToThrow(),
			fmt.Errorf("StartSession() (%s): %w", feature, err))
	}

	return Result{
		User:   authshared.NewUserResource(user.ID, user.Name, user.Email, user.Role),
		Tokens: tokens,
	}, nil
}

// giveBack logs a failed Reset / Return of a failure budget; it never fails
// the login.
func (s *service) giveBack(ctx context.Context, limiter string, err error) {
	if err != nil {
		slog.WarnContext(ctx, "login: give back failure count failed", "feature", feature, "limiter", limiter, "err", err)
	}
}
