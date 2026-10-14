package refresh

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/refresh/sqlc"
	authsharedsqlc "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/shared/sqlc"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/jwttoken"
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
	Refresh(ctx context.Context, refreshToken string) (Result, error)
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

func internalError(err error) error {
	return apperror.New(apperror.CodeInternal, apperror.CodeInternal.SafeToThrow(), err)
}

func (s *service) Refresh(ctx context.Context, refreshToken string) (Result, error) {
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

		if token.RevokedAt.Valid {
			if token.ReplacedBy.Valid && s.now().Sub(token.RevokedAt.Time) < reuseGrace {
				return sessionExpired(fmt.Errorf("(%s): token %s was just rotated (concurrent refresh)", feature, jti))
			}
			if err := q.RevokeRefreshTokenFamily(ctx, jti); err != nil {
				return internalError(fmt.Errorf("RevokeRefreshTokenFamily(%s) (%s): %w", jti, feature, err))
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
			return apperror.New(apperror.CodeUnauthorized, "Your account is no longer active.",
				fmt.Errorf("GetUserForRefresh(%d) (%s): missing or inactive: %v", userID, feature, err))
		}
		if err != nil {
			return internalError(fmt.Errorf("GetUserForRefresh(%d) (%s): %w", userID, feature, err))
		}

		pair, err := s.tokenMaker.CreateToken(internalshareddto.User{
			ID:    user.ID,
			Name:  user.Name,
			Email: user.Email,
			Role:  enum.Role(user.Role),
		}, s.accessTokenDuration, refreshTokenDuration)
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
