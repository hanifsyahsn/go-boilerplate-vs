package list

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/features/session/list/sqlc"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/features/session/shared"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/apperror"
)

const feature = "List Sessions service"

type S interface {
	// List returns the caller's live sessions in sort order (a querySpec
	// sort value); the one of currentSession is marked current.
	List(ctx context.Context, userID int64, currentSession uuid.UUID, sort string) ([]shared.SessionResource, error)
}

type service struct {
	querier sqlc.Querier
}

func NewService(querier sqlc.Querier) S {
	return &service{querier: querier}
}

func (s *service) List(ctx context.Context, userID int64, currentSession uuid.UUID, sort string) ([]shared.SessionResource, error) {
	rows, err := s.querier.ListLiveSessions(ctx, sqlc.ListLiveSessionsParams{UserID: userID, Sort: sort})
	if err != nil {
		return nil, apperror.New(apperror.CodeInternal,
			"We couldn't load your devices. Please try again, and contact support if the problem continues.",
			fmt.Errorf("ListLiveSessions(%d) (%s): %w", userID, feature, err))
	}

	items := make([]shared.SessionResource, 0, len(rows)) // never nil: no session is "data": []
	for _, r := range rows {
		item := shared.SessionResource{
			ID:         r.FamilyID.String(),
			UserAgent:  r.UserAgent,
			ClientIP:   r.ClientIp,
			Trusted:    r.Trusted,
			CreatedAt:  r.CreatedAt,
			LastUsedAt: r.LastUsedAt,
			Current:    r.FamilyID == currentSession,
		}
		if r.SessionExpiresAt.Valid {
			expiresAt := r.SessionExpiresAt.Time
			item.ExpiresAt = &expiresAt
		}
		items = append(items, item)
	}
	return items, nil
}
