package update

import "github.com/gin-gonic/gin"

// Route registers PATCH /notes/:id on the authenticated group.
func Route(rg *gin.RouterGroup, h H) {
	rg.PATCH("/notes/:id", h.Update)
}
