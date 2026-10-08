package list

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/features/note/shared"
	internalsharedconstant "github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/constant"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/handlerutil"
)

type H interface {
	List(c *gin.Context)
}

type handler struct {
	service S
}

func NewHandler(service S) H {
	return &handler{service: service}
}

func (h *handler) List(c *gin.Context) {
	q, ok := handlerutil.ParseQuery(c, shared.NotesRT, querySpec)
	if !ok {
		return
	}

	notes, meta, err := h.service.List(c, requestFromQuery(q), c.GetInt64(internalsharedconstant.UserIdKey))
	if handlerutil.WriteError(c, err, "List Note Handler") {
		return
	}

	handlerutil.WriteData(c, http.StatusOK, shared.NotesRT, notes, q, meta)
}
