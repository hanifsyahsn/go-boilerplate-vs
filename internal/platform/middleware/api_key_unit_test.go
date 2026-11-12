package middleware

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/logging"
	"github.com/stretchr/testify/require"
)

func TestAPIKey_U(t *testing.T) {
	const unauthorized = `{"jsonapi":{"version":"1.1"},"errors":[{"status":"401","code":"unauthorized","title":"Unauthorized",
		"detail":"A valid API key is required.","source":{"header":"X-API-Key"}}]}`
	keys := map[string]string{
		"billing":   strings.Repeat("b", 32),
		"reporting": strings.Repeat("r", 40),
	}

	testCases := []struct {
		name       string
		keys       map[string]string
		header     string
		wantCode   int
		wantClient string
	}{
		{name: "a valid key: through, named in the context", keys: keys, header: strings.Repeat("b", 32),
			wantCode: http.StatusTeapot, wantClient: "billing"},
		{name: "one of several keys", keys: keys, header: strings.Repeat("r", 40), wantCode: http.StatusTeapot, wantClient: "reporting"},
		{name: "a missing key is a 401 on the header", keys: keys, wantCode: http.StatusUnauthorized},
		{name: "a wrong key is a 401", keys: keys, header: strings.Repeat("x", 32), wantCode: http.StatusUnauthorized},
		{name: "a key's prefix is a 401", keys: keys, header: strings.Repeat("b", 31), wantCode: http.StatusUnauthorized},
		{name: "no keys configured: nothing gets through, an empty header included", keys: map[string]string{}, header: "",
			wantCode: http.StatusUnauthorized},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			client := ""
			r := gin.New()
			r.Use(APIKey(tc.keys))
			r.GET("/x", func(c *gin.Context) { client = c.GetString(APIClientKey); c.Status(http.StatusTeapot) })

			req := httptest.NewRequest(http.MethodGet, "/x", nil)
			if tc.header != "" {
				req.Header.Set(APIKeyHeader, tc.header)
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			require.Equal(t, tc.wantCode, w.Code, w.Body.String())
			require.Equal(t, tc.wantClient, client)
			if tc.wantCode == http.StatusUnauthorized {
				require.JSONEq(t, unauthorized, w.Body.String())
			}
		})
	}
}

func TestAccessLog_APIClient_U(t *testing.T) {
	testCases := []struct {
		name       string
		header     string
		wantClient any
	}{
		{name: "the access log carries the API client's name", header: strings.Repeat("b", 32), wantClient: "billing"},
		{name: "no API client: no attribute", header: "", wantClient: nil},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			l, err := logging.New(&buf, logging.FormatJSON, "debug")
			require.NoError(t, err)
			prev := slog.Default()
			slog.SetDefault(l)
			defer slog.SetDefault(prev)

			gin.SetMode(gin.TestMode)
			r := gin.New()
			r.Use(AccessLog())
			r.GET("/machine", APIKey(map[string]string{"billing": strings.Repeat("b", 32)}), func(c *gin.Context) { c.Status(http.StatusOK) })
			r.GET("/public", func(c *gin.Context) { c.Status(http.StatusOK) })

			path := "/public"
			req := httptest.NewRequest(http.MethodGet, path, nil)
			if tc.header != "" {
				req = httptest.NewRequest(http.MethodGet, "/machine", nil)
				req.Header.Set(APIKeyHeader, tc.header)
			}
			r.ServeHTTP(httptest.NewRecorder(), req)

			var rec map[string]any
			require.NoError(t, json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &rec), buf.String())
			require.Equal(t, "request", rec["msg"])
			require.Equal(t, tc.wantClient, rec["api_client"])
			require.NotContains(t, buf.String(), strings.Repeat("b", 32), "the key itself is never logged")
		})
	}
}

func TestAPIKey_RefusalLog_U(t *testing.T) {
	sent := strings.Repeat("w", 40) // often a real key sent to the wrong service
	testCases := []struct {
		name   string
		header string
	}{
		{name: "a refused key never reaches the log", header: sent},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			l, err := logging.New(&buf, logging.FormatJSON, "debug")
			require.NoError(t, err)
			prev := slog.Default()
			slog.SetDefault(l)
			defer slog.SetDefault(prev)

			gin.SetMode(gin.TestMode)
			r := gin.New()
			r.Use(AccessLog())
			r.GET("/machine", APIKey(map[string]string{"billing": strings.Repeat("b", 32)}), func(c *gin.Context) { c.Status(http.StatusOK) })

			req := httptest.NewRequest(http.MethodGet, "/machine", nil)
			req.Header.Set(APIKeyHeader, tc.header)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			require.Equal(t, http.StatusUnauthorized, w.Code)
			require.Contains(t, buf.String(), `"msg":"request rejected"`, "the refusal is logged")
			require.NotContains(t, buf.String(), tc.header, "the key that was sent is never logged")
			require.NotContains(t, w.Body.String(), tc.header, "nor echoed in the response")
		})
	}
}
