package shared

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/shared/sqlc"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/mailer"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/onetimetoken"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/requestid"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/utility"
)

// The purposes of auth_tokens (the column's CHECK allows exactly these).
const (
	PurposeVerifyEmail   = "VERIFY_EMAIL"
	PurposeResetPassword = "RESET_PASSWORD"
)

// How long a link works.
const (
	ResetTokenTTL  = time.Hour
	VerifyTokenTTL = 24 * time.Hour
)

// IssueAuthToken spends the user's unused tokens of purpose and records a new
// one valid for ttl from now; it returns the plain token for the link (only
// its hash is stored). Run it in the caller's transaction, so the old link
// dies exactly when the new one is born.
func IssueAuthToken(ctx context.Context, q sqlc.Querier, purpose string, userID int64, ttl time.Duration, now time.Time) (string, error) {
	plain, hash, _ := onetimetoken.New() // never fails
	if err := q.SpendUnusedAuthTokens(ctx, sqlc.SpendUnusedAuthTokensParams{UserID: userID, Purpose: purpose}); err != nil {
		return "", fmt.Errorf("SpendUnusedAuthTokens(%d, %s): %w", userID, purpose, err)
	}
	if err := q.InsertAuthToken(ctx, sqlc.InsertAuthTokenParams{
		Purpose: purpose, UserID: userID, TokenHash: hash, ExpiresAt: now.Add(ttl),
	}); err != nil {
		return "", fmt.Errorf("InsertAuthToken(%d, %s): %w", userID, purpose, err)
	}
	return plain, nil
}

// The emails, kept as the contract writes them.
const (
	resetSubject = "Reset your password"
	resetText    = "Someone asked to reset the password of your account. Open this link within 1 hour to choose a new one:\n\n%s\n\nIf it wasn't you, ignore this email; your password stays as it is."

	passwordChangedSubject = "Your password was changed"
	passwordChangedText    = "The password of your account was just changed and every other device was signed out. If it wasn't you, reset your password now and contact support."

	verifySubject = "Verify your email"
	verifyText    = "Open this link within 24 hours to verify your email:\n\n%s"
)

// ResetEmail is the forgot-password email with its link.
func ResetEmail(appBaseURL, to, plainToken string) mailer.Message {
	return mailer.Message{To: to, Subject: resetSubject,
		Text: fmt.Sprintf(resetText, appBaseURL+"/reset-password?token="+plainToken)}
}

// PasswordChangedEmail tells the owner their password changed (reset or
// change), so a takeover doesn't go unnoticed.
func PasswordChangedEmail(to string) mailer.Message {
	return mailer.Message{To: to, Subject: passwordChangedSubject, Text: passwordChangedText}
}

// VerifyEmail is the email-verification email with its link.
func VerifyEmail(appBaseURL, to, plainToken string) mailer.Message {
	return mailer.Message{To: to, Subject: verifySubject,
		Text: fmt.Sprintf(verifyText, appBaseURL+"/verify-email?token="+plainToken)}
}

// Emailer sends the auth emails off the request path: the response never
// waits for SMTP, and forgot-password answers in the same time whether the
// email exists or not.
type Emailer struct {
	Mailer mailer.Mailer
	// Go starts the send in main's background group (utility.Group.Go), so a
	// shutdown waits for it; tests pass one that runs it inline.
	Go utility.GoFunc
	// AppBaseURL is APP_BASE_URL, the front end the links point at.
	AppBaseURL string
}

// Send sends m in the background. A failure is logged with the purpose and
// the recipient's user id (recipient_id: user_id is the caller, from ctx) —
// never the address or the link (it carries a token).
func (e Emailer) Send(ctx context.Context, purpose string, userID int64, m mailer.Message) {
	e.Go(requestid.Detach(ctx), func(ctx context.Context) {
		if err := e.Mailer.Send(ctx, m); err != nil {
			slog.ErrorContext(ctx, "send email failed", "purpose", purpose, "recipient_id", userID, "err", err)
		}
	})
}
