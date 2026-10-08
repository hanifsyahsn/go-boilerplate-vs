// Package shared holds what more than one note slice needs: the JSON:API
// resource every note endpoint answers with, the ownership rule, and the
// shared sqlc querier (sqlc/). Rule: a query two slices need lives here,
// never copied into both.
package shared

import (
	"strconv"
	"time"

	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/jsonapi"
)

// NotesRT is the JSON:API resource type of a note.
const NotesRT = "notes"

// NoteIncludes is the ?include policy of every endpoint answering with notes:
// the creator is expanded into "included" by default.
var NoteIncludes = jsonapi.IncludeConfig{
	Allowed: []string{"created_by"},
	Default: []string{"created_by"},
}

// NoteResource is a note, serialized as a JSON:API "notes" resource with a
// `created_by` relationship to a "users" resource.
type NoteResource struct {
	ID        string       `json:"id" jsonapi:"id"`
	Title     string       `json:"title"`
	Body      string       `json:"body"`
	CreatedAt time.Time    `json:"created_at"`
	UpdatedAt time.Time    `json:"updated_at"`
	CreatedBy UserResource `jsonapi:"relation,created_by,users"`
}

// UserResource is a note's creator.
type UserResource struct {
	ID    string `json:"id" jsonapi:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

// NoteFields is what NewNoteResource needs; each slice's sqlc row maps onto
// it (the rows are distinct generated types with the same columns).
type NoteFields struct {
	ID           int64
	Title        string
	Body         string
	CreatedBy    int64
	CreatedAt    time.Time
	UpdatedAt    time.Time
	CreatorName  string
	CreatorEmail string
}

// NewNoteResource builds the response resource of one note.
func NewNoteResource(f NoteFields) NoteResource {
	return NoteResource{
		ID:        strconv.FormatInt(f.ID, 10),
		Title:     f.Title,
		Body:      f.Body,
		CreatedAt: f.CreatedAt,
		UpdatedAt: f.UpdatedAt,
		CreatedBy: UserResource{
			ID:    strconv.FormatInt(f.CreatedBy, 10),
			Name:  f.CreatorName,
			Email: f.CreatorEmail,
		},
	}
}
