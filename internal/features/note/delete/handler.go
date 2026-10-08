package delete

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/features/note/shared"
	internalsharedconstant "github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/constant"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/handlerutil"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/jsonapi"
)

type H interface {
	Delete(c *gin.Context)
}

type handler struct {
	service S
}

func NewHandler(service S) H {
	return &handler{service: service}
}

// Delete answers 204 with no document. It supports no query parameters.
func (h *handler) Delete(c *gin.Context) {
	id, ok := handlerutil.ParamID(c, "id", "note")
	if !ok {
		return
	}

	if _, ok = handlerutil.ParseQuery(c, shared.NotesRT, jsonapi.QuerySpec{}); !ok {
		return
	}

	err := h.service.Delete(c, id, c.GetInt64(internalsharedconstant.UserIdKey))
	if handlerutil.WriteError(c, err, "Delete Note Handler") {
		return
	}

	c.Status(http.StatusNoContent)
}
