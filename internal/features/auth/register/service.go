package register

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/register/sqlc"
	authshared "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/shared"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/apperror"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/password"
	"github.com/lib/pq"
)

const feature = "Register service"

const createFailedMessage = "We couldn't create your account. Please try again, and contact support if the problem continues."

// pqUniqueViolation is Postgres' unique_violation SQLSTATE.
const pqUniqueViolation = "23505"

type S interface {
	// Register creates a new account with role user. A taken email → 409.
	// In the same transaction it issues a VERIFY_EMAIL token (24 h); after
	// the commit the verification link is emailed. Login isn't blocked for
	// an unverified email (GET /api/me tells the client).
	Register(ctx context.Context, req Request) (authshared.UserResource, error)
}

type service struct {
	store   StoreI
	emailer authshared.Emailer
	hash    func(string) (string, error)
	now     func() time.Time
}

func NewService(store StoreI, emailer authshared.Emailer) S {
	return &service{store: store, emailer: emailer, hash: password.Hash, now: time.Now}
}

func (s *service) Register(ctx context.Context, req Request) (authshared.UserResource, error) {
	hash, err := s.hash(req.Password)
	if err != nil {
		return authshared.UserResource{}, apperror.New(apperror.CodeInternal, apperror.CodeInternal.SafeToThrow(),
			fmt.Errorf("password.Hash() (%s): %w", feature, err))
	}

	var (
		user  sqlc.CreateUserRow
		plain string
	)
	err = s.store.RunInTx(ctx, func(q Querier) error {
		var err error
		user, err = q.CreateUser(ctx, sqlc.CreateUserParams{
			Name:         strings.TrimSpace(req.Name),
			Email:        strings.TrimSpace(req.Email),
			PasswordHash: hash,
		})
		if err != nil {
			var pqErr *pq.Error
			if errors.As(err, &pqErr) && pqErr.Code == pqUniqueViolation {
				return apperror.New(apperror.CodeConflict, "That email is already registered.",
					fmt.Errorf("querier.CreateUser() (%s): %w", feature, err))
			}
			return apperror.New(apperror.CodeInternal, createFailedMessage,
				fmt.Errorf("querier.CreateUser() (%s): %w", feature, err))
		}
		plain, err = authshared.IssueAuthToken(ctx, q, authshared.PurposeVerifyEmail, user.ID, authshared.VerifyTokenTTL, s.now())
		if err != nil {
			return apperror.New(apperror.CodeInternal, createFailedMessage,
				fmt.Errorf("IssueAuthToken(%d) (%s): %w", user.ID, feature, err))
		}
		return nil
	})
	if err != nil {
		return authshared.UserResource{}, err
	}

	s.emailer.Send(ctx, authshared.PurposeVerifyEmail, user.ID, authshared.VerifyEmail(s.emailer.AppBaseURL, user.Email, plain))
	return authshared.NewUserResource(user.ID, user.Name, user.Email, user.Role), nil
}
