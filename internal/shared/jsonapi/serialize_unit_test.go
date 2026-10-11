package jsonapi

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type testCompany struct {
	ID   int64  `jsonapi:"id"`
	Name string `json:"name"`
}

type testUser struct {
	ID      int64        `jsonapi:"id"`
	Name    string       `json:"name"`
	Company *testCompany `jsonapi:"relation,company,companies"`
}

type testNote struct {
	ID        string     `json:"id" jsonapi:"id"`
	Title     string     `json:"title"`
	Body      *string    `json:"body,omitempty"`
	Secret    string     `json:"-"`
	Internal  string     // no json tag: not an attribute
	CreatedAt time.Time  `json:"created_at"`
	CreatedBy testUser   `jsonapi:"relation,created_by,users"`
	Editors   []testUser `jsonapi:"relation,editors,users"`
	Reviewer  *testUser  `jsonapi:"relation,reviewer,users"`
}

func TestBuild_U(t *testing.T) {
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	body := "text"
	acme := &testCompany{ID: 9, Name: "Acme"}
	jane := testUser{ID: 1, Name: "Jane", Company: acme}
	john := testUser{ID: 2, Name: "John"}
	note := testNote{ID: "5", Title: "T", Body: &body, Secret: "s", Internal: "i", CreatedAt: at, CreatedBy: jane, Editors: []testUser{jane, john}}

	janeRes := Resource{Type: "users", ID: "1", Attributes: map[string]any{"name": "Jane"},
		Relationships: map[string]Relationship{"company": {Data: ResourceIdentifier{Type: "companies", ID: "9"}}}}
	johnRes := Resource{Type: "users", ID: "2", Attributes: map[string]any{"name": "John"},
		Relationships: map[string]Relationship{"company": {Data: nil}}}
	noteRels := map[string]Relationship{
		"created_by": {Data: ResourceIdentifier{Type: "users", ID: "1"}},
		"editors":    {Data: []ResourceIdentifier{{Type: "users", ID: "1"}, {Type: "users", ID: "2"}}},
		"reviewer":   {Data: nil},
	}
	noteRes := Resource{Type: "notes", ID: "5",
		Attributes:    map[string]any{"title": "T", "body": &body, "created_at": at},
		Relationships: noteRels}

	testCases := []struct {
		name         string
		resourceType string
		data         any
		include      IncludePolicy
		fields       Fieldsets
		want         Document
	}{
		{
			name:         "a single resource: attributes, to-one, to-many, null to-one; nothing included",
			resourceType: "notes",
			data:         note,
			want:         Document{JSONAPI: versionObject, Data: noteRes},
		},
		{
			name:         "a pointer to a resource with an include: each related resource once",
			resourceType: "notes",
			data:         &note,
			include:      IncludePolicy{"notes": {"created_by": {}, "editors": {}}},
			want:         Document{JSONAPI: versionObject, Data: noteRes, Included: []Resource{janeRes, johnRes}},
		},
		{
			name:         "a nested include path reaches through a relationship",
			resourceType: "notes",
			data:         note,
			include:      IncludePolicy{"notes": {"created_by": {"company": {}}}},
			want: Document{JSONAPI: versionObject, Data: noteRes, Included: []Resource{
				{Type: "companies", ID: "9", Attributes: map[string]any{"name": "Acme"}, Relationships: map[string]Relationship{}},
				janeRes,
			}},
		},
		{
			name:         "a collection; an included resource that's also primary data is not repeated",
			resourceType: "users",
			data:         []testUser{jane, john},
			include:      IncludePolicy{"users": {"company": {}}},
			want: Document{JSONAPI: versionObject, Data: []Resource{janeRes, johnRes}, Included: []Resource{
				{Type: "companies", ID: "9", Attributes: map[string]any{"name": "Acme"}, Relationships: map[string]Relationship{}},
			}},
		},
		{
			name:         "an empty collection is []",
			resourceType: "users",
			data:         []testUser{},
			want:         Document{JSONAPI: versionObject, Data: []Resource{}},
		},
		{
			name:         "sparse fieldsets trim attributes and relationships per type",
			resourceType: "notes",
			data:         note,
			fields:       Fieldsets{"notes": {"title": true, "created_by": true}},
			want: Document{JSONAPI: versionObject, Data: Resource{Type: "notes", ID: "5",
				Attributes:    map[string]any{"title": "T"},
				Relationships: map[string]Relationship{"created_by": noteRels["created_by"]}}},
		},
		{
			name:         "a sparse fieldset applies to included resources too",
			resourceType: "notes",
			data:         note,
			include:      IncludePolicy{"notes": {"created_by": {}}},
			fields:       Fieldsets{"notes": {}, "users": {"name": true}},
			want: Document{JSONAPI: versionObject,
				Data: Resource{Type: "notes", ID: "5", Attributes: map[string]any{}, Relationships: map[string]Relationship{}},
				Included: []Resource{{Type: "users", ID: "1", Attributes: map[string]any{"name": "Jane"},
					Relationships: map[string]Relationship{}}}},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, Build(tc.resourceType, tc.data, tc.include, tc.fields))
		})
	}
}

type testNode struct {
	ID     int64     `jsonapi:"id"`
	Parent *testNode `jsonapi:"relation,parent,nodes"`
}

func TestBuild_Pointers_U(t *testing.T) {
	acme := &testCompany{ID: 9, Name: "Acme"}
	shared := &testUser{ID: 1, Name: "Jane", Company: acme}
	loop := &testNode{ID: 1}
	loop.Parent = loop

	reviewer := func(id string) Relationship { return Relationship{Data: ResourceIdentifier{Type: "users", ID: id}} }

	testCases := []struct {
		name         string
		resourceType string
		data         any
		include      IncludePolicy
		assert       func(t *testing.T, doc Document)
	}{
		{
			name:         "one pointer shared by two resources is linked from both",
			resourceType: "notes",
			data:         []testNote{{ID: "1", Reviewer: shared}, {ID: "2", Reviewer: shared}},
			include:      IncludePolicy{"notes": {"reviewer": {}}},
			assert: func(t *testing.T, doc Document) {
				data := doc.Data.([]Resource)
				require.Equal(t, reviewer("1"), data[0].Relationships["reviewer"])
				require.Equal(t, reviewer("1"), data[1].Relationships["reviewer"])
				require.Len(t, doc.Included, 1)
			},
		},
		{
			name:         "a real cycle is cut",
			resourceType: "nodes",
			data:         loop,
			include:      IncludePolicy{"nodes": {"parent": {}}},
			assert: func(t *testing.T, doc Document) {
				require.Equal(t, Relationship{Data: nil}, doc.Data.(Resource).Relationships["parent"])
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.assert(t, Build(tc.resourceType, tc.data, tc.include, nil))
		})
	}
}
