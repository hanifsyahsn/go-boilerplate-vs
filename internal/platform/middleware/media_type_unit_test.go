package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/jsonapi"
	"github.com/stretchr/testify/require"
)

func TestJSONAPIMediaType_U(t *testing.T) {
	testCases := []struct {
		name        string
		method      string
		body        string
		contentType string
		accept      string
		wantCode    int
	}{
		{name: "a GET with no headers", method: http.MethodGet, wantCode: http.StatusTeapot},
		{name: "Accept */*", method: http.MethodGet, accept: "*/*", wantCode: http.StatusTeapot},
		{name: "Accept the JSON:API media type", method: http.MethodGet, accept: jsonapi.MediaType, wantCode: http.StatusTeapot},
		{name: "Accept with a profile", method: http.MethodGet, accept: jsonapi.MediaType + `; profile="https://example.com/p"`, wantCode: http.StatusTeapot},
		{name: "Accept with q", method: http.MethodGet, accept: jsonapi.MediaType + ";q=0.9", wantCode: http.StatusTeapot},
		{
			name: "one usable instance among unusable ones", method: http.MethodGet,
			accept: jsonapi.MediaType + ";charset=utf-8, " + jsonapi.MediaType, wantCode: http.StatusTeapot,
		},
		{name: "Accept only with an unsupported parameter: 406", method: http.MethodGet, accept: jsonapi.MediaType + ";charset=utf-8", wantCode: http.StatusNotAcceptable},
		{name: "Accept only with an extension: 406", method: http.MethodGet, accept: jsonapi.MediaType + `;ext="https://jsonapi.org/ext/atomic"`, wantCode: http.StatusNotAcceptable},
		{name: "Accept not naming JSON:API is fine", method: http.MethodGet, accept: "application/json", wantCode: http.StatusTeapot},
		{name: "a JSON:API body", method: http.MethodPost, body: `{}`, contentType: jsonapi.MediaType, wantCode: http.StatusTeapot},
		{name: "a JSON:API body with a profile", method: http.MethodPost, body: `{}`, contentType: jsonapi.MediaType + `; profile="https://example.com/p"`, wantCode: http.StatusTeapot},
		{name: "a plain JSON body: 415", method: http.MethodPost, body: `{}`, contentType: "application/json", wantCode: http.StatusUnsupportedMediaType},
		{name: "a body with no Content-Type: 415", method: http.MethodPost, body: `{}`, wantCode: http.StatusUnsupportedMediaType},
		{name: "a JSON:API body with charset: 415", method: http.MethodPost, body: `{}`, contentType: jsonapi.MediaType + ";charset=utf-8", wantCode: http.StatusUnsupportedMediaType},
		{name: "a JSON:API body with an extension: 415", method: http.MethodPost, body: `{}`, contentType: jsonapi.MediaType + `;ext="https://jsonapi.org/ext/atomic"`, wantCode: http.StatusUnsupportedMediaType},
		{name: "a POST without a body needs no Content-Type", method: http.MethodPost, wantCode: http.StatusTeapot},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			r := gin.New()
			r.Use(JSONAPIMediaType())
			r.Any("/x", func(c *gin.Context) { c.Status(http.StatusTeapot) })

			var req *http.Request
			if tc.body != "" {
				req = httptest.NewRequest(tc.method, "/x", strings.NewReader(tc.body))
			} else {
				req = httptest.NewRequest(tc.method, "/x", nil)
			}
			if tc.contentType != "" {
				req.Header.Set("Content-Type", tc.contentType)
			}
			if tc.accept != "" {
				req.Header.Set("Accept", tc.accept)
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			require.Equal(t, tc.wantCode, w.Code, w.Body.String())
			require.Equal(t, "Accept", w.Header().Get("Vary"))
			if tc.wantCode != http.StatusTeapot {
				require.Equal(t, jsonapi.MediaType, w.Header().Get("Content-Type"))
				require.Contains(t, w.Body.String(), `"source":{"header":`)
			}
		})
	}
}
