package detail

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/features/note/shared"
	internalsharedconstant "github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/constant"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/handlerutil"
)

type H interface {
	Get(c *gin.Context)
}

type handler struct {
	service S
}

func NewHandler(service S) H {
	return &handler{service: service}
}

func (h *handler) Get(c *gin.Context) {
	id, ok := handlerutil.ParamID(c, "id", "note")
	if !ok {
		return
	}

	q, ok := handlerutil.ParseQuery(c, shared.NotesRT, shared.NoteQuery)
	if !ok {
		return
	}

	note, err := h.service.Get(c, id, c.GetInt64(internalsharedconstant.UserIdKey))
	if handlerutil.WriteError(c, err, "Detail Note Handler") {
		return
	}

	handlerutil.WriteData(c, http.StatusOK, shared.NotesRT, note, q, nil)
}
