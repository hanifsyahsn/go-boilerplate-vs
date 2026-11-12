// Package passwordchange lets a signed-in user change their password.
package passwordchange

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/passwordchange/sqlc"
	authshared "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/shared"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/ratelimit"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/apperror"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/password"
)

const feature = "Password Change service"

// CodeWrongPassword is the 422 of a wrong current password.
var CodeWrongPassword = apperror.NewCode("wrong_password", http.StatusUnprocessableEntity)

const (
	wrongPasswordMessage   = "The current password is incorrect." // #nosec G101 -- a user-facing message, not a credential
	tooManyFailuresMessage = "Too many wrong attempts. Please wait a few minutes and try again."
)

type S interface {
	// Change checks the caller's current password and sets the new one; in
	// the same transaction every other session of the caller ends
	// (password_changed) — the caller's own (sessionID) stays signed in.
	// The owner then gets a "password changed" notice.
	//
	// Wrong current passwords are capped per user (5 / 15 min): every
	// attempt takes a token first, atomically (parallel guesses can't all
	// pass), the right one refills the bucket. Past it → 429, the right
	// password too. A wrong current password is a 422 wrong_password on
	// /data/attributes/current_password.
	Change(ctx context.Context, userID int64, sessionID uuid.UUID, req Request) error
}

type service struct {
	querier  sqlc.Querier
	store    StoreI
	failures ratelimit.Limiter
	emailer  authshared.Emailer
	hash     func(string) (string, error)
	check    func(hash, plain string) bool
}

func NewService(querier sqlc.Querier, store StoreI, failures ratelimit.Limiter, emailer authshared.Emailer) S {
	return &service{querier: querier, store: store, failures: failures, emailer: emailer,
		hash: password.Hash, check: password.Check}
}

func failureKey(userID int64) string {
	return "pwchange:" + strconv.FormatInt(userID, 10)
}

func internal(err error) error {
	return apperror.New(apperror.CodeInternal, apperror.CodeInternal.SafeToThrow(), err)
}

func (s *service) Change(ctx context.Context, userID int64, sessionID uuid.UUID, req Request) error {
	key := failureKey(userID)
	d, err := s.failures.Take(ctx, key)
	if err != nil {
		return internal(fmt.Errorf("failures.Take() (%s): %w", feature, err))
	}
	if !d.Allowed {
		return apperror.New(apperror.CodeTooManyRequests, tooManyFailuresMessage,
			fmt.Errorf("(%s): wrong current passwords exceeded for user %d", feature, userID))
	}

	current, err := s.querier.GetUserPassword(ctx, userID)
	if err != nil {
		// The middleware just read the user; a miss is a race with a delete.
		return internal(fmt.Errorf("GetUserPassword(%d) (%s): %w", userID, feature, err))
	}
	if !s.check(current, req.CurrentPassword) {
		return apperror.New(CodeWrongPassword, wrongPasswordMessage,
			fmt.Errorf("(%s): wrong current password for user %d", feature, userID)).WithPointer("/data/attributes/current_password")
	}
	if err = s.failures.Reset(ctx, key); err != nil {
		slog.WarnContext(ctx, "password change: reset failure count failed", "err", err)
	}

	hash, err := s.hash(req.Password)
	if err != nil {
		return internal(fmt.Errorf("password.Hash() (%s): %w", feature, err))
	}

	var email string
	err = s.store.RunInTx(ctx, func(q Querier) error {
		var err error
		if email, err = q.SetPassword(ctx, sqlc.SetPasswordParams{ID: userID, PasswordHash: hash}); err != nil {
			return internal(fmt.Errorf("SetPassword(%d) (%s): %w", userID, feature, err))
		}
		if _, err = authshared.EndUserSessions(ctx, q, userID, uuid.NullUUID{UUID: sessionID, Valid: true},
			authshared.RevokeReasonPasswordChanged); err != nil {
			return internal(fmt.Errorf("(%s): %w", feature, err))
		}
		return nil
	})
	if err != nil {
		return err
	}

	s.emailer.Send(ctx, authshared.EmailPasswordChanged, userID, authshared.PasswordChangedEmail(email))
	return nil
}
