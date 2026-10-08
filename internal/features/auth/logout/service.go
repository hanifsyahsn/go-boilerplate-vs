package logout

import (
	"context"
	"fmt"

	authsharedsqlc "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/shared/sqlc"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/jwttoken"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/apperror"
)

const feature = "Logout service"

type S interface {
	// Logout revokes every refresh token of the caller's login (the family of
	// the presented token), so neither the current nor a leaked earlier token
	// can be refreshed again. A missing, invalid or expired token is no
	// session: nothing to revoke, not an error.
	Logout(ctx context.Context, refreshToken string) error
}

type service struct {
	authSharedQuerier authsharedsqlc.Querier
	tokenMaker        jwttoken.M
}

func NewService(authSharedQuerier authsharedsqlc.Querier, tokenMaker jwttoken.M) S {
	return &service{authSharedQuerier: authSharedQuerier, tokenMaker: tokenMaker}
}

func (s *service) Logout(ctx context.Context, refreshToken string) error {
	if refreshToken == "" {
		return nil
	}

	claims, err := s.tokenMaker.VerifyRefreshToken(refreshToken)
	if err != nil {
		return nil
	}
	jti, err := jwttoken.TokenID(claims)
	if err != nil {
		// defensive: VerifyRefreshToken already checked jti
		return nil
	}

	if err = s.authSharedQuerier.RevokeRefreshTokenFamily(ctx, jti); err != nil {
		return apperror.New(apperror.CodeInternal, apperror.CodeInternal.SafeToThrow(),
			fmt.Errorf("RevokeRefreshTokenFamily(%s) (%s): %w", jti, feature, err))
	}
	return nil
}
