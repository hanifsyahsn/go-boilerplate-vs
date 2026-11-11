package emailverification

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/handlerutil"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/jsonapi"
)

type H interface {
	Verify(c *gin.Context)
}

type handler struct {
	service S
}

func NewHandler(service S) H {
	return &handler{service: service}
}

// Verify answers 204 with no document.
func (h *handler) Verify(c *gin.Context) {
	if _, ok := handlerutil.ParseQuery(c, ResourceType, jsonapi.QuerySpec{}); !ok {
		return
	}

	var req Request
	if !handlerutil.BindResource(c, handlerutil.ResourceSpec{Type: ResourceType}, &req) {
		return
	}

	if handlerutil.WriteError(c, h.service.Verify(c, req), "Email Verification Handler") {
		return
	}

	c.Status(http.StatusNoContent)
}
