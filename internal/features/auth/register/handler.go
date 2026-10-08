package register

import (
	"net/http"

	"github.com/gin-gonic/gin"
	authshared "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/shared"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/handlerutil"
)

type H interface {
	Register(c *gin.Context)
}

type handler struct {
	service S
}

func NewHandler(service S) H {
	return &handler{service: service}
}

// Register answers 201 with the new account. There's no Location header:
// the API has no per-user URL for a plain user (GET /api/me is the caller).
func (h *handler) Register(c *gin.Context) {
	q, ok := handlerutil.ParseQuery(c, authshared.UsersRT, authshared.UserQuery)
	if !ok {
		return
	}

	var req Request
	if !handlerutil.BindResource(c, handlerutil.ResourceSpec{Type: authshared.UsersRT}, &req) {
		return
	}

	user, err := h.service.Register(c, req)
	if handlerutil.WriteError(c, err, "Register Handler") {
		return
	}

	handlerutil.WriteData(c, http.StatusCreated, authshared.UsersRT, user, q, nil)
}
