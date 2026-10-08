package me

import (
	"net/http"

	"github.com/gin-gonic/gin"
	internalsharedconstant "github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/constant"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/handlerutil"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/jsonapi"
)

// usersRT is the JSON:API resource type of the caller.
const usersRT = "users"

// meQuery: sparse fieldsets on the caller.
var meQuery = jsonapi.QuerySpec{Fields: map[string]any{usersRT: MeResource{}}}

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
	q, ok := handlerutil.ParseQuery(c, usersRT, meQuery)
	if !ok {
		return
	}

	item, err := h.service.Get(c, c.GetInt64(internalsharedconstant.UserIdKey))
	if handlerutil.WriteError(c, err, "Me Handler") {
		return
	}

	handlerutil.WriteData(c, http.StatusOK, usersRT, item, q, nil)
}
