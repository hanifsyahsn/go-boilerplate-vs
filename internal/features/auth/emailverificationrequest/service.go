// Package emailverificationrequest sends a signed-in user a new email
// verification link.
package emailverificationrequest

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/emailverificationrequest/sqlc"
	authshared "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/shared"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/ratelimit"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/apperror"
)

const feature = "Email Verification Request service"

// CodeEmailAlreadyVerified is the 409 of a resend for a verified email.
var CodeEmailAlreadyVerified = apperror.NewCode("email_already_verified", http.StatusConflict)

// SentMessage is the 202's meta message.
const SentMessage = "We've sent a new verification link."

const tooManyMessage = "Too many attempts. Please wait a few minutes and try again."

type S interface {
	// Request emails the caller a new verification link (VERIFY_EMAIL, 24 h;
	// earlier links stop working). Already verified → 409
	// email_already_verified. At most 3 links per hour per user (429).
	Request(ctx context.Context, userID int64) error
}

type service struct {
	querier sqlc.Querier
	store   StoreI
	limit   ratelimit.Limiter
	emailer authshared.Emailer
	now     func() time.Time
}

func NewService(querier sqlc.Querier, store StoreI, limit ratelimit.Limiter, emailer authshared.Emailer) S {
	return &service{querier: querier, store: store, limit: limit, emailer: emailer, now: time.Now}
}

func internal(err error) error {
	return apperror.New(apperror.CodeInternal, apperror.CodeInternal.SafeToThrow(), err)
}

func (s *service) Request(ctx context.Context, userID int64) error {
	user, err := s.querier.GetUserForVerification(ctx, userID)
	if err != nil {
		// The middleware just read the user; a miss is a race with a delete.
		return internal(fmt.Errorf("GetUserForVerification(%d) (%s): %w", userID, feature, err))
	}
	if user.EmailVerified {
		return apperror.New(CodeEmailAlreadyVerified, "Your email is already verified.",
			fmt.Errorf("(%s): user %d is verified already", feature, userID))
	}

	d, err := s.limit.Take(ctx, "verify:"+strconv.FormatInt(userID, 10))
	if err != nil {
		return internal(fmt.Errorf("limit.Take() (%s): %w", feature, err))
	}
	if !d.Allowed {
		return apperror.New(apperror.CodeTooManyRequests, tooManyMessage,
			fmt.Errorf("(%s): verification links exceeded for user %d", feature, userID))
	}

	var plain string
	err = s.store.RunInTx(ctx, func(q Querier) error {
		var err error
		plain, err = authshared.IssueAuthToken(ctx, q, authshared.PurposeVerifyEmail, userID, authshared.VerifyTokenTTL, s.now())
		return err
	})
	if err != nil {
		return internal(fmt.Errorf("IssueAuthToken(%d) (%s): %w", userID, feature, err))
	}

	s.emailer.Send(ctx, authshared.PurposeVerifyEmail, userID, authshared.VerifyEmail(s.emailer.AppBaseURL, user.Email, plain))
	return nil
}
