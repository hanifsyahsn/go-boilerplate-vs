package delete

import "github.com/gin-gonic/gin"

// Route registers DELETE /notes/:id on the authenticated group.
func Route(rg *gin.RouterGroup, h H) {
	rg.DELETE("/notes/:id", h.Delete)
}
