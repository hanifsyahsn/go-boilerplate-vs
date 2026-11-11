package passwordresetrequest

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/handlerutil"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/jsonapi"
)

type H interface {
	Request(c *gin.Context)
}

type handler struct {
	service S
}

func NewHandler(service S) H {
	return &handler{service: service}
}

// Request answers 202 with a meta-only document, the same whether a link was
// sent or not.
func (h *handler) Request(c *gin.Context) {
	if _, ok := handlerutil.ParseQuery(c, ResourceType, jsonapi.QuerySpec{}); !ok {
		return
	}

	var req Request
	if !handlerutil.BindResource(c, handlerutil.ResourceSpec{Type: ResourceType}, &req) {
		return
	}

	if handlerutil.WriteError(c, h.service.Request(c, req), "Password Reset Request Handler") {
		return
	}

	handlerutil.WriteMeta(c, http.StatusAccepted, map[string]string{"message": SentMessage})
}
