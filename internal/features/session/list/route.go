package list

import "github.com/gin-gonic/gin"

// Route registers GET /me/sessions on the authenticated group: the caller's
// signed-in devices.
func Route(rg *gin.RouterGroup, h H) {
	rg.GET("/me/sessions", h.List)
}
