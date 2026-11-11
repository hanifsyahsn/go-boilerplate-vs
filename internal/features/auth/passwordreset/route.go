package passwordreset

import "github.com/gin-gonic/gin"

// Route registers POST /auth/password-resets on the limited public group (it
// takes a credential, the link's token).
func Route(rg *gin.RouterGroup, h H) {
	rg.POST("/auth/password-resets", h.Reset)
}
