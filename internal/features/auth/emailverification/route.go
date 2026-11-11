package emailverification

import "github.com/gin-gonic/gin"

// Route registers POST /auth/email-verifications on the limited public group
// (it takes a credential, the link's token).
func Route(rg *gin.RouterGroup, h H) {
	rg.POST("/auth/email-verifications", h.Verify)
}
