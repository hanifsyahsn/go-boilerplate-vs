package logout

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/logout/sqlc"
	authshared "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/shared"
	authsharedsqlc "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/shared/sqlc"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/jwttoken"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/apperror"
)

const feature = "Logout service"

type S interface {
	// Logout ends the caller's session (the family of the presented token):
	// every refresh token of it is revoked, so neither the current nor a
	// leaked earlier token can be refreshed again, and the session is marked
	// revoked ("logout"), so its access tokens stop working too. A missing,
	// invalid, expired or unknown token is no session: nothing to revoke, not
	// an error.
	Logout(ctx context.Context, refreshToken string) error
}

type service struct {
	querier    sqlc.Querier
	store      StoreI
	tokenMaker jwttoken.M
}

func NewService(querier sqlc.Querier, store StoreI, tokenMaker jwttoken.M) S {
	return &service{querier: querier, store: store, tokenMaker: tokenMaker}
}

func (s *service) Logout(ctx context.Context, refreshToken string) error {
	if refreshToken == "" {
		return nil
	}

	claims, err := s.tokenMaker.VerifyRefreshToken(refreshToken)
	if err != nil {
		return nil //nolint:nilerr // an invalid / expired token has nothing to revoke: logout still succeeds
	}
	jti, err := jwttoken.TokenID(claims)
	if err != nil {
		// defensive: VerifyRefreshToken already checked jti
		return nil //nolint:nilerr // unreachable after VerifyRefreshToken; logout still succeeds
	}

	familyID, err := s.querier.GetRefreshTokenFamily(ctx, jti)
	if errors.Is(err, sql.ErrNoRows) {
		return nil // a signed token with no row (already pruned): nothing to revoke
	}
	if err != nil {
		return apperror.New(apperror.CodeInternal, apperror.CodeInternal.SafeToThrow(),
			fmt.Errorf("GetRefreshTokenFamily(%s) (%s): %w", jti, feature, err))
	}

	// Session row locked first, then revoked (authshared.EndSession): a
	// refresh in flight finishes first, and the token it rotated in is revoked
	// with the rest.
	if err = s.store.RunInTx(ctx, func(q authsharedsqlc.Querier) error {
		_, err := authshared.EndSession(ctx, q, familyID, authshared.RevokeReasonLogout)
		return err
	}); err != nil {
		return apperror.New(apperror.CodeInternal, apperror.CodeInternal.SafeToThrow(),
			fmt.Errorf("(%s): %w", feature, err))
	}
	return nil
}
