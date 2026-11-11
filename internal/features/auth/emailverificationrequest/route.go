package emailverificationrequest

import "github.com/gin-gonic/gin"

// Route registers POST /me/email-verification-requests on the authenticated
// group.
func Route(rg *gin.RouterGroup, h H) {
	rg.POST("/me/email-verification-requests", h.Request)
}
