package jsonapi

import (
	"net/url"
	"strconv"

	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/pagination"
)

// PaginationLinks builds first / last / prev / next for one page of a
// collection fetched at u, keeping the request's other query parameters. An
// unavailable link (prev on the first page, next on the last) is null.
func PaginationLinks(u *url.URL, m pagination.Meta) Links {
	last := m.TotalPage
	if last < 1 {
		last = 1
	}

	page := func(n int) string {
		q := u.Query()
		q.Set("page[number]", strconv.Itoa(n))
		q.Set("page[size]", strconv.Itoa(m.PageSize))
		return u.Path + "?" + q.Encode()
	}

	links := Links{"first": page(1), "last": page(last), "prev": nil, "next": nil}
	if m.Page > 1 {
		prev := m.Page - 1
		if prev > last {
			prev = last
		}
		links["prev"] = page(prev)
	}
	if m.Page < last {
		links["next"] = page(m.Page + 1)
	}
	return links
}
