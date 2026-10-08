// Package pagination is the page[number] / page[size] contract shared by every
// paginated list endpoint, and the "meta" envelope it answers with.
package pagination

const (
	DefaultPageSize = 20
	MaxPageSize     = 100
)

// Request is embedded in a list endpoint's query struct. Out-of-range values
// are clamped, never rejected.
type Request struct {
	Page     int
	PageSize int
}

// Resolve clamps page[number] / page[size] into range and returns the values actually
// used plus the SQL limit / offset.
func (r Request) Resolve() (page, pageSize int, limit, offset int32) {
	page = r.Page
	if page < 1 {
		page = 1
	}

	pageSize = r.PageSize
	if pageSize < 1 {
		pageSize = DefaultPageSize
	}
	if pageSize > MaxPageSize {
		pageSize = MaxPageSize
	}

	return page, pageSize, int32(pageSize), int32((page - 1) * pageSize)
}

// Meta is the pagination envelope, written under the response "meta" object.
type Meta struct {
	Page      int   `json:"page"`
	PageSize  int   `json:"page_size"`
	Total     int64 `json:"total"`
	TotalPage int   `json:"total_page"`
}

// NewMeta builds the envelope for one page of a result of total rows.
func NewMeta(page, pageSize int, total int64) Meta {
	totalPage := 0
	if total > 0 {
		totalPage = int((total + int64(pageSize) - 1) / int64(pageSize))
	}
	return Meta{Page: page, PageSize: pageSize, Total: total, TotalPage: totalPage}
}
