package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/golang/mock/gomock"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/jwttoken"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/logging"
	middlewaresqlc "github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/middleware/sqlc"
	"github.com/stretchr/testify/require"
)

// TestAccessTokenMiddleware_LogContext_U checks the middleware logs with the
// request's own context: a DB error on a request the client abandoned is
// "request cancelled" at warn, not a server error.
func TestAccessTokenMiddleware_LogContext_U(t *testing.T) {
	testCases := []struct {
		name      string
		cancelled bool
		dbErr     error
		wantLevel string
		wantMsg   string
	}{
		{name: "a live request's DB error is request failed", dbErr: errors.New("pq: connection refused"), wantLevel: "ERROR", wantMsg: "request failed"},
		{
			name: "a cancelled request's DB error is request cancelled", cancelled: true,
			dbErr: errors.New("pq: canceling statement due to user request"), wantLevel: "WARN", wantMsg: "request cancelled",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			l, err := logging.New(&buf, logging.FormatJSON, "debug")
			require.NoError(t, err)
			prev := slog.Default()
			slog.SetDefault(l)
			defer slog.SetDefault(prev)

			ctrl := gomock.NewController(t)
			maker := jwttoken.NewMockM(ctrl)
			users := middlewaresqlc.NewMockQuerier(ctrl)
			maker.EXPECT().VerifyAccessToken("good").Return(jwt.MapClaims{"sub": float64(7)}, nil)
			users.EXPECT().GetUserIsActive(gomock.Any(), int64(7)).Return(false, tc.dbErr)

			gin.SetMode(gin.TestMode)
			r := gin.New()
			r.Use(AccessTokenMiddleware(maker, users))
			r.GET("/x", func(c *gin.Context) { c.Status(http.StatusTeapot) })

			req := httptest.NewRequest(http.MethodGet, "/x", nil)
			req.Header.Set("Authorization", "Bearer good")
			if tc.cancelled {
				ctx, cancel := context.WithCancel(req.Context())
				cancel()
				req = req.WithContext(ctx)
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			require.Equal(t, http.StatusInternalServerError, w.Code)
			var rec map[string]any
			require.NoError(t, json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &rec), buf.String())
			require.Equal(t, tc.wantLevel, rec["level"])
			require.Equal(t, tc.wantMsg, rec["msg"])
		})
	}
}
