package update

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/features/note/shared"
	internalsharedconstant "github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/constant"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/handlerutil"
)

type H interface {
	Update(c *gin.Context)
}

type handler struct {
	service S
}

func NewHandler(service S) H {
	return &handler{service: service}
}

func (h *handler) Update(c *gin.Context) {
	id, ok := handlerutil.ParamID(c, "id", "note")
	if !ok {
		return
	}

	q, ok := handlerutil.ParseQuery(c, shared.NotesRT, shared.NoteQuery)
	if !ok {
		return
	}

	var req Request
	spec := handlerutil.ResourceSpec{Type: shared.NotesRT, ID: strconv.FormatInt(id, 10)}
	if !handlerutil.BindResource(c, spec, &req) {
		return
	}

	note, err := h.service.Update(c, id, req, c.GetInt64(internalsharedconstant.UserIdKey))
	if handlerutil.WriteError(c, err, "Update Note Handler") {
		return
	}

	handlerutil.WriteData(c, http.StatusOK, shared.NotesRT, note, q, nil)
}
