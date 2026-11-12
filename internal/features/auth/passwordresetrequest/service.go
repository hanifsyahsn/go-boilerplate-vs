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
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/requestid"
)

const feature = "Password Reset Request service"

// SentMessage is the one answer, whatever happened: it never tells whether
// the email has an account.
const SentMessage = "If an account exists for that email, we've sent a link to reset the password."

type S interface {
	// Request starts the reset in the background and returns at once, so the
	// answer takes the same time whether the email has an account or not (a
	// lookup or a token write before answering would tell them apart). There
	// is nothing to return: every outcome, a DB error included, is only
	// logged, never shown (decision 0007, Q14 / Q16).
	//
	// In the background: the recipient limit (3 / h per email; refused or
	// failing → nothing sent, never an uncounted email), the lookup, a
	// RESET_PASSWORD token (1 h; earlier unused links stop working) for an
	// active account, then the email. Unknown email, inactive account, a
	// limit and a failure all end quietly.
	Request(ctx context.Context, req Request)
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

func (s *service) Request(ctx context.Context, req Request) {
	email := strings.TrimSpace(req.Email)
	// main's background group (emailer.Go = bg.Go): a shutdown waits for it.
	s.emailer.Go(requestid.Detach(ctx), func(ctx context.Context) { s.reset(ctx, email) })
}

// reset is the work behind a request, off the request path. Logs never carry
// the address (or the link).
func (s *service) reset(ctx context.Context, email string) {
	d, err := s.recipient.Take(ctx, recipientKey(email))
	if err != nil {
		slog.ErrorContext(ctx, "password reset: recipient limit failed", "err", err)
		return
	}
	if !d.Allowed {
		slog.WarnContext(ctx, "password reset: recipient limit reached")
		return
	}

	user, err := s.querier.GetUserForPasswordReset(ctx, email)
	if errors.Is(err, sql.ErrNoRows) {
		return
	}
	if err != nil {
		slog.ErrorContext(ctx, "password reset: lookup failed", "err", fmt.Errorf("GetUserForPasswordReset() (%s): %w", feature, err))
		return
	}
	if !user.IsActive {
		return
	}

	var plain string
	err = s.store.RunInTx(ctx, func(q Querier) error {
		var err error
		plain, err = authshared.IssueAuthToken(ctx, q, authshared.PurposeResetPassword, user.ID, authshared.ResetTokenTTL, s.now())
		return err
	})
	if err != nil {
		slog.ErrorContext(ctx, "password reset: issue token failed", "recipient_id", user.ID,
			"err", fmt.Errorf("IssueAuthToken(%d) (%s): %w", user.ID, feature, err))
		return
	}

	s.emailer.Send(ctx, authshared.PurposeResetPassword, user.ID, authshared.ResetEmail(s.emailer.AppBaseURL, user.Email, plain))
}
