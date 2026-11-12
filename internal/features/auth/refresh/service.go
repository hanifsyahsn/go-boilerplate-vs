package refresh

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/refresh/sqlc"
	authshared "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/shared"
	authsharedsqlc "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/shared/sqlc"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/jwttoken"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/middleware"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/apperror"
	internalshareddto "github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/dto"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/enum"
)

const feature = "Refresh service"

const sessionExpiredMessage = "Your session has expired. Please log in again."

// reuseGrace is how long a just-rotated refresh token may come back without
// counting as stolen: two tabs refreshing at the same moment send the same
// cookie, and the loser must not log the winner out. Inside the window the
// loser gets a 401 (its browser already holds the winner's new cookie, so a
// retry works); after it, a spent token presented again revokes its family.
const reuseGrace = 10 * time.Second

type S interface {
	// Refresh trades a valid refresh token for a new access + refresh token
	// pair (rotation): the presented token is spent and the new one expires
	// REFRESH_TOKEN_DURATION from now, so a user who keeps coming back is
	// never logged out — on a trusted device. An untrusted device's login
	// carries a session_expires_at: the new token never outlives it, and
	// past it the refresh is a 401. The user is re-read, so the new token carries the
	// current role, and a deactivated user gets none. A spent token
	// presented again (after reuseGrace) revokes every token of its login.
	//
	// Its session (auth_sessions) must be live — a signed-out session's
	// token is a 401 — and a refresh records its use (last_used_at, the
	// client's User-Agent and IP). The new access token keeps the session's
	// sid.
	Refresh(ctx context.Context, refreshToken string, client authshared.SessionClient) (Result, error)
}

type service struct {
	store                StoreI
	tokenMaker           jwttoken.M
	accessTokenDuration  time.Duration
	refreshTokenDuration time.Duration
	now                  func() time.Time
}

func NewService(store StoreI, tokenMaker jwttoken.M, accessTokenDuration, refreshTokenDuration time.Duration) S {
	return &service{
		store:                store,
		tokenMaker:           tokenMaker,
		accessTokenDuration:  accessTokenDuration,
		refreshTokenDuration: refreshTokenDuration,
		now:                  time.Now,
	}
}

func sessionExpired(err error) error {
	return apperror.New(apperror.CodeUnauthorized, sessionExpiredMessage, err)
}

// accountInactiveMessage is the 401 detail for a missing or deactivated
// account (the access-token middleware says the same).
const accountInactiveMessage = "Your account is no longer active."

func accountInactive(err error) error {
	return apperror.New(apperror.CodeUnauthorized, accountInactiveMessage, err)
}

func internalError(err error) error {
	return apperror.New(apperror.CodeInternal, apperror.CodeInternal.SafeToThrow(), err)
}

func (s *service) Refresh(ctx context.Context, refreshToken string, client authshared.SessionClient) (Result, error) {
	device := authshared.NewSessionClient(client.UserAgent, client.IP)
	if refreshToken == "" {
		return Result{}, sessionExpired(fmt.Errorf("(%s): no refresh token in the cookie or the body", feature))
	}

	claims, err := s.tokenMaker.VerifyRefreshToken(refreshToken)
	if err != nil {
		return Result{}, sessionExpired(fmt.Errorf("tokenMaker.VerifyRefreshToken() (%s): %w", feature, err))
	}

	// defensive: VerifyRefreshToken already checked sub and jti
	userID, err := jwttoken.SubjectID(claims)
	if err != nil {
		return Result{}, sessionExpired(fmt.Errorf("jwttoken.SubjectID() (%s): %w", feature, err))
	}
	jti, err := jwttoken.TokenID(claims)
	if err != nil {
		return Result{}, sessionExpired(fmt.Errorf("jwttoken.TokenID() (%s): %w", feature, err))
	}

	var (
		res    Result
		reused bool
	)
	err = s.store.RunInTx(ctx, func(q Querier) error {
		// Session row first, then the token row: the order every revoke
		// follows too (authshared.EndSession), so they never deadlock.
		if err := q.LockSessionOfToken(ctx, jti); err != nil {
			return internalError(fmt.Errorf("LockSessionOfToken(%s) (%s): %w", jti, feature, err))
		}
		token, err := q.LockRefreshToken(ctx, jti)
		if errors.Is(err, sql.ErrNoRows) {
			return sessionExpired(fmt.Errorf("LockRefreshToken(%s) (%s): unknown token", jti, feature))
		}
		if err != nil {
			return internalError(fmt.Errorf("LockRefreshToken(%s) (%s): %w", jti, feature, err))
		}
		if token.UserID != userID {
			// defensive: the jti is signed together with the sub
			return sessionExpired(fmt.Errorf("(%s): token %s belongs to user %d, not %d", feature, jti, token.UserID, userID))
		}

		if !token.SessionLive {
			// Signed out (logout, a remote sign-out, a password change): the
			// tokens were revoked with it, so this isn't a reuse to punish.
			// The details match the access-token middleware: an inactive
			// account first (an admin deactivation also ends its sessions),
			// else "your session has ended".
			user, err := q.GetUserForRefresh(ctx, userID)
			if errors.Is(err, sql.ErrNoRows) || (err == nil && !user.IsActive) {
				return accountInactive(fmt.Errorf("GetUserForRefresh(%d) (%s): signed out, missing or inactive: %v", userID, feature, err)) //nolint:errorlint // err is nil for an inactive user; %w would print %!w(<nil>)
			}
			if err != nil {
				return internalError(fmt.Errorf("GetUserForRefresh(%d) (%s): %w", userID, feature, err))
			}
			return apperror.New(apperror.CodeUnauthorized, middleware.SessionEndedMessage,
				fmt.Errorf("(%s): session %s of token %s is not live", feature, token.FamilyID, jti))
		}

		if token.RevokedAt.Valid {
			if token.ReplacedBy.Valid && s.now().Sub(token.RevokedAt.Time) < reuseGrace {
				return sessionExpired(fmt.Errorf("(%s): token %s was just rotated (concurrent refresh)", feature, jti))
			}
			if _, err := q.RevokeSession(ctx, authsharedsqlc.RevokeSessionParams{
				FamilyID: token.FamilyID, Reason: authshared.RevokeReasonTokenReused,
			}); err != nil {
				return internalError(fmt.Errorf("RevokeSession(%s) (%s): %w", token.FamilyID, feature, err))
			}
			// Commit the revocation; the 401 is returned after the tx.
			reused = true
			return nil
		}

		refreshTokenDuration := s.refreshTokenDuration
		if token.SessionExpiresAt.Valid {
			left := token.SessionExpiresAt.Time.Sub(s.now())
			if left <= 0 {
				return sessionExpired(fmt.Errorf("(%s): session of token %s ended at %s", feature, jti, token.SessionExpiresAt.Time))
			}
			refreshTokenDuration = min(refreshTokenDuration, left)
		}

		user, err := q.GetUserForRefresh(ctx, userID)
		if errors.Is(err, sql.ErrNoRows) || (err == nil && !user.IsActive) {
			return accountInactive(fmt.Errorf("GetUserForRefresh(%d) (%s): missing or inactive: %v", userID, feature, err)) //nolint:errorlint // err is nil for an inactive user; %w would print %!w(<nil>)
		}
		if err != nil {
			return internalError(fmt.Errorf("GetUserForRefresh(%d) (%s): %w", userID, feature, err))
		}

		pair, err := s.tokenMaker.CreateToken(internalshareddto.User{
			ID:    user.ID,
			Name:  user.Name,
			Email: user.Email,
			Role:  enum.Role(user.Role),
		}, token.FamilyID, s.accessTokenDuration, refreshTokenDuration)
		if err != nil {
			return internalError(fmt.Errorf("tokenMaker.CreateToken() (%s): %w", feature, err))
		}

		if err := q.InsertRefreshToken(ctx, authsharedsqlc.InsertRefreshTokenParams{
			Jti:       pair.RefreshTokenID,
			UserID:    user.ID,
			FamilyID:  token.FamilyID,
			ExpiresAt: pair.RefreshTokenExpiresAt,

			SessionExpiresAt: token.SessionExpiresAt,
		}); err != nil {
			return internalError(fmt.Errorf("InsertRefreshToken() (%s): %w", feature, err))
		}
		if err := q.RotateRefreshToken(ctx, sqlc.RotateRefreshTokenParams{Jti: jti, ReplacedBy: pair.RefreshTokenID}); err != nil {
			return internalError(fmt.Errorf("RotateRefreshToken(%s) (%s): %w", jti, feature, err))
		}
		if err := q.TouchSession(ctx, sqlc.TouchSessionParams{
			FamilyID: token.FamilyID, UserAgent: device.UserAgent, ClientIp: device.IP,
		}); err != nil {
			return internalError(fmt.Errorf("TouchSession(%s) (%s): %w", token.FamilyID, feature, err))
		}

		res = Result{Tokens: pair}
		return nil
	})
	if err != nil {
		return Result{}, err
	}
	if reused {
		return Result{}, sessionExpired(fmt.Errorf("(%s): spent token %s presented again, family revoked", feature, jti))
	}

	return res, nil
}
