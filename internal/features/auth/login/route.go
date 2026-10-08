package login

import "github.com/gin-gonic/gin"

// Route registers POST /auth/login on the public group.
func Route(rg *gin.RouterGroup, h H) {
	rg.POST("/auth/login", h.Login)
}
