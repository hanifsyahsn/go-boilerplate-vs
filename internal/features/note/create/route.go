package create

import "github.com/gin-gonic/gin"

// Route registers POST /notes on the authenticated group.
func Route(rg *gin.RouterGroup, h H) {
	rg.POST("/notes", h.Create)
}
