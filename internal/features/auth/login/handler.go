package login

import (
	"net/http"

	"github.com/gin-gonic/gin"
	authshared "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/shared"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/handlerutil"
)

type H interface {
	Login(c *gin.Context)
}

type handler struct {
	service S
	cookies authshared.CookieConfig
}

func NewHandler(service S, cookies authshared.CookieConfig) H {
	return &handler{service: service, cookies: cookies}
}

// Login takes a "sessions" document with the credentials. It sets both tokens
// as HttpOnly cookies (browser clients) and answers with the account as the
// primary data and the tokens under meta (other clients).
func (h *handler) Login(c *gin.Context) {
	q, ok := handlerutil.ParseQuery(c, authshared.UsersRT, authshared.UserQuery)
	if !ok {
		return
	}

	var req Request
	if !handlerutil.BindResource(c, handlerutil.ResourceSpec{Type: authshared.SessionsRT}, &req) {
		return
	}

	res, err := h.service.Login(c, req)
	if handlerutil.WriteError(c, err, "Login Handler") {
		return
	}

	authshared.SetTokenCookies(c, h.cookies, res.Tokens)
	handlerutil.WriteData(c, http.StatusOK, authshared.UsersRT, res.User, q, authshared.NewTokenMeta(res.Tokens))
}
