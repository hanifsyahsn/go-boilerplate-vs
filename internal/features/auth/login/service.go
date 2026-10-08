package login

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/login/sqlc"
	authshared "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/shared"
	authsharedsqlc "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/shared/sqlc"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/jwttoken"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/apperror"
	internalshareddto "github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/dto"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/enum"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/password"
)

const feature = "Login service"

// invalidCredentialsMessage is the one 401 for an unknown email and a wrong
// password alike, so a login never tells which emails exist.
const invalidCredentialsMessage = "The email or password is incorrect."

type S interface {
	// Login checks the credentials of an active account and issues a token
	// pair. The refresh token starts a new family in refresh_tokens (one per
	// login, i.e. per device).
	Login(ctx context.Context, req Request) (Result, error)
}

type service struct {
	querier              sqlc.Querier
	authSharedQuerier    authsharedsqlc.Querier
	tokenMaker           jwttoken.M
	accessTokenDuration  time.Duration
	refreshTokenDuration time.Duration
	newFamilyID          func() uuid.UUID
}

func NewService(
	querier sqlc.Querier,
	authSharedQuerier authsharedsqlc.Querier,
	tokenMaker jwttoken.M,
	accessTokenDuration, refreshTokenDuration time.Duration,
) S {
	return &service{
		querier:              querier,
		authSharedQuerier:    authSharedQuerier,
		tokenMaker:           tokenMaker,
		accessTokenDuration:  accessTokenDuration,
		refreshTokenDuration: refreshTokenDuration,
		newFamilyID:          uuid.New,
	}
}

func (s *service) Login(ctx context.Context, req Request) (Result, error) {
	user, err := s.querier.GetUserForLogin(ctx, strings.TrimSpace(req.Email))
	if errors.Is(err, sql.ErrNoRows) {
		return Result{}, apperror.New(apperror.CodeUnauthorized, invalidCredentialsMessage,
			fmt.Errorf("GetUserForLogin() (%s): unknown email: %w", feature, err))
	}
	if err != nil {
		return Result{}, apperror.New(apperror.CodeInternal, apperror.CodeInternal.SafeToThrow(),
			fmt.Errorf("GetUserForLogin() (%s): %w", feature, err))
	}

	if !password.Check(user.PasswordHash, req.Password) {
		return Result{}, apperror.New(apperror.CodeUnauthorized, invalidCredentialsMessage,
			fmt.Errorf("(%s): wrong password for user %d", feature, user.ID))
	}

	// Checked after the password, so only the account holder learns it's
	// deactivated.
	if !user.IsActive {
		return Result{}, apperror.New(apperror.CodeUnauthorized, "Your account is no longer active.",
			fmt.Errorf("(%s): user %d is not active", feature, user.ID))
	}

	tokens, err := s.tokenMaker.CreateToken(internalshareddto.User{
		ID:    user.ID,
		Name:  user.Name,
		Email: user.Email,
		Role:  enum.Role(user.Role),
	}, s.accessTokenDuration, s.refreshTokenDuration)
	if err != nil {
		return Result{}, apperror.New(apperror.CodeInternal, apperror.CodeInternal.SafeToThrow(),
			fmt.Errorf("tokenMaker.CreateToken() (%s): %w", feature, err))
	}

	if err = s.authSharedQuerier.InsertRefreshToken(ctx, authsharedsqlc.InsertRefreshTokenParams{
		Jti:       tokens.RefreshTokenID,
		UserID:    user.ID,
		FamilyID:  s.newFamilyID(),
		ExpiresAt: tokens.RefreshTokenExpiresAt,
	}); err != nil {
		return Result{}, apperror.New(apperror.CodeInternal, apperror.CodeInternal.SafeToThrow(),
			fmt.Errorf("InsertRefreshToken() (%s): %w", feature, err))
	}

	return Result{
		User:   authshared.NewUserResource(user.ID, user.Name, user.Email, user.Role),
		Tokens: tokens,
	}, nil
}
