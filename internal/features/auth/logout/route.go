package logout

import "github.com/gin-gonic/gin"

// Route registers POST /auth/logout on the public group: clearing cookies
// needs no valid token (an expired session must still be able to log out).
func Route(rg *gin.RouterGroup, h H) {
	rg.POST("/auth/logout", h.Logout)
}
