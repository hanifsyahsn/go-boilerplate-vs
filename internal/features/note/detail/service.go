package detail

import (
	"context"

	"github.com/hanifsyahsn/go_boilerplate_vs/internal/features/note/shared"
	notesharedsqlc "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/note/shared/sqlc"
)

const feature = "Detail Note service"

type S interface {
	// Get returns one of the caller's live notes. Missing, deleted and
	// someone else's all read as 404.
	Get(ctx context.Context, id, userID int64) (shared.NoteResource, error)
}

// service has no sqlc of its own: everything it reads is note-shared.
type service struct {
	noteSharedQuerier notesharedsqlc.Querier
}

func NewService(noteSharedQuerier notesharedsqlc.Querier) S {
	return &service{noteSharedQuerier: noteSharedQuerier}
}

func (s *service) Get(ctx context.Context, id, userID int64) (shared.NoteResource, error) {
	note, err := shared.GetOwnedNote(ctx, s.noteSharedQuerier, id, userID, feature)
	if err != nil {
		return shared.NoteResource{}, err
	}
	return shared.NewNoteResource(shared.FieldsFromRow(note)), nil
}
