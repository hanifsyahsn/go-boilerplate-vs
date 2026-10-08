package refresh

import "github.com/gin-gonic/gin"

// Route registers POST /auth/refresh on the public group (the refresh token
// is the credential).
func Route(rg *gin.RouterGroup, h H) {
	rg.POST("/auth/refresh", h.Refresh)
}
