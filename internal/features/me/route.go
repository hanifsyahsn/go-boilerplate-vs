package me

import "github.com/gin-gonic/gin"

// Route registers GET /me on the authenticated group: the caller's own
// profile, for the client.
func Route(rg *gin.RouterGroup, h H) {
	rg.GET("/me", h.Get)
}
