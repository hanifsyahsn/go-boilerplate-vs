package handlerutil

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/apperror"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/jsonapi"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/validation"
	"github.com/stretchr/testify/require"
)

type testAttrs struct {
	Name *string `json:"name" binding:"omitempty,notblank,max=5" label:"Name"`
	Age  int     `json:"age" binding:"min=0" label:"Age"`
}

type requiredAttrs struct {
	Name string `json:"name" binding:"required" label:"Name"`
}

func errorDoc(t *testing.T, body string) jsonapi.ErrorDocument {
	t.Helper()
	var doc jsonapi.ErrorDocument
	require.NoError(t, json.Unmarshal([]byte(body), &doc))
	return doc
}

func TestBindResource_U(t *testing.T) {
	require.NoError(t, validation.RegisterGinValidators())
	name := "Ann"

	testCases := []struct {
		name        string
		spec        ResourceSpec
		body        string
		required    bool // decode into requiredAttrs instead of testAttrs
		wantOK      bool
		wantAttrs   testAttrs
		wantStatus  int
		wantPointer string
	}{
		{
			name:      "a create: attributes decoded",
			spec:      ResourceSpec{Type: "people"},
			body:      `{"data":{"type":"people","attributes":{"name":"Ann","age":3}}}`,
			wantOK:    true,
			wantAttrs: testAttrs{Name: &name, Age: 3},
		},
		{
			name:      "an update with the matching id, partial attributes",
			spec:      ResourceSpec{Type: "people", ID: "7"},
			body:      `{"data":{"type":"people","id":"7","attributes":{"age":4}}}`,
			wantOK:    true,
			wantAttrs: testAttrs{Age: 4},
		},
		{
			name:   "an update with no attributes at all: a no-op",
			spec:   ResourceSpec{Type: "people", ID: "7"},
			body:   `{"data":{"type":"people","id":"7"}}`,
			wantOK: true,
		},
		{
			name:      "a local id (lid) and top-level meta are accepted",
			spec:      ResourceSpec{Type: "people"},
			body:      `{"data":{"type":"people","lid":"tmp-1","attributes":{"age":1}},"meta":{"x":1}}`,
			wantOK:    true,
			wantAttrs: testAttrs{Age: 1},
		},
		{name: "optional and no body", spec: ResourceSpec{Type: "people", Optional: true}, wantOK: true},
		{name: "required body missing", spec: ResourceSpec{Type: "people"}, body: "", wantStatus: http.StatusBadRequest},
		{name: "not JSON", spec: ResourceSpec{Type: "people"}, body: `{`, wantStatus: http.StatusBadRequest},
		{name: "no data", spec: ResourceSpec{Type: "people"}, body: `{"meta":{}}`, wantStatus: http.StatusBadRequest, wantPointer: "/data"},
		{name: "null data", spec: ResourceSpec{Type: "people"}, body: `{"data":null}`, wantStatus: http.StatusBadRequest, wantPointer: "/data"},
		{name: "data is an array", spec: ResourceSpec{Type: "people"}, body: `{"data":[]}`, wantStatus: http.StatusBadRequest, wantPointer: "/data"},
		{name: "plain JSON, not a document", spec: ResourceSpec{Type: "people"}, body: `{"name":"Ann"}`, wantStatus: http.StatusBadRequest, wantPointer: "/data"},
		{name: "no type", spec: ResourceSpec{Type: "people"}, body: `{"data":{"attributes":{}}}`, wantStatus: http.StatusBadRequest, wantPointer: "/data/type"},
		{name: "another type", spec: ResourceSpec{Type: "people"}, body: `{"data":{"type":"pets"}}`, wantStatus: http.StatusConflict, wantPointer: "/data/type"},
		{name: "a client id on a create", spec: ResourceSpec{Type: "people"}, body: `{"data":{"type":"people","id":"1"}}`, wantStatus: http.StatusForbidden, wantPointer: "/data/id"},
		{name: "no id on an update", spec: ResourceSpec{Type: "people", ID: "7"}, body: `{"data":{"type":"people"}}`, wantStatus: http.StatusBadRequest, wantPointer: "/data/id"},
		{name: "another id on an update", spec: ResourceSpec{Type: "people", ID: "7"}, body: `{"data":{"type":"people","id":"8"}}`, wantStatus: http.StatusConflict, wantPointer: "/data/id"},
		{
			name: "relationships", spec: ResourceSpec{Type: "people"},
			body:       `{"data":{"type":"people","relationships":{"owner":{"data":null}}}}`,
			wantStatus: http.StatusForbidden, wantPointer: "/data/relationships",
		},
		{
			name: "an attribute that can't be set", spec: ResourceSpec{Type: "people"},
			body:       `{"data":{"type":"people","attributes":{"role":"admin"}}}`,
			wantStatus: http.StatusForbidden, wantPointer: "/data/attributes/role",
		},
		{
			name: "an attribute of the wrong type", spec: ResourceSpec{Type: "people"},
			body:       `{"data":{"type":"people","attributes":{"age":"three"}}}`,
			wantStatus: http.StatusUnprocessableEntity, wantPointer: "/data/attributes/age",
		},
		{
			name: "attributes not an object", spec: ResourceSpec{Type: "people"},
			body:       `{"data":{"type":"people","attributes":[1]}}`,
			wantStatus: http.StatusBadRequest, wantPointer: "/data/attributes",
		},
		{
			name: "a failed rule points at the attribute", spec: ResourceSpec{Type: "people"},
			body:       `{"data":{"type":"people","attributes":{"name":"toolong"}}}`,
			wantStatus: http.StatusUnprocessableEntity, wantPointer: "/data/attributes/name",
		},
		{
			name: "a missing required attribute", spec: ResourceSpec{Type: "people"}, required: true,
			body:       `{"data":{"type":"people"}}`,
			wantStatus: http.StatusUnprocessableEntity, wantPointer: "/data/attributes/name",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			var body *strings.Reader
			if tc.body != "" {
				body = strings.NewReader(tc.body)
				c.Request = httptest.NewRequest(http.MethodPost, "/", body)
			} else {
				c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
			}

			var attrs testAttrs
			var target any = &attrs
			if tc.required {
				target = &requiredAttrs{}
			}

			ok := BindResource(c, tc.spec, target)
			require.Equal(t, tc.wantOK, ok, w.Body.String())
			if tc.wantOK {
				require.Equal(t, tc.wantAttrs, attrs)
				return
			}

			require.Equal(t, tc.wantStatus, w.Code)
			doc := errorDoc(t, w.Body.String())
			require.NotEmpty(t, doc.Errors)
			if tc.wantPointer != "" {
				require.Equal(t, &apperror.Source{Pointer: tc.wantPointer}, doc.Errors[0].Source)
			}
		})
	}
}

func TestParseQuery_U(t *testing.T) {
	testCases := []struct {
		name       string
		target     string
		wantOK     bool
		wantParam  string
		wantFilter map[string]string
	}{
		{name: "supported parameters", target: "/?filter%5Bq%5D=hi", wantOK: true, wantFilter: map[string]string{"q": "hi"}},
		{name: "an unsupported one is a 400", target: "/?sort=name", wantParam: "sort"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodGet, tc.target, nil)

			q, ok := ParseQuery(c, "people", jsonapi.QuerySpec{Filters: []string{"q"}})
			require.Equal(t, tc.wantOK, ok)
			if tc.wantOK {
				require.Equal(t, tc.wantFilter, q.Filter)
				return
			}
			require.Equal(t, http.StatusBadRequest, w.Code)
			require.Equal(t, &apperror.Source{Parameter: tc.wantParam}, errorDoc(t, w.Body.String()).Errors[0].Source)
		})
	}
}

func TestParamID_U(t *testing.T) {
	testCases := []struct {
		name   string
		value  string
		wantID int64
		wantOK bool
	}{
		{name: "positive id", value: "12", wantID: 12, wantOK: true},
		{name: "zero", value: "0"},
		{name: "negative", value: "-3"},
		{name: "not a number", value: "abc"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Params = gin.Params{{Key: "id", Value: tc.value}}

			id, ok := ParamID(c, "id", "note")
			require.Equal(t, tc.wantOK, ok)
			require.Equal(t, tc.wantID, id)
			if !tc.wantOK {
				require.Equal(t, http.StatusNotFound, w.Code)
				require.Contains(t, w.Body.String(), "That note was not found.")
			}
		})
	}
}

func TestWriteData_U(t *testing.T) {
	type person struct {
		ID   string `jsonapi:"id"`
		Name string `json:"name"`
	}

	testCases := []struct {
		name   string
		write  func(c *gin.Context)
		status int
		want   string
	}{
		{
			name: "a resource",
			write: func(c *gin.Context) {
				WriteData(c, http.StatusCreated, "people", person{ID: "1", Name: "Ann"}, jsonapi.Query{}, nil)
			},
			status: http.StatusCreated,
			want:   `{"jsonapi":{"version":"1.1"},"data":{"type":"people","id":"1","attributes":{"name":"Ann"}},"links":{"self":"/p"}}`,
		},
		{
			name:   "meta only",
			write:  func(c *gin.Context) { WriteMeta(c, http.StatusOK, map[string]int{"n": 1}) },
			status: http.StatusOK,
			want:   `{"jsonapi":{"version":"1.1"},"meta":{"n":1},"links":{"self":"/p"}}`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodGet, "/p", nil)

			tc.write(c)
			require.Equal(t, tc.status, w.Code)
			require.Equal(t, jsonapi.MediaType, w.Header().Get("Content-Type"))
			require.JSONEq(t, tc.want, w.Body.String())
		})
	}
}

func TestWriteError_U(t *testing.T) {
	testCases := []struct {
		name        string
		err         error
		wantWrote   bool
		wantCode    int
		wantInBody  string
		wantPointer string
	}{
		{name: "no error", err: nil, wantWrote: false},
		{name: "app error kept", err: apperror.New(apperror.CodeNotFound, "Not here.", nil), wantWrote: true, wantCode: http.StatusNotFound, wantInBody: "Not here."},
		{
			name:      "validation errors: a 422 per field",
			err:       apperror.ValidationErrors{{Detail: "Title is required.", Pointer: "/data/attributes/title"}},
			wantWrote: true, wantCode: http.StatusUnprocessableEntity, wantInBody: "Title is required.",
			wantPointer: "/data/attributes/title",
		},
		{name: "other error is a 500", err: errors.New("boom"), wantWrote: true, wantCode: http.StatusInternalServerError, wantInBody: "Something went wrong"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			require.Equal(t, tc.wantWrote, WriteError(c, tc.err, "Test Handler"))
			if !tc.wantWrote {
				return
			}
			require.Equal(t, tc.wantCode, w.Code)
			require.Contains(t, w.Body.String(), tc.wantInBody)
			if tc.wantPointer != "" {
				require.Equal(t, &apperror.Source{Pointer: tc.wantPointer}, errorDoc(t, w.Body.String()).Errors[0].Source)
			}
		})
	}
}
