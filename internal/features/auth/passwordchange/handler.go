package passwordchange

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	internalsharedconstant "github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/constant"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/handlerutil"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/jsonapi"
)

// ResourceType is the JSON:API type of the request document. It has no id.
const ResourceType = "password-changes"

// Request is the attributes of the POST /api/auth/password-changes document.
// Both passwords are bounded in bytes (bcrypt's 72).
type Request struct {
	CurrentPassword string `json:"current_password" binding:"required,checkmaxbytes=72" label:"Current password"`
	Password        string `json:"password" binding:"required,min=8,nfcmaxbytes=72" label:"New password"`
}

type H interface {
	Change(c *gin.Context)
}

type handler struct {
	service S
}

func NewHandler(service S) H {
	return &handler{service: service}
}

// Change answers 204 with no document; the caller stays signed in.
func (h *handler) Change(c *gin.Context) {
	if _, ok := handlerutil.ParseQuery(c, ResourceType, jsonapi.QuerySpec{}); !ok {
		return
	}

	var req Request
	if !handlerutil.BindResource(c, handlerutil.ResourceSpec{Type: ResourceType}, &req) {
		return
	}

	current, _ := c.Get(internalsharedconstant.SessionIDKey)
	sessionID, _ := current.(uuid.UUID) // set by AccessTokenMiddleware
	err := h.service.Change(c, c.GetInt64(internalsharedconstant.UserIDKey), sessionID, req)
	if handlerutil.WriteError(c, err, "Password Change Handler") {
		return
	}

	c.Status(http.StatusNoContent)
}
