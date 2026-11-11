package revoke

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	authshared "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/shared"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/features/session/shared"
	internalsharedconstant "github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/constant"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/handlerutil"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/jsonapi"
)

type H interface {
	Revoke(c *gin.Context)
}

type handler struct {
	service S
	cookies authshared.CookieConfig
}

func NewHandler(service S, cookies authshared.CookieConfig) H {
	return &handler{service: service, cookies: cookies}
}

// Revoke answers 204 with no document. Signing out the request's own session
// is a logout: the auth cookies are cleared too.
func (h *handler) Revoke(c *gin.Context) {
	if _, ok := handlerutil.ParseQuery(c, shared.SessionsRT, jsonapi.QuerySpec{}); !ok {
		return
	}

	id := c.Param("id")
	err := h.service.Revoke(c, c.GetInt64(internalsharedconstant.UserIDKey), id)
	if handlerutil.WriteError(c, err, "Revoke Session Handler") {
		return
	}

	current, _ := c.Get(internalsharedconstant.SessionIDKey)
	if sid, ok := current.(uuid.UUID); ok && sid.String() == id {
		authshared.ClearCookies(c, h.cookies)
	}
	c.Status(http.StatusNoContent)
}
