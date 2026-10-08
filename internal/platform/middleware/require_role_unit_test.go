package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	internalsharedconstant "github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/constant"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/enum"
	"github.com/stretchr/testify/require"
)

func TestRequireRole_U(t *testing.T) {
	testCases := []struct {
		name     string
		role     string
		allowed  []enum.Role
		wantCode int
	}{
		{name: "an allowed role", role: "admin", allowed: []enum.Role{enum.RoleAdmin}, wantCode: http.StatusOK},
		{name: "one of several allowed roles", role: "user", allowed: []enum.Role{enum.RoleAdmin, enum.RoleUser}, wantCode: http.StatusOK},
		{name: "another role", role: "user", allowed: []enum.Role{enum.RoleAdmin}, wantCode: http.StatusForbidden},
		{name: "no role", role: "", allowed: []enum.Role{enum.RoleAdmin}, wantCode: http.StatusForbidden},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			r := gin.New()
			r.Use(func(c *gin.Context) { c.Set(internalsharedconstant.UserRoleKey, tc.role); c.Next() })
			r.Use(RequireRole(tc.allowed...))
			r.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })

			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
			require.Equal(t, tc.wantCode, w.Code)
		})
	}
}
