// Package emailverification marks an account's email verified from the
// emailed link.
package emailverification

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	authshared "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/shared"
	authsharedsqlc "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/shared/sqlc"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/apperror"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/onetimetoken"
)

const feature = "Email Verification service"

type S interface {
	// Verify uses the link's token (VERIFY_EMAIL, once, unexpired) and marks
	// the account's email verified (the first time is kept). A bad token is
	// the same 422 invalid_token as a reset, on /data/attributes/token.
	Verify(ctx context.Context, req Request) error
}

type service struct {
	store StoreI
}

func NewService(store StoreI) S {
	return &service{store: store}
}

func (s *service) Verify(ctx context.Context, req Request) error {
	return s.store.RunInTx(ctx, func(q Querier) error {
		userID, err := q.ConsumeAuthToken(ctx, authsharedsqlc.ConsumeAuthTokenParams{
			TokenHash: onetimetoken.Hash(req.Token), Purpose: authshared.PurposeVerifyEmail,
		})
		if errors.Is(err, sql.ErrNoRows) {
			return apperror.New(authshared.CodeInvalidToken, authshared.InvalidTokenMessage,
				fmt.Errorf("ConsumeAuthToken() (%s): unknown, used or expired: %w", feature, err)).WithPointer("/data/attributes/token")
		}
		if err != nil {
			return apperror.New(apperror.CodeInternal, apperror.CodeInternal.SafeToThrow(),
				fmt.Errorf("ConsumeAuthToken() (%s): %w", feature, err))
		}
		if err = q.MarkEmailVerified(ctx, userID); err != nil {
			return apperror.New(apperror.CodeInternal, apperror.CodeInternal.SafeToThrow(),
				fmt.Errorf("MarkEmailVerified(%d) (%s): %w", userID, feature, err))
		}
		return nil
	})
}
