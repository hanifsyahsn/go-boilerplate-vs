package update

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/hanifsyahsn/go_boilerplate_vs/internal/features/note/shared"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/features/note/update/sqlc"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/apperror"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/utility"
)

const feature = "Update Note service"

type S interface {
	// Update changes the given attributes of one of the caller's live notes
	// (nil keeps the current value) and returns the note. Missing, deleted
	// and someone else's all read as 404.
	Update(ctx context.Context, id int64, req Request, userID int64) (shared.NoteResource, error)
}

type service struct {
	store StoreI
}

func NewService(store StoreI) S {
	return &service{store: store}
}

func (s *service) Update(ctx context.Context, id int64, req Request, userID int64) (shared.NoteResource, error) {
	var out shared.NoteResource

	err := s.store.RunInTx(ctx, func(q Querier) error {
		// Lock first: a concurrent update / delete of the same note waits.
		if err := shared.LockOwnedNote(ctx, q, id, userID, feature); err != nil {
			return err
		}

		params := sqlc.UpdateNoteParams{ID: id}
		if req.Title != nil {
			params.Title = utility.NullString(strings.TrimSpace(*req.Title))
		}
		if req.Body != nil {
			params.Body = sql.NullString{String: *req.Body, Valid: true}
		}
		if err := q.UpdateNote(ctx, params); err != nil {
			return apperror.New(apperror.CodeInternal,
				"We couldn't save the note. Please try again, and contact support if the problem continues.",
				fmt.Errorf("q.UpdateNote(%d) (%s): %w", id, feature, err))
		}

		note, err := shared.GetOwnedNote(ctx, q, id, userID, feature)
		if err != nil {
			return err
		}
		out = shared.NewNoteResource(shared.FieldsFromRow(note))
		return nil
	})
	if err != nil {
		return shared.NoteResource{}, err
	}

	return out, nil
}
