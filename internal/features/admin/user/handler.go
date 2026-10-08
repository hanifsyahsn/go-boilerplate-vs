package user

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	internalsharedconstant "github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/constant"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/handlerutil"
)

type H interface {
	List(c *gin.Context)
	Update(c *gin.Context)
}

type handler struct {
	service S
}

func NewHandler(service S) H {
	return &handler{service: service}
}

func (h *handler) List(c *gin.Context) {
	q, ok := handlerutil.ParseQuery(c, usersRT, listQuery)
	if !ok {
		return
	}
	isActive, appErr := q.FilterBool("is_active")
	if appErr != nil {
		handlerutil.WriteError(c, appErr, "Admin User Handler List")
		return
	}

	items, meta, err := h.service.List(c, ListRequest{Q: q.Filter["q"], IsActive: isActive, Sort: q.Sort, Request: q.Page})
	if handlerutil.WriteError(c, err, "Admin User Handler List") {
		return
	}

	handlerutil.WriteData(c, http.StatusOK, usersRT, items, q, meta)
}

func (h *handler) Update(c *gin.Context) {
	id, ok := handlerutil.ParamID(c, "userId", "user")
	if !ok {
		return
	}

	q, ok := handlerutil.ParseQuery(c, usersRT, updateQuery)
	if !ok {
		return
	}

	var req UpdateRequest
	if !handlerutil.BindResource(c, handlerutil.ResourceSpec{Type: usersRT, ID: strconv.FormatInt(id, 10)}, &req) {
		return
	}

	item, err := h.service.Update(c, id, req, c.GetInt64(internalsharedconstant.UserIdKey))
	if handlerutil.WriteError(c, err, "Admin User Handler Update") {
		return
	}

	handlerutil.WriteData(c, http.StatusOK, usersRT, item, q, nil)
}
