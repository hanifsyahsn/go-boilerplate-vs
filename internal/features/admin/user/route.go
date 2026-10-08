package user

import "github.com/gin-gonic/gin"

// Route registers the user-directory endpoints on the admin group (mounted at
// /api/admin, gated by middleware.RequireRole(enum.RoleAdmin)).
func Route(rg *gin.RouterGroup, h H) {
	rg.GET("/users", h.List)
	rg.PATCH("/users/:userId", h.Update)
}
