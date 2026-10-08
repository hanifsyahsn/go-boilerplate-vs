package middleware

import (
	"errors"
	"mime"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/apperror"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/jsonapi"
)

// JSONAPIMediaType enforces JSON:API content negotiation on every API
// request (no extensions are supported; profiles are accepted and ignored):
//
//   - a body must be sent as application/vnd.api+json, with no media type
//     parameter but profile, else 415;
//   - an Accept header that lists the JSON:API media type only with
//     parameters other than profile (or q) can't be satisfied: 406. One that
//     doesn't list it at all (e.g. */*) is fine.
//
// Every response varies on Accept.
func JSONAPIMediaType() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Vary", "Accept")

		if !acceptsJSONAPI(c.GetHeader("Accept")) {
			jsonapi.WriteAppError(c.Writer, apperror.New(apperror.CodeNotAcceptable,
				"Accept must allow "+jsonapi.MediaType+" without media type parameters other than profile.",
				errors.New("JSONAPIMediaType: no acceptable JSON:API media type in Accept")).WithHeader("Accept"))
			c.Abort()
			return
		}

		if hasBody(c.Request) && !isJSONAPIContentType(c.GetHeader("Content-Type")) {
			jsonapi.WriteAppError(c.Writer, apperror.New(apperror.CodeUnsupportedMedia,
				"Send the request body as "+jsonapi.MediaType+".",
				errors.New("JSONAPIMediaType: request body with a non JSON:API Content-Type")).WithHeader("Content-Type"))
			c.Abort()
			return
		}

		c.Next()
	}
}

func hasBody(r *http.Request) bool {
	return r.Body != nil && r.Body != http.NoBody && r.ContentLength != 0
}

func isJSONAPIContentType(header string) bool {
	mediaType, params, err := mime.ParseMediaType(header)
	if err != nil || mediaType != jsonapi.MediaType {
		return false
	}
	for name := range params {
		if name != "profile" {
			return false // ext (unsupported) or any other parameter
		}
	}
	return true
}

// acceptsJSONAPI reports whether the Accept header can be served: true when
// it doesn't name the JSON:API media type at all, or names it at least once
// with no parameter but profile / q.
func acceptsJSONAPI(header string) bool {
	if strings.TrimSpace(header) == "" {
		return true
	}
	named := false
	for _, part := range strings.Split(header, ",") {
		mediaType, params, err := mime.ParseMediaType(strings.TrimSpace(part))
		if err != nil || mediaType != jsonapi.MediaType {
			continue
		}
		named = true
		usable := true
		for name := range params {
			if name != "profile" && name != "q" {
				usable = false
			}
		}
		if usable {
			return true
		}
	}
	return !named
}
