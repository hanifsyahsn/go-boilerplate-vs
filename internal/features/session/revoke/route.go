package revoke

import "github.com/gin-gonic/gin"

// Route registers DELETE /me/sessions/:id on the authenticated group: sign a
// device out.
func Route(rg *gin.RouterGroup, h H) {
	rg.DELETE("/me/sessions/:id", h.Revoke)
}
