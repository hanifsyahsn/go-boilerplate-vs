package me

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"

	"github.com/hanifsyahsn/go_boilerplate_vs/internal/features/me/sqlc"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/apperror"
)

const feature = "Me service"

type S interface {
	// Get returns the caller's profile, read live from the DB.
	Get(ctx context.Context, userID int64) (MeResource, error)
}

type service struct {
	querier sqlc.Querier
}

func NewService(querier sqlc.Querier) S {
	return &service{querier: querier}
}

func (s *service) Get(ctx context.Context, userID int64) (MeResource, error) {
	r, err := s.querier.GetMe(ctx, userID)
	if errors.Is(err, sql.ErrNoRows) {
		// The auth middleware already 401s a missing user; a miss here is a
		// race with a delete.
		return MeResource{}, apperror.New(apperror.CodeNotFound, "Your account was not found.",
			fmt.Errorf("GetMe(%d) (%s): %w", userID, feature, err))
	}
	if err != nil {
		return MeResource{}, apperror.New(apperror.CodeInternal,
			"We couldn't load your profile. Please try again, and contact support if the problem continues.",
			fmt.Errorf("GetMe(%d) (%s): %w", userID, feature, err))
	}

	return MeResource{
		ID:        strconv.FormatInt(r.ID, 10),
		Name:      r.Name,
		Email:     r.Email,
		Role:      r.Role,
		CreatedAt: r.CreatedAt,
	}, nil
}
