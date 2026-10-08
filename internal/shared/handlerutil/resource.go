package handlerutil

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/apperror"
)

// attributesPointer prefixes every attribute's JSON Pointer.
const attributesPointer = "/data/attributes"

// ResourceSpec is what a request document must carry.
type ResourceSpec struct {
	// Type is the resource type the endpoint takes ("notes").
	Type string
	// ID is "" on a create (the server assigns ids; a client id is a 403)
	// and the URL's id on an update (required, must match: else 409).
	ID string
	// Optional lets a request come without any body (e.g. refresh, where
	// the token may come from a cookie instead).
	Optional bool
}

type resourceObject struct {
	Type          *string         `json:"type"`
	ID            *string         `json:"id"`
	LID           *string         `json:"lid"`
	Attributes    json.RawMessage `json:"attributes"`
	Relationships json.RawMessage `json:"relationships"`
}

// DecodeResource decodes a JSON:API request document into attrs and
// validates it. It returns an *apperror.AppError (with source.pointer) or
// apperror.ValidationErrors (one per failing field, pointers into
// /data/attributes):
//
//	not JSON / no data / data not one object  400
//	no type                                   400
//	another type                              409
//	an id on a create (client-generated)      403
//	no id on an update                        400
//	an id that isn't the URL's                409
//	any relationships                         403 (none are writable here)
//	an attribute attrs doesn't declare        403 (not writable)
//	an attribute of the wrong JSON type       422
//	a failed binding rule                     422 per field
func DecodeResource(body io.Reader, spec ResourceSpec, attrs any) error {
	if body == nil {
		return badBody("/data", "The request body must be a JSON:API document with a data member.", nil)
	}

	var doc struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(body).Decode(&doc); err != nil {
		return badBody("", "The request body is not valid JSON.", err)
	}
	if isAbsent(doc.Data) {
		return badBody("/data", "The request body must be a JSON:API document with a data member.", nil)
	}

	var res resourceObject
	if err := json.Unmarshal(doc.Data, &res); err != nil {
		return badBody("/data", "data must be a single resource object.", err)
	}

	if res.Type == nil || *res.Type == "" {
		return badBody("/data/type", "type is required.", nil)
	}
	if *res.Type != spec.Type {
		return apperror.New(apperror.CodeConflict, fmt.Sprintf("This endpoint takes %q resources.", spec.Type),
			fmt.Errorf("request type %q, want %q", *res.Type, spec.Type)).WithPointer("/data/type")
	}

	if spec.ID == "" {
		if res.ID != nil {
			return apperror.New(apperror.CodeForbidden, "Client-generated ids aren't supported.",
				fmt.Errorf("client sent id %q on a create", *res.ID)).WithPointer("/data/id")
		}
	} else {
		if res.ID == nil || *res.ID == "" {
			return badBody("/data/id", "id is required.", nil)
		}
		if *res.ID != spec.ID {
			return apperror.New(apperror.CodeConflict, "The id in the body doesn't match the URL.",
				fmt.Errorf("body id %q, URL id %q", *res.ID, spec.ID)).WithPointer("/data/id")
		}
	}

	if !isAbsent(res.Relationships) {
		return apperror.New(apperror.CodeForbidden, "Relationships can't be set on this endpoint.",
			errors.New("client sent relationships")).WithPointer("/data/relationships")
	}

	if !isAbsent(res.Attributes) {
		if err := decodeAttributes(res.Attributes, attrs); err != nil {
			return err
		}
	}

	if err := binding.Validator.ValidateStruct(attrs); err != nil {
		var ve validator.ValidationErrors
		if errors.As(err, &ve) {
			errs := apperror.ValidationErrors(apperror.ValidatorError(ve, attrs))
			for i := range errs {
				errs[i].Pointer = attributesPointer + errs[i].Pointer
			}
			return errs
		}
		// defensive: a struct target only yields ValidationErrors
		return badBody(attributesPointer, "The attributes are not valid.", err)
	}

	return nil
}

func decodeAttributes(raw json.RawMessage, attrs any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	err := dec.Decode(attrs)
	if err == nil {
		return nil
	}

	if name, ok := strings.CutPrefix(err.Error(), "json: unknown field "); ok {
		if unquoted, uqErr := strconv.Unquote(name); uqErr == nil {
			name = unquoted
		}
		return apperror.New(apperror.CodeForbidden, fmt.Sprintf("%q can't be set on this endpoint.", name), err).
			WithPointer(attributesPointer + "/" + name)
	}

	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &typeErr) && typeErr.Field != "" {
		field := strings.ReplaceAll(typeErr.Field, ".", "/")
		return apperror.New(apperror.CodeUnprocessableEntity, fmt.Sprintf("%s has the wrong type.", typeErr.Field), err).
			WithPointer(attributesPointer + "/" + field)
	}

	return badBody(attributesPointer, "attributes must be an object.", err)
}

func isAbsent(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null"))
}

func badBody(pointer, message string, err error) *apperror.AppError {
	if err == nil {
		err = errors.New(message)
	}
	e := apperror.New(apperror.CodeBadRequest, message, fmt.Errorf("decode request document: %w", err))
	if pointer != "" {
		e = e.WithPointer(pointer)
	}
	return e
}
