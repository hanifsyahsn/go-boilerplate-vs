package user

import (
	"time"

	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/jsonapi"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/pagination"
)

// usersRT is the JSON:API resource type of a user.
const usersRT = "users"

var userFieldsets = map[string]any{usersRT: UserResource{}}

// listQuery is what GET /api/admin/users accepts: sparse fieldsets, ?sort
// (one field), ?page[number] / ?page[size], ?filter[q] and
// ?filter[is_active].
var listQuery = jsonapi.QuerySpec{
	Fields:  userFieldsets,
	Sort:    &jsonapi.SortConfig{Allowed: []string{"created_at", "name", "email"}, Default: "-created_at"},
	Page:    true,
	Filters: []string{"q", "is_active"},
}

// updateQuery is what PATCH /api/admin/users/:userId accepts: sparse
// fieldsets on the returned user.
var updateQuery = jsonapi.QuerySpec{Fields: userFieldsets}

// ListRequest is the directory's parsed query. Every field is optional: no
// filter lists every user, active or not.
type ListRequest struct {
	Q        string // filter[q]: matches name or email
	IsActive *bool  // filter[is_active]
	Sort     string // one listQuery sort value, e.g. "-created_at"
	pagination.Request
}

// UpdateRequest is the attributes of the PATCH /api/admin/users/:userId
// document. Only is_active is writable; left out (nil), nothing changes.
type UpdateRequest struct {
	IsActive *bool `json:"is_active" label:"Active flag"`
}

// UserResource is one user, serialized as a JSON:API "users" resource.
type UserResource struct {
	ID        string    `json:"id" jsonapi:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
