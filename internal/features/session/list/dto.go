package list

import (
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/features/session/shared"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/jsonapi"
)

// querySpec: sparse fieldsets and one-field sort. No include, filter or
// pagination — a user has a handful of live sessions.
var querySpec = jsonapi.QuerySpec{
	Fields: shared.Fieldsets,
	Sort:   &jsonapi.SortConfig{Allowed: []string{"last_used_at", "created_at"}, Default: "-last_used_at"},
}
