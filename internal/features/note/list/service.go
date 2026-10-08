package list

import (
	"context"
	"fmt"
	"strings"

	"github.com/hanifsyahsn/go_boilerplate_vs/internal/features/note/list/sqlc"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/features/note/shared"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/apperror"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/pagination"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/utility"
)

const feature = "List Note service"

const loadErrorMessage = "We couldn't load your notes. Please try again, and contact support if the problem continues."

type S interface {
	// List returns one page of the caller's live notes, in req.Sort order.
	List(ctx context.Context, req Request, userID int64) ([]shared.NoteResource, pagination.Meta, error)
}

type service struct {
	querier sqlc.Querier
}

func NewService(querier sqlc.Querier) S {
	return &service{querier: querier}
}

func (s *service) List(ctx context.Context, req Request, userID int64) ([]shared.NoteResource, pagination.Meta, error) {
	page, pageSize, limit, offset := req.Resolve()
	q := utility.NullString(strings.TrimSpace(req.Q))

	rows, err := s.querier.ListNotes(ctx, sqlc.ListNotesParams{UserID: userID, Q: q, Sort: req.Sort, Limit: limit, Offset: offset})
	if err != nil {
		return nil, pagination.Meta{}, apperror.New(apperror.CodeInternal, loadErrorMessage,
			fmt.Errorf("querier.ListNotes() (%s): %w", feature, err))
	}

	total, err := s.querier.CountNotes(ctx, sqlc.CountNotesParams{UserID: userID, Q: q})
	if err != nil {
		return nil, pagination.Meta{}, apperror.New(apperror.CodeInternal, loadErrorMessage,
			fmt.Errorf("querier.CountNotes() (%s): %w", feature, err))
	}

	notes := make([]shared.NoteResource, 0, len(rows))
	for _, r := range rows {
		notes = append(notes, shared.NewNoteResource(shared.NoteFields{
			ID:           r.ID,
			Title:        r.Title,
			Body:         r.Body,
			CreatedBy:    r.CreatedBy,
			CreatedAt:    r.CreatedAt,
			UpdatedAt:    r.UpdatedAt,
			CreatorName:  r.CreatorName,
			CreatorEmail: r.CreatorEmail,
		}))
	}

	return notes, pagination.NewMeta(page, pageSize, total), nil
}
