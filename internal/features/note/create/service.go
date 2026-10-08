package create

import (
	"context"
	"fmt"
	"strings"

	"github.com/hanifsyahsn/go_boilerplate_vs/internal/features/note/create/sqlc"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/features/note/shared"
	notesharedsqlc "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/note/shared/sqlc"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/apperror"
)

const feature = "Create Note service"

type S interface {
	// Create stores a new note owned by userID and returns it.
	Create(ctx context.Context, req Request, userID int64) (shared.NoteResource, error)
}

type service struct {
	noteSharedQuerier notesharedsqlc.Querier
	querier           sqlc.Querier
}

func NewService(noteSharedQuerier notesharedsqlc.Querier, querier sqlc.Querier) S {
	return &service{noteSharedQuerier: noteSharedQuerier, querier: querier}
}

func (s *service) Create(ctx context.Context, req Request, userID int64) (shared.NoteResource, error) {
	id, err := s.querier.CreateNote(ctx, sqlc.CreateNoteParams{
		Title:     strings.TrimSpace(req.Title),
		Body:      req.Body,
		CreatedBy: userID,
	})
	if err != nil {
		return shared.NoteResource{}, apperror.New(apperror.CodeInternal,
			"We couldn't save the note. Please try again, and contact support if the problem continues.",
			fmt.Errorf("querier.CreateNote() (%s): %w", feature, err))
	}

	// Read it back through the shared query so the response carries the
	// creator and the DB-set timestamps, exactly like detail's.
	note, err := shared.GetOwnedNote(ctx, s.noteSharedQuerier, id, userID, feature)
	if err != nil {
		return shared.NoteResource{}, err
	}

	return shared.NewNoteResource(shared.FieldsFromRow(note)), nil
}
