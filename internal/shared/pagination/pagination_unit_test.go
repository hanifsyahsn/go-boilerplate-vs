package pagination

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRequest_Resolve_U(t *testing.T) {
	testCases := []struct {
		name                   string
		req                    Request
		wantPage, wantPageSize int
		wantLimit, wantOffset  int32
	}{
		{name: "defaults", req: Request{}, wantPage: 1, wantPageSize: 20, wantLimit: 20, wantOffset: 0},
		{name: "explicit page", req: Request{Page: 3, PageSize: 10}, wantPage: 3, wantPageSize: 10, wantLimit: 10, wantOffset: 20},
		{name: "negative page is page 1", req: Request{Page: -2, PageSize: 5}, wantPage: 1, wantPageSize: 5, wantLimit: 5, wantOffset: 0},
		{name: "page_size capped", req: Request{Page: 2, PageSize: 1000}, wantPage: 2, wantPageSize: 100, wantLimit: 100, wantOffset: 100},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			page, pageSize, limit, offset := tc.req.Resolve()
			require.Equal(t, tc.wantPage, page)
			require.Equal(t, tc.wantPageSize, pageSize)
			require.Equal(t, tc.wantLimit, limit)
			require.Equal(t, tc.wantOffset, offset)
		})
	}
}

func TestNewMeta_U(t *testing.T) {
	testCases := []struct {
		name          string
		page, size    int
		total         int64
		wantTotalPage int
	}{
		{name: "no rows", page: 1, size: 20, total: 0, wantTotalPage: 0},
		{name: "exact pages", page: 1, size: 10, total: 30, wantTotalPage: 3},
		{name: "partial last page", page: 1, size: 10, total: 31, wantTotalPage: 4},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := NewMeta(tc.page, tc.size, tc.total)
			require.Equal(t, Meta{Page: tc.page, PageSize: tc.size, Total: tc.total, TotalPage: tc.wantTotalPage}, got)
		})
	}
}
