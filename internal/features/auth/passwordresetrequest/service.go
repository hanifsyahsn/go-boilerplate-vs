// Package passwordresetrequest is "forgot password": it emails a one-time
// link to reset the password, answering the same whether the email has an
// account or not.
package passwordresetrequest

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/passwordresetrequest/sqlc"
	authshared "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/shared"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/ratelimit"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/apperror"
)

const feature = "Password Reset Request service"

// SentMessage is the one answer, whatever happened: it never tells whether
// the email has an account.
const SentMessage = "If an account exists for that email, we've sent a link to reset the password."

type S interface {
	// Request emails a reset link (RESET_PASSWORD, 1 h; earlier unused links
	// stop working) to an active account with that email. An unknown email,
	// an inactive account, a recipient past its limit and a failed send all
	// look the same to the caller (nil): only a DB error is returned.
	//
	// The recipient limit (3 / h per email, taken before the lookup) stops
	// the endpoint from being used to flood one inbox; a refused or failed
	// count sends nothing (never an uncounted email).
	Request(ctx context.Context, req Request) error
}

type service struct {
	querier   sqlc.Querier
	store     StoreI
	recipient ratelimit.Limiter
	emailer   authshared.Emailer
	now       func() time.Time
}

func NewService(querier sqlc.Querier, store StoreI, recipient ratelimit.Limiter, emailer authshared.Emailer) S {
	return &service{querier: querier, store: store, recipient: recipient, emailer: emailer, now: time.Now}
}

// recipientKey is the limiter's key: the email trimmed and lowercased, the
// way the lookup matches it.
func recipientKey(email string) string {
	return "reset:" + strings.ToLower(strings.TrimSpace(email))
}

func (s *service) Request(ctx context.Context, req Request) error {
	email := strings.TrimSpace(req.Email)

	d, err := s.recipient.Take(ctx, recipientKey(email))
	if err != nil {
		slog.ErrorContext(ctx, "password reset: recipient limit failed", "err", err)
		return nil
	}
	if !d.Allowed {
		slog.WarnContext(ctx, "password reset: recipient limit reached")
		return nil
	}

	user, err := s.querier.GetUserForPasswordReset(ctx, email)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return apperror.New(apperror.CodeInternal, apperror.CodeInternal.SafeToThrow(),
			fmt.Errorf("GetUserForPasswordReset() (%s): %w", feature, err))
	}
	if !user.IsActive {
		return nil
	}

	var plain string
	err = s.store.RunInTx(ctx, func(q Querier) error {
		var err error
		plain, err = authshared.IssueAuthToken(ctx, q, authshared.PurposeResetPassword, user.ID, authshared.ResetTokenTTL, s.now())
		return err
	})
	if err != nil {
		return apperror.New(apperror.CodeInternal, apperror.CodeInternal.SafeToThrow(),
			fmt.Errorf("IssueAuthToken(%d) (%s): %w", user.ID, feature, err))
	}

	s.emailer.Send(ctx, authshared.PurposeResetPassword, user.ID, authshared.ResetEmail(s.emailer.AppBaseURL, user.Email, plain))
	return nil
}
