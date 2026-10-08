package register

import "github.com/gin-gonic/gin"

// Route registers POST /auth/register on the public group.
func Route(rg *gin.RouterGroup, h H) {
	rg.POST("/auth/register", h.Register)
}
