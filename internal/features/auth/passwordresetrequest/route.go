package passwordresetrequest

import "github.com/gin-gonic/gin"

// Route registers POST /auth/password-reset-requests on the limited public
// group (it sends email).
func Route(rg *gin.RouterGroup, h H) {
	rg.POST("/auth/password-reset-requests", h.Request)
}
