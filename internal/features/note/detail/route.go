package detail

import "github.com/gin-gonic/gin"

// Route registers GET /notes/:id on the authenticated group.
func Route(rg *gin.RouterGroup, h H) {
	rg.GET("/notes/:id", h.Get)
}
