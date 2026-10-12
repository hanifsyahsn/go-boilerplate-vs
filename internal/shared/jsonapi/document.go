// Package jsonapi implements the JSON:API 1.1 response side (documents,
// resource serialization, compound documents, sparse fieldsets, links,
// error objects) and the query side (include / fields / sort / page /
// filter). The request-body side lives in handlerutil.BindResource, the
// media-type negotiation in middleware.JSONAPIMediaType.
package jsonapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/apperror"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/pagination"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/requestid"
)

// MediaType is the JSON:API media type, sent on every response and required
// on every request body.
const MediaType = "application/vnd.api+json"

// Version is the spec version this server implements, sent as the top-level
// "jsonapi" member.
const Version = "1.1"

// Object is the top-level "jsonapi" member.
type Object struct {
	Version string `json:"version"`
}

var versionObject = &Object{Version: Version}

// ResourceIdentifier is a relationship's linkage: {type, id}.
type ResourceIdentifier struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

// Relationship is a resource's relationship object. Data is a
// ResourceIdentifier, a []ResourceIdentifier, or nil (an empty to-one,
// rendered as null).
type Relationship struct {
	Data any `json:"data"`
}

// Resource is a resource object.
type Resource struct {
	Type          string                  `json:"type"`
	ID            string                  `json:"id"`
	Attributes    map[string]any          `json:"attributes,omitempty"`
	Relationships map[string]Relationship `json:"relationships,omitempty"`
}

// Links is a links object. A value is a URL string, or nil for a link that
// is unavailable (rendered as null, e.g. "prev" on the first page).
type Links map[string]any

// Document is a top-level data / meta document.
type Document struct {
	JSONAPI  *Object    `json:"jsonapi,omitempty"`
	Data     any        `json:"data,omitempty"`
	Included []Resource `json:"included,omitempty"`
	Meta     any        `json:"meta,omitempty"`
	Links    Links      `json:"links,omitempty"`
}

// ErrorObject is one error object.
type ErrorObject struct {
	Status string           `json:"status"`
	Code   string           `json:"code,omitempty"`
	Title  string           `json:"title"`
	Detail string           `json:"detail,omitempty"`
	Source *apperror.Source `json:"source,omitempty"`
}

// ErrorDocument is a top-level error document.
type ErrorDocument struct {
	JSONAPI *Object       `json:"jsonapi,omitempty"`
	Errors  []ErrorObject `json:"errors"`
}

// WriteJSON writes payload with the JSON:API media type.
func WriteJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", MediaType)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

// WriteData writes a document whose primary data is data (a resource struct,
// or a slice of them for a collection), shaped by the request's query:
// q.Include expands relationships into "included", q.Fields trims resources
// to the requested fields. links.self is the request URL; a pagination.Meta
// meta also adds first / last / prev / next.
func WriteData(w http.ResponseWriter, r *http.Request, status int, resourceType string, data any, q Query, meta any) {
	doc := Build(resourceType, data, q.Include, q.Fields)
	doc.Meta = meta
	doc.Links = Links{"self": r.URL.RequestURI()}
	if pm, ok := meta.(pagination.Meta); ok {
		for k, v := range PaginationLinks(r.URL, pm) {
			doc.Links[k] = v
		}
	}
	WriteJSON(w, status, doc)
}

// WriteMeta writes a meta-only document (no primary data), e.g. a token
// refresh.
func WriteMeta(w http.ResponseWriter, r *http.Request, status int, meta any) {
	WriteJSON(w, status, Document{
		JSONAPI: versionObject,
		Meta:    meta,
		Links:   Links{"self": r.URL.RequestURI()},
	})
}

// WriteErrors writes one error object per error. The status is the errors'
// own when they share one, else the most general: 400 for all-4xx, 500
// otherwise. It's WriteErrorsContext without a request context — prefer that
// one when there is one.
func WriteErrors(w http.ResponseWriter, errs ...*apperror.AppError) {
	WriteErrorsContext(context.Background(), w, errs...)
}

// WriteErrorsContext writes errs like WriteErrors and logs each one's wrapped
// cause with ctx:
//
//   - a 5xx: "request failed" at error level — or "request cancelled" at warn
//     when the client went away (ctx cancelled, or context.Canceled in the
//     error chain). The context is the main signal: lib/pq answers a
//     cancelled query with a Postgres error, not context.Canceled. A
//     context.DeadlineExceeded stays an error: that's our timeout.
//   - a 4xx: "request rejected" at warn.
//
// Pass the request's own context (c.Request.Context()), not a *gin.Context:
// its Err() is always nil. The request id comes from ctx; when ctx carries
// none it's read back from the response header (middleware.RequestID sets it
// before the handlers run), so it's logged once either way.
func WriteErrorsContext(ctx context.Context, w http.ResponseWriter, errs ...*apperror.AppError) {
	reqID := ""
	if requestid.FromContext(ctx) == "" {
		reqID = w.Header().Get(requestid.Header)
	}
	ctxCancelled := errors.Is(ctx.Err(), context.Canceled)

	objects := make([]ErrorObject, 0, len(errs))
	status := 0
	for _, e := range errs {
		s := apperror.MapToHTTPStatus(e.Code)
		switch {
		case status == 0:
			status = s
		case status != s && (status >= 500 || s >= 500):
			status = http.StatusInternalServerError
		case status != s:
			status = http.StatusBadRequest
		}

		if e.Err != nil {
			args := []any{"status", s, "code", string(e.Code), "err", e.Err}
			if reqID != "" {
				args = append(args, requestid.Key, reqID)
			}
			switch {
			case s < 500:
				slog.WarnContext(ctx, "request rejected", args...)
			case ctxCancelled || errors.Is(e, context.Canceled):
				slog.WarnContext(ctx, "request cancelled", args...)
			default:
				slog.ErrorContext(ctx, "request failed", args...)
			}
		}

		objects = append(objects, newErrorObject(e, s))
	}
	if status == 0 {
		// defensive: callers always pass at least one error
		status = http.StatusInternalServerError
	}

	WriteJSON(w, status, ErrorDocument{JSONAPI: versionObject, Errors: objects})
}

func newErrorObject(e *apperror.AppError, status int) ErrorObject {
	return ErrorObject{
		Status: strconv.Itoa(status),
		Code:   string(e.Code),
		Title:  e.Code.Title(),
		Detail: e.Message,
		Source: e.Source,
	}
}

// WriteError writes a single error built from its parts.
func WriteError(w http.ResponseWriter, code apperror.Code, message string, err error) {
	WriteErrors(w, apperror.New(code, message, err))
}

// WriteAppError writes a single *apperror.AppError.
func WriteAppError(w http.ResponseWriter, err *apperror.AppError) {
	WriteErrors(w, err)
}

// WriteValidationErrors writes a 422 with one error object per failing
// field; each ValidationError.Pointer is prefixed with pointerPrefix (e.g.
// "/data/attributes") to point into the request document.
func WriteValidationErrors(w http.ResponseWriter, pointerPrefix string, errs apperror.ValidationErrors) {
	appErrs := make([]*apperror.AppError, 0, len(errs))
	for _, e := range errs {
		appErrs = append(appErrs,
			apperror.New(apperror.CodeUnprocessableEntity, e.Detail, nil).WithPointer(pointerPrefix+e.Pointer))
	}
	WriteErrors(w, appErrs...)
}
