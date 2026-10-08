package create

import (
	"net/http"
	"path"

	"github.com/gin-gonic/gin"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/features/note/shared"
	internalsharedconstant "github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/constant"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/handlerutil"
)

type H interface {
	Create(c *gin.Context)
}

type handler struct {
	service S
}

func NewHandler(service S) H {
	return &handler{service: service}
}

// Create answers 201 with the new note and a Location header naming it.
func (h *handler) Create(c *gin.Context) {
	q, ok := handlerutil.ParseQuery(c, shared.NotesRT, shared.NoteQuery)
	if !ok {
		return
	}

	var req Request
	if !handlerutil.BindResource(c, handlerutil.ResourceSpec{Type: shared.NotesRT}, &req) {
		return
	}

	note, err := h.service.Create(c, req, c.GetInt64(internalsharedconstant.UserIdKey))
	if handlerutil.WriteError(c, err, "Create Note Handler") {
		return
	}

	c.Header("Location", path.Join(c.Request.URL.Path, note.ID))
	handlerutil.WriteData(c, http.StatusCreated, shared.NotesRT, note, q, nil)
}
