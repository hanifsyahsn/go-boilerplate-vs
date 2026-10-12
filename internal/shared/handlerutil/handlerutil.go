// Package handlerutil holds the JSON:API steps every feature handler repeats,
// so a handler only does: parse query → bind body → call service → write
// response.
package handlerutil

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/apperror"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/jsonapi"
)

// ParseQuery parses the request's JSON:API query parameters against what the
// endpoint supports. On failure it writes the 400 and returns false. Call it
// first, before any side effect, so a bad parameter never half-runs a write.
func ParseQuery(c *gin.Context, primaryType string, spec jsonapi.QuerySpec) (jsonapi.Query, bool) {
	q, err := jsonapi.ParseQuery(c.Request.URL.Query(), primaryType, spec)
	if err != nil {
		jsonapi.WriteErrorsContext(RequestContext(c), c.Writer, err)
		return jsonapi.Query{}, false
	}
	return q, true
}

// BindResource reads a JSON:API request document whose primary data is one
// resource object of spec.Type, decodes its attributes into attrs and
// validates them (gin binding tags). On failure it writes the error response
// and returns false. See DecodeResource for the rules.
func BindResource(c *gin.Context, spec ResourceSpec, attrs any) bool {
	if spec.Optional && !hasBody(c.Request) {
		return true
	}
	if err := DecodeResource(c.Request.Body, spec, attrs); err != nil {
		WriteError(c, err, "BindResource")
		return false
	}
	return true
}

func hasBody(r *http.Request) bool {
	return r.Body != nil && r.Body != http.NoBody && r.ContentLength != 0
}

// ParamID parses a positive int64 path parameter. A value that isn't one
// can't name any resource, so it's a 404 naming label ("note", "user", …);
// it writes that and returns false.
func ParamID(c *gin.Context, name, label string) (int64, bool) {
	id, err := strconv.ParseInt(c.Param(name), 10, 64)
	if err != nil || id <= 0 {
		jsonapi.WriteErrorsContext(RequestContext(c), c.Writer, apperror.New(apperror.CodeNotFound,
			fmt.Sprintf("That %s was not found.", label),
			fmt.Errorf("strconv.ParseInt(%q): invalid :%s", c.Param(name), name)))
		return 0, false
	}
	return id, true
}

// WriteData writes a resource (or a collection) as a JSON:API document shaped
// by the request's query: include, sparse fieldsets, links (and pagination
// links when meta is a pagination.Meta).
func WriteData(c *gin.Context, status int, resourceType string, data any, q jsonapi.Query, meta any) {
	jsonapi.WriteData(c.Writer, c.Request, status, resourceType, data, q, meta)
}

// WriteMeta writes a meta-only JSON:API document.
func WriteMeta(c *gin.Context, status int, meta any) {
	jsonapi.WriteMeta(c.Writer, c.Request, status, meta)
}

// WriteError writes a service error and reports whether there was one. An
// *apperror.AppError is written as-is, apperror.ValidationErrors as a 422
// with one error object per field; anything else becomes a 500 whose log
// line names where (e.g. "Create Note Handler").
func WriteError(c *gin.Context, err error, where string) bool {
	if err == nil {
		return false
	}
	var appErr *apperror.AppError
	if errors.As(err, &appErr) {
		jsonapi.WriteErrorsContext(RequestContext(c), c.Writer, appErr)
		return true
	}
	var validationErrs apperror.ValidationErrors
	if errors.As(err, &validationErrs) {
		jsonapi.WriteValidationErrors(c.Writer, "", validationErrs)
		return true
	}
	jsonapi.WriteErrorsContext(RequestContext(c), c.Writer, apperror.New(apperror.CodeInternal,
		apperror.CodeInternal.SafeToThrow(), fmt.Errorf("(%s): %w", where, err)))
	return true
}

// RequestContext is the context to log an error with: the request's own, so
// a client cancellation is visible (a *gin.Context's Err() is always nil),
// or context.Background() when there's no request (gin.CreateTestContext).
func RequestContext(c *gin.Context) context.Context {
	if c.Request == nil {
		return context.Background()
	}
	return c.Request.Context()
}
