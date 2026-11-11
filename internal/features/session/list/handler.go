package list

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/features/session/shared"
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

// List answers 200 with the caller's live sessions, "current" marking the
// one this request's access token belongs to.
func (h *handler) List(c *gin.Context) {
	q, ok := handlerutil.ParseQuery(c, shared.SessionsRT, querySpec)
	if !ok {
		return
	}

	current, _ := c.Get(internalsharedconstant.SessionIDKey)
	currentSession, _ := current.(uuid.UUID) // set by AccessTokenMiddleware
	items, err := h.service.List(c, c.GetInt64(internalsharedconstant.UserIDKey), currentSession, q.Sort)
	if handlerutil.WriteError(c, err, "List Sessions Handler") {
		return
	}

	handlerutil.WriteData(c, http.StatusOK, shared.SessionsRT, items, q, nil)
}
