package passwordchange

import "github.com/gin-gonic/gin"

// Route registers POST /auth/password-changes on the authenticated group.
func Route(rg *gin.RouterGroup, h H) {
	rg.POST("/auth/password-changes", h.Change)
}
