package shared

import "github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/jsonapi"

// UsersRT is the JSON:API resource type of a note's creator.
const UsersRT = "users"

// NoteFieldsets lists the resource types a note response carries, for
// sparse fieldsets (?fields[notes]=title&fields[users]=name).
var NoteFieldsets = map[string]any{NotesRT: NoteResource{}, UsersRT: UserResource{}}

// NoteQuery is the query support of every endpoint answering with a single
// note: ?include (created_by by default) and sparse fieldsets.
var NoteQuery = jsonapi.QuerySpec{
	Include: &NoteIncludes,
	Fields:  NoteFieldsets,
}
