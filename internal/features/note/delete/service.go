package delete

import (
	"context"
	"fmt"

	"github.com/hanifsyahsn/go_boilerplate_vs/internal/features/note/delete/sqlc"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/features/note/shared"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/apperror"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/utility"
)

const feature = "Delete Note service"

type S interface {
	// Delete soft-deletes one of the caller's live notes (the row is kept).
	// Missing, already deleted and someone else's all read as 404.
	Delete(ctx context.Context, id, userID int64) error
}

type service struct {
	store StoreI
}

func NewService(store StoreI) S {
	return &service{store: store}
}

func (s *service) Delete(ctx context.Context, id, userID int64) error {
	return s.store.RunInTx(ctx, func(q Querier) error {
		if err := shared.LockOwnedNote(ctx, q, id, userID, feature); err != nil {
			return err
		}

		if err := q.SoftDeleteNote(ctx, sqlc.SoftDeleteNoteParams{ID: id, DeletedBy: utility.NullInt64(userID)}); err != nil {
			return apperror.New(apperror.CodeInternal,
				"We couldn't delete the note. Please try again, and contact support if the problem continues.",
				fmt.Errorf("q.SoftDeleteNote(%d) (%s): %w", id, feature, err))
		}
		return nil
	})
}
