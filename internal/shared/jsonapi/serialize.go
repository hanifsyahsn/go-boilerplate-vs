package jsonapi

import (
	"fmt"
	"reflect"
	"strings"
)

// IncludePolicy is the tree of relationship paths to expand into
// "included", keyed at the root by the primary resource type:
// {"notes": {"created_by": {}}}.
type IncludePolicy map[string]IncludePolicy

// Fieldsets is a sparse-fieldset request (?fields[TYPE]=a,b): per resource
// type, the attribute / relationship names to keep. A type that isn't
// present keeps every field.
type Fieldsets map[string]map[string]bool

// maxDepth bounds how deep relationships are followed.
const maxDepth = 5

type serializeContext struct {
	policy   IncludePolicy
	fields   Fieldsets
	included map[string]Resource
	order    []string // included keys, in first-seen order (stable output)
	visited  map[uintptr]bool
	depth    int
}

// Build serializes v (a resource struct, a pointer to one, or a slice of
// them) as the primary data of a document. Struct tags drive it:
//
//	`jsonapi:"id"`                         the resource id (any type, rendered as a string)
//	`json:"name"`                          an attribute
//	`jsonapi:"relation,<name>[,<type>]"`   a relationship: <name> is its key and its include
//	                                       path segment, <type> the related resource type
//	                                       (defaults to <name>); a nil pointer is a null to-one,
//	                                       a slice a to-many
//
// include selects what goes into "included" (a resource also in the primary
// data is never repeated there); fields trims resources per type.
func Build(resourceType string, v any, include IncludePolicy, fields Fieldsets) Document {
	ctx := &serializeContext{
		policy:   include[resourceType],
		fields:   fields,
		included: map[string]Resource{},
		visited:  map[uintptr]bool{},
	}

	primary := map[string]bool{}
	var data any
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Slice {
		resources := make([]Resource, 0, rv.Len())
		for i := 0; i < rv.Len(); i++ {
			res := serialize(resourceType, rv.Index(i).Interface(), ctx)
			primary[key(res)] = true
			resources = append(resources, res)
		}
		data = resources
	} else {
		res := serialize(resourceType, v, ctx)
		primary[key(res)] = true
		data = res
	}

	doc := Document{JSONAPI: versionObject, Data: data}
	for _, k := range ctx.order {
		if !primary[k] {
			doc.Included = append(doc.Included, ctx.included[k])
		}
	}
	return doc
}

func key(r Resource) string { return r.Type + ":" + r.ID }

func serialize(resourceType string, v any, ctx *serializeContext) Resource {
	rv := reflect.ValueOf(v)
	if !rv.IsValid() {
		return Resource{Type: resourceType}
	}
	if rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return Resource{Type: resourceType}
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return Resource{Type: resourceType}
	}

	// cycle protection
	if rv.CanAddr() {
		ptr := rv.UnsafeAddr()
		if ctx.visited[ptr] {
			return Resource{Type: resourceType}
		}
		ctx.visited[ptr] = true
	}

	rt := rv.Type()
	res := Resource{Type: resourceType, Attributes: map[string]any{}, Relationships: map[string]Relationship{}}

	for i := 0; i < rv.NumField(); i++ {
		field := rt.Field(i)
		value := rv.Field(i)
		jsonapiTag := field.Tag.Get("jsonapi")

		if jsonapiTag == "id" {
			res.ID = fmt.Sprintf("%v", value.Interface())
			continue
		}

		if relName, relType, ok := parseRelationTag(jsonapiTag); ok {
			if ctx.depth+1 >= maxDepth {
				continue
			}
			childPolicy, expand := ctx.policy[relName]
			child := &serializeContext{
				policy:   childPolicy,
				fields:   ctx.fields,
				included: ctx.included,
				order:    nil,
				visited:  ctx.visited,
				depth:    ctx.depth + 1,
			}
			res.Relationships[relName] = Relationship{Data: relationData(value, relType, child, expand)}
			ctx.order = append(ctx.order, child.order...)
			continue
		}

		name, omitempty := attributeName(field)
		if name == "" {
			continue
		}
		if omitempty && value.Kind() == reflect.Pointer && value.IsNil() {
			continue
		}
		res.Attributes[name] = value.Interface()
	}

	if keep, ok := ctx.fields[resourceType]; ok {
		for name := range res.Attributes {
			if !keep[name] {
				delete(res.Attributes, name)
			}
		}
		for name := range res.Relationships {
			if !keep[name] {
				delete(res.Relationships, name)
			}
		}
	}

	return res
}

// parseRelationTag reads `jsonapi:"relation,<name>[,<type>]"`.
func parseRelationTag(tag string) (name, relType string, ok bool) {
	parts := strings.Split(tag, ",")
	if parts[0] != "relation" || len(parts) < 2 || parts[1] == "" {
		return "", "", false
	}
	name, relType = parts[1], parts[1]
	if len(parts) >= 3 && parts[2] != "" {
		relType = parts[2]
	}
	return name, relType, true
}

// attributeName is the field's attribute key from its json tag ("" when the
// field isn't an attribute).
func attributeName(field reflect.StructField) (name string, omitempty bool) {
	parts := strings.Split(field.Tag.Get("json"), ",")
	if parts[0] == "" || parts[0] == "-" {
		return "", false
	}
	for _, opt := range parts[1:] {
		if opt == "omitempty" {
			omitempty = true
		}
	}
	return parts[0], omitempty
}

func relationData(value reflect.Value, relType string, ctx *serializeContext, expand bool) any {
	if value.Kind() == reflect.Slice {
		// non-nil, so an empty to-many renders as [] not null
		ids := []ResourceIdentifier{}
		for i := 0; i < value.Len(); i++ {
			child := serialize(relType, value.Index(i).Interface(), ctx)
			if child.ID == "" {
				continue
			}
			ids = append(ids, ResourceIdentifier{Type: child.Type, ID: child.ID})
			collect(child, ctx, expand)
		}
		return ids
	}

	child := serialize(relType, value.Interface(), ctx)
	if child.ID == "" {
		return nil // an empty to-one: "data": null
	}
	collect(child, ctx, expand)
	return ResourceIdentifier{Type: child.Type, ID: child.ID}
}

func collect(res Resource, ctx *serializeContext, expand bool) {
	if !expand {
		return
	}
	k := key(res)
	if _, seen := ctx.included[k]; seen {
		return
	}
	ctx.included[k] = res
	ctx.order = append(ctx.order, k)
}

// FieldNames lists the fields of a resource struct a sparse fieldset may
// name: its attributes and relationships (not the id).
func FieldNames(resource any) map[string]bool {
	names := map[string]bool{}
	rt := reflect.TypeOf(resource)
	if rt == nil {
		return names
	}
	if rt.Kind() == reflect.Pointer {
		rt = rt.Elem()
	}
	if rt.Kind() != reflect.Struct {
		return names
	}
	for i := 0; i < rt.NumField(); i++ {
		field := rt.Field(i)
		tag := field.Tag.Get("jsonapi")
		if tag == "id" {
			continue
		}
		if name, _, ok := parseRelationTag(tag); ok {
			names[name] = true
			continue
		}
		if name, _ := attributeName(field); name != "" {
			names[name] = true
		}
	}
	return names
}
