// Package passwordreset sets a new password from a forgot-password link.
package passwordreset

import (
	"context"
	"fmt"

	"database/sql"
	"errors"

	"github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/passwordreset/sqlc"
	authshared "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/shared"
	authsharedsqlc "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/shared/sqlc"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/apperror"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/onetimetoken"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/password"
)

const feature = "Password Reset service"

type S interface {
	// Reset uses the link's token (RESET_PASSWORD, once, unexpired) to set
	// a new password; in the same transaction the email counts as verified
	// (the link proved it) and every session of the user ends
	// (password_reset). A bad token is a 422 invalid_token on
	// /data/attributes/token. After the commit the owner gets a "password
	// changed" notice. An inactive user's password is set too (they still
	// can't log in).
	Reset(ctx context.Context, req Request) error
}

type service struct {
	store   StoreI
	emailer authshared.Emailer
	hash    func(string) (string, error)
}

func NewService(store StoreI, emailer authshared.Emailer) S {
	return &service{store: store, emailer: emailer, hash: password.Hash}
}

func (s *service) Reset(ctx context.Context, req Request) error {
	// bcrypt before the transaction, so it doesn't hold the token row.
	hash, err := s.hash(req.Password)
	if err != nil {
		return apperror.New(apperror.CodeInternal, apperror.CodeInternal.SafeToThrow(),
			fmt.Errorf("password.Hash() (%s): %w", feature, err))
	}

	var userID int64
	var email string
	err = s.store.RunInTx(ctx, func(q Querier) error {
		var err error
		userID, err = q.ConsumeAuthToken(ctx, authsharedsqlc.ConsumeAuthTokenParams{
			TokenHash: onetimetoken.Hash(req.Token), Purpose: authshared.PurposeResetPassword,
		})
		if errors.Is(err, sql.ErrNoRows) {
			return apperror.New(authshared.CodeInvalidToken, authshared.InvalidTokenMessage,
				fmt.Errorf("ConsumeAuthToken() (%s): unknown, used or expired: %w", feature, err)).WithPointer("/data/attributes/token")
		}
		if err != nil {
			return apperror.New(apperror.CodeInternal, apperror.CodeInternal.SafeToThrow(),
				fmt.Errorf("ConsumeAuthToken() (%s): %w", feature, err))
		}

		if email, err = q.SetPasswordAfterReset(ctx, sqlc.SetPasswordAfterResetParams{ID: userID, PasswordHash: hash}); err != nil {
			return apperror.New(apperror.CodeInternal, apperror.CodeInternal.SafeToThrow(),
				fmt.Errorf("SetPasswordAfterReset(%d) (%s): %w", userID, feature, err))
		}
		if _, err = q.RevokeUserSessions(ctx, authsharedsqlc.RevokeUserSessionsParams{
			UserID: userID, Reason: authshared.RevokeReasonPasswordReset,
		}); err != nil {
			return apperror.New(apperror.CodeInternal, apperror.CodeInternal.SafeToThrow(),
				fmt.Errorf("RevokeUserSessions(%d) (%s): %w", userID, feature, err))
		}
		return nil
	})
	if err != nil {
		return err
	}

	s.emailer.Send(ctx, authshared.EmailPasswordChanged, userID, authshared.PasswordChangedEmail(email))
	return nil
}
