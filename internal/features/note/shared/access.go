package shared

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/hanifsyahsn/go_boilerplate_vs/internal/features/note/shared/sqlc"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/apperror"
)

// NotFoundMessage is the 404 detail for a note that is missing, deleted, or
// someone else's — the three read the same, so a note's existence never
// leaks to a non-owner.
const NotFoundMessage = "We couldn't find that note. It may have been removed."

// GetOwnedNote reads a live note and checks the caller owns it: missing,
// deleted and someone else's all → 404.
func GetOwnedNote(ctx context.Context, q sqlc.Querier, id, userID int64, feature string) (sqlc.GetNoteByIDRow, error) {
	note, err := q.GetNoteByID(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return sqlc.GetNoteByIDRow{}, apperror.New(apperror.CodeNotFound, NotFoundMessage,
			fmt.Errorf("GetNoteByID(%d) (%s): %w", id, feature, err))
	}
	if err != nil {
		return sqlc.GetNoteByIDRow{}, apperror.New(apperror.CodeInternal,
			"We couldn't load the note. Please try again, and contact support if the problem continues.",
			fmt.Errorf("GetNoteByID(%d) (%s): %w", id, feature, err))
	}

	if err = EnsureOwner(note.CreatedBy, userID, id, feature); err != nil {
		return sqlc.GetNoteByIDRow{}, err
	}
	return note, nil
}

// LockOwnedNote locks a live note's row inside the caller's transaction and
// checks the caller owns it: missing, deleted and someone else's all → 404.
func LockOwnedNote(ctx context.Context, q sqlc.Querier, id, userID int64, feature string) error {
	note, err := q.LockNoteForUpdate(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return apperror.New(apperror.CodeNotFound, NotFoundMessage,
			fmt.Errorf("LockNoteForUpdate(%d) (%s): %w", id, feature, err))
	}
	if err != nil {
		return apperror.New(apperror.CodeInternal,
			"We couldn't load the note. Please try again, and contact support if the problem continues.",
			fmt.Errorf("LockNoteForUpdate(%d) (%s): %w", id, feature, err))
	}

	return EnsureOwner(note.CreatedBy, userID, id, feature)
}

// EnsureOwner 404s a note the caller didn't create.
func EnsureOwner(createdBy, userID, noteID int64, feature string) error {
	if createdBy != userID {
		return apperror.New(apperror.CodeNotFound, NotFoundMessage,
			fmt.Errorf("note %d not owned by caller %d (%s)", noteID, userID, feature))
	}
	return nil
}

// FieldsFromRow maps the shared GetNoteByID row onto NoteFields.
func FieldsFromRow(r sqlc.GetNoteByIDRow) NoteFields {
	return NoteFields{
		ID:           r.ID,
		Title:        r.Title,
		Body:         r.Body,
		CreatedBy:    r.CreatedBy,
		CreatedAt:    r.CreatedAt,
		UpdatedAt:    r.UpdatedAt,
		CreatorName:  r.CreatorName,
		CreatorEmail: r.CreatorEmail,
	}
}
