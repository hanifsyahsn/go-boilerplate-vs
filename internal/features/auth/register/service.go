package register

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/register/sqlc"
	authshared "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/shared"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/apperror"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/password"
	"github.com/lib/pq"
)

const feature = "Register service"

// pqUniqueViolation is Postgres' unique_violation SQLSTATE.
const pqUniqueViolation = "23505"

type S interface {
	// Register creates a new account with role user. A taken email → 409.
	Register(ctx context.Context, req Request) (authshared.UserResource, error)
}

type service struct {
	querier sqlc.Querier
	hash    func(string) (string, error)
}

func NewService(querier sqlc.Querier) S {
	return &service{querier: querier, hash: password.Hash}
}

func (s *service) Register(ctx context.Context, req Request) (authshared.UserResource, error) {
	hash, err := s.hash(req.Password)
	if err != nil {
		return authshared.UserResource{}, apperror.New(apperror.CodeInternal, apperror.CodeInternal.SafeToThrow(),
			fmt.Errorf("password.Hash() (%s): %w", feature, err))
	}

	user, err := s.querier.CreateUser(ctx, sqlc.CreateUserParams{
		Name:         strings.TrimSpace(req.Name),
		Email:        strings.TrimSpace(req.Email),
		PasswordHash: hash,
	})
	if err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == pqUniqueViolation {
			return authshared.UserResource{}, apperror.New(apperror.CodeConflict,
				"That email is already registered.",
				fmt.Errorf("querier.CreateUser() (%s): %w", feature, err))
		}
		return authshared.UserResource{}, apperror.New(apperror.CodeInternal,
			"We couldn't create your account. Please try again, and contact support if the problem continues.",
			fmt.Errorf("querier.CreateUser() (%s): %w", feature, err))
	}

	return authshared.NewUserResource(user.ID, user.Name, user.Email, user.Role), nil
}
