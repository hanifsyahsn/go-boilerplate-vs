package list

import "github.com/gin-gonic/gin"

// Route registers GET /notes on the authenticated group.
func Route(rg *gin.RouterGroup, h H) {
	rg.GET("/notes", h.List)
}
