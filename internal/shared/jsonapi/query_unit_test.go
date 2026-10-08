package jsonapi

import (
	"net/url"
	"testing"

	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/apperror"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/pagination"
	"github.com/stretchr/testify/require"
)

func TestParseQuery_U(t *testing.T) {
	full := QuerySpec{
		Include: &IncludeConfig{Allowed: []string{"created_by"}, Default: []string{"created_by"}},
		Fields:  map[string]any{"notes": testNote{}, "users": testUser{}},
		Sort:    &SortConfig{Allowed: []string{"title", "created_at"}, Default: "-created_at"},
		Page:    true,
		Filters: []string{"q", "is_active"},
	}
	defaultInclude := IncludePolicy{"notes": {"created_by": {}}}

	testCases := []struct {
		name      string
		raw       string
		spec      QuerySpec
		want      Query
		wantParam string // the 400's source.parameter; "" => no error
	}{
		{
			name: "nothing given: defaults",
			spec: full,
			want: Query{Include: defaultInclude, Sort: "-created_at", Filter: map[string]string{}},
		},
		{
			name: "every family given",
			raw:  "include=&fields[notes]=title,created_by&fields[users]=name&sort=title&page[number]=2&page[size]=10&filter[q]=+hi+&filter[is_active]=true",
			spec: full,
			want: Query{
				Fields: Fieldsets{"notes": {"title": true, "created_by": true}, "users": {"name": true}},
				Sort:   "title",
				Page:   pagination.Request{Page: 2, PageSize: 10},
				Filter: map[string]string{"q": "hi", "is_active": "true"},
			},
		},
		{
			name: "an empty fieldset keeps no fields",
			raw:  "fields[notes]=",
			spec: full,
			want: Query{Include: defaultInclude, Fields: Fieldsets{"notes": {}}, Sort: "-created_at", Filter: map[string]string{}},
		},
		{
			name: "descending sort",
			raw:  "sort=-title",
			spec: full,
			want: Query{Include: defaultInclude, Sort: "-title", Filter: map[string]string{}},
		},
		{
			name: "implementation-specific parameters are ignored",
			raw:  "camelCase=1&with_underscore=2",
			spec: QuerySpec{},
			want: Query{Filter: map[string]string{}},
		},
		{name: "an unknown spec-reserved name", raw: "q=x", spec: full, wantParam: "q"},
		{name: "an unknown reserved family", raw: "search[x]=1", spec: full, wantParam: "search[x]"},
		{name: "include not supported", raw: "include=created_by", spec: QuerySpec{}, wantParam: "include"},
		{name: "include not allowed", raw: "include=bogus", spec: full, wantParam: "include"},
		{name: "fields not supported", raw: "fields[notes]=title", spec: QuerySpec{}, wantParam: "fields[notes]"},
		{name: "fields of a type not returned", raw: "fields[tags]=name", spec: full, wantParam: "fields[tags]"},
		{name: "an unknown field", raw: "fields[notes]=nope", spec: full, wantParam: "fields[notes]"},
		{name: "the id is not a field", raw: "fields[notes]=id", spec: full, wantParam: "fields[notes]"},
		{name: "fields without a type", raw: "fields=title", spec: full, wantParam: "fields"},
		{name: "sort not supported", raw: "sort=title", spec: QuerySpec{}, wantParam: "sort"},
		{name: "sort by an unknown field", raw: "sort=body", spec: full, wantParam: "sort"},
		{name: "sort by two fields", raw: "sort=title,-created_at", spec: full, wantParam: "sort"},
		{name: "page not supported", raw: "page[number]=1", spec: QuerySpec{}, wantParam: "page[number]"},
		{name: "an unknown page member", raw: "page[offset]=1", spec: full, wantParam: "page[offset]"},
		{name: "a non-numeric page", raw: "page[size]=ten", spec: full, wantParam: "page[size]"},
		{name: "a zero page", raw: "page[number]=0", spec: full, wantParam: "page[number]"},
		{name: "an unknown filter", raw: "filter[title]=x", spec: full, wantParam: "filter[title]"},
		{name: "filter without a name", raw: "filter=x", spec: full, wantParam: "filter"},
		{name: "a too-long filter", raw: "filter[q]=" + longValue(), spec: full, wantParam: "filter[q]"},
		{name: "a parameter given twice", raw: "sort=title&sort=-title", spec: full, wantParam: "sort"},
		{
			name: "a malformed bracket name is no spec parameter: ignored",
			raw:  "page[number=1",
			spec: full,
			want: Query{Include: defaultInclude, Sort: "-created_at", Filter: map[string]string{}},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			values, err := url.ParseQuery(tc.raw)
			require.NoError(t, err)

			got, appErr := ParseQuery(values, "notes", tc.spec)
			if tc.wantParam != "" {
				require.NotNil(t, appErr)
				require.Equal(t, apperror.CodeBadRequest, appErr.Code)
				require.Equal(t, &apperror.Source{Parameter: tc.wantParam}, appErr.Source)
				return
			}
			require.Nil(t, appErr)
			require.Equal(t, tc.want, got)
		})
	}
}

func longValue() string {
	b := make([]byte, MaxFilterLength+1)
	for i := range b {
		b[i] = 'a'
	}
	return string(b)
}

func TestQuery_FilterBool_U(t *testing.T) {
	yes, no := true, false

	testCases := []struct {
		name    string
		filter  map[string]string
		want    *bool
		wantErr bool
	}{
		{name: "absent", filter: map[string]string{}, want: nil},
		{name: "empty", filter: map[string]string{"is_active": ""}, want: nil},
		{name: "true", filter: map[string]string{"is_active": "true"}, want: &yes},
		{name: "false", filter: map[string]string{"is_active": "false"}, want: &no},
		{name: "not a bool", filter: map[string]string{"is_active": "maybe"}, wantErr: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Query{Filter: tc.filter}.FilterBool("is_active")
			if tc.wantErr {
				require.NotNil(t, err)
				require.Equal(t, &apperror.Source{Parameter: "filter[is_active]"}, err.Source)
				return
			}
			require.Nil(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestFieldNames_U(t *testing.T) {
	testCases := []struct {
		name     string
		resource any
		want     map[string]bool
	}{
		{
			name:     "attributes and relationships, not the id or untagged fields",
			resource: testNote{},
			want: map[string]bool{"title": true, "body": true, "created_at": true,
				"created_by": true, "editors": true, "reviewer": true},
		},
		{name: "a pointer", resource: &testCompany{}, want: map[string]bool{"name": true}},
		{name: "not a struct", resource: 42, want: map[string]bool{}},
		{name: "nil", resource: nil, want: map[string]bool{}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, FieldNames(tc.resource))
		})
	}
}
