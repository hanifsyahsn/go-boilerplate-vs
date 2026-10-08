package list

import (
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/features/note/shared"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/jsonapi"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/pagination"
)

// querySpec is what GET /api/notes accepts: ?include, sparse fieldsets,
// ?sort (one field), ?page[number] / ?page[size] and ?filter[q].
var querySpec = jsonapi.QuerySpec{
	Include: &shared.NoteIncludes,
	Fields:  shared.NoteFieldsets,
	Sort:    &jsonapi.SortConfig{Allowed: []string{"created_at", "updated_at", "title"}, Default: "-created_at"},
	Page:    true,
	Filters: []string{"q"},
}

// Request is the list's parsed query.
type Request struct {
	Q    string // filter[q]: matches title or body
	Sort string // one querySpec sort value, e.g. "-created_at"
	pagination.Request
}

// requestFromQuery maps the parsed JSON:API query onto the service request.
func requestFromQuery(q jsonapi.Query) Request {
	return Request{Q: q.Filter["q"], Sort: q.Sort, Request: q.Page}
}
