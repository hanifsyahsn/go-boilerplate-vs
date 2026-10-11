package jsonapi

import (
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/apperror"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/pagination"
)

// MaxFilterLength caps a filter[...] value.
const MaxFilterLength = 255

// QuerySpec declares which JSON:API query parameters one endpoint supports.
// Anything it leaves out is a 400 when sent, as the spec requires.
type QuerySpec struct {
	// Include is the ?include policy; nil means the endpoint doesn't support
	// include.
	Include *IncludeConfig
	// Fields maps each resource type the response can carry to a zero value
	// of its resource struct, which names the fields ?fields[TYPE] may list.
	// Empty means no sparse fieldsets.
	Fields map[string]any
	// Sort lists the sortable fields; nil means no ?sort.
	Sort *SortConfig
	// Page enables ?page[number] / ?page[size].
	Page bool
	// Filters lists the supported ?filter[NAME] names.
	Filters []string
}

// SortConfig is an endpoint's sort support: one field at a time, "-" for
// descending.
type SortConfig struct {
	Allowed []string // field names, without "-"
	Default string   // used when ?sort is absent, e.g. "-created_at"
}

// Query is a request's parsed JSON:API query parameters.
type Query struct {
	Include IncludePolicy
	Fields  Fieldsets
	// Sort is one field, "-" prefixed when descending ("" when the endpoint
	// doesn't sort).
	Sort string
	// Page is the requested page (zero values when not given: Resolve fills
	// the defaults).
	Page pagination.Request
	// Filter holds the given filter[NAME] values, trimmed.
	Filter map[string]string
}

// reservedFamilies are the spec's query parameter families. Any other
// all-lowercase a-z name is reserved by the spec for future use, so a server
// must reject it.
var reservedFamilies = map[string]bool{"include": true, "fields": true, "sort": true, "page": true, "filter": true}

// ParseQuery validates and parses values for an endpoint whose primary data
// is of primaryType. Errors are 400s whose source names the parameter.
// Implementation-specific parameters (names with a non a-z character) are
// ignored.
func ParseQuery(values url.Values, primaryType string, spec QuerySpec) (Query, *apperror.AppError) {
	q := Query{Filter: map[string]string{}}
	if spec.Sort != nil {
		q.Sort = spec.Sort.Default
	}

	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys) // deterministic: the first bad parameter is always the same one

	for _, key := range keys {
		family, arg, bracketed, ok := splitParam(key)
		if !ok {
			if isLowerAlpha(key) {
				return Query{}, badParam(key, fmt.Sprintf("%q is not a supported query parameter.", key))
			}
			continue // implementation-specific
		}
		if !isLowerAlpha(family) {
			continue // implementation-specific
		}
		if !reservedFamilies[family] {
			return Query{}, badParam(key, fmt.Sprintf("%q is not a supported query parameter.", key))
		}
		if len(values[key]) > 1 {
			return Query{}, badParam(key, fmt.Sprintf("%q is given more than once.", key))
		}
		value := values.Get(key)

		var err *apperror.AppError
		switch family {
		case "include":
			err = parseInclude(&q, key, bracketed, spec, primaryType, value)
		case "fields":
			err = parseFields(&q, key, arg, bracketed, spec, value)
		case "sort":
			err = parseSort(&q, key, bracketed, spec, value)
		case "page":
			err = parsePage(&q, key, arg, bracketed, spec, value)
		case "filter":
			err = parseFilter(&q, key, arg, bracketed, spec, value)
		}
		if err != nil {
			return Query{}, err
		}
	}

	if _, given := values["include"]; !given && spec.Include != nil {
		policy, err := ResolveIncludePolicy("", false, primaryType, *spec.Include)
		if err != nil {
			// defensive: a Default outside Allowed is a programming error
			return Query{}, err
		}
		q.Include = policy
	}

	return q, nil
}

func parseInclude(q *Query, key string, bracketed bool, spec QuerySpec, primaryType, value string) *apperror.AppError {
	if bracketed || spec.Include == nil {
		return unsupported(key)
	}
	policy, err := ResolveIncludePolicy(value, true, primaryType, *spec.Include)
	if err != nil {
		return err
	}
	q.Include = policy
	return nil
}

func parseFields(q *Query, key, resourceType string, bracketed bool, spec QuerySpec, value string) *apperror.AppError {
	if !bracketed || len(spec.Fields) == 0 {
		return unsupported(key)
	}
	resource, ok := spec.Fields[resourceType]
	if !ok {
		return badParam(key, fmt.Sprintf("This endpoint doesn't return %q resources.", resourceType))
	}
	known := FieldNames(resource)
	keep := map[string]bool{}
	for _, name := range strings.Split(value, ",") {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if !known[name] {
			return badParam(key, fmt.Sprintf("%q is not a field of %q.", name, resourceType))
		}
		keep[name] = true
	}
	if q.Fields == nil {
		q.Fields = Fieldsets{}
	}
	q.Fields[resourceType] = keep
	return nil
}

func parseSort(q *Query, key string, bracketed bool, spec QuerySpec, value string) *apperror.AppError {
	if bracketed || spec.Sort == nil {
		return unsupported(key)
	}
	value = strings.TrimSpace(value)
	if strings.Contains(value, ",") {
		return badParam(key, "Sort by one field at a time.")
	}
	field := strings.TrimPrefix(value, "-")
	for _, allowed := range spec.Sort.Allowed {
		if field == allowed {
			q.Sort = value
			return nil
		}
	}
	return badParam(key, fmt.Sprintf("Can't sort by %q. Allowed values: %s.", field, strings.Join(spec.Sort.Allowed, ", ")))
}

func parsePage(q *Query, key, arg string, bracketed bool, spec QuerySpec, value string) *apperror.AppError {
	if !bracketed || !spec.Page || (arg != "number" && arg != "size") {
		return unsupported(key)
	}
	n, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || n < 1 {
		return badParam(key, fmt.Sprintf("%s must be a positive whole number.", key))
	}
	if arg == "number" {
		q.Page.Page = n
	} else {
		q.Page.PageSize = n
	}
	return nil
}

func parseFilter(q *Query, key, name string, bracketed bool, spec QuerySpec, value string) *apperror.AppError {
	if !bracketed {
		return unsupported(key)
	}
	for _, allowed := range spec.Filters {
		if name == allowed {
			value = strings.TrimSpace(value)
			if len([]rune(value)) > MaxFilterLength {
				return badParam(key, fmt.Sprintf("%s must be at most %d characters long.", key, MaxFilterLength))
			}
			q.Filter[name] = value
			return nil
		}
	}
	return unsupported(key)
}

// Includes reports whether the relationship path ("created_by",
// "created_by.company") goes into "included", asked by the client or by the
// endpoint's default. A service uses it to load a costly relationship only
// when it's included.
func (q Query) Includes(path string) bool {
	for _, node := range q.Include { // one root: the primary type
		for _, segment := range strings.Split(path, ".") {
			next, ok := node[segment]
			if !ok {
				return false
			}
			node = next
		}
		return true
	}
	return false
}

// WantsField reports whether a resourceType response keeps the field (an
// attribute or relationship): true unless ?fields[resourceType] leaves it
// out. A relationship that isn't included but is wanted still needs its
// linkage (the related ids), never an empty to-many standing in for
// "not loaded".
func (q Query) WantsField(resourceType, name string) bool {
	keep, ok := q.Fields[resourceType]
	return !ok || keep[name]
}

// FilterBool reads filter[name] as a boolean: nil when not given.
func (q Query) FilterBool(name string) (*bool, *apperror.AppError) {
	raw, ok := q.Filter[name]
	if !ok || raw == "" {
		return nil, nil
	}
	b, err := strconv.ParseBool(raw)
	if err != nil {
		key := "filter[" + name + "]"
		return nil, badParam(key, fmt.Sprintf("%s must be true or false.", key))
	}
	return &b, nil
}

// splitParam splits "family[arg]" (bracketed) or "family". ok is false for a
// malformed name (unbalanced or nested brackets).
func splitParam(key string) (family, arg string, bracketed, ok bool) {
	open := strings.IndexByte(key, '[')
	if open < 0 {
		if strings.ContainsRune(key, ']') {
			return "", "", false, false
		}
		return key, "", false, true
	}
	if !strings.HasSuffix(key, "]") {
		return "", "", false, false
	}
	arg = key[open+1 : len(key)-1]
	if arg == "" || strings.ContainsAny(arg, "[]") {
		return "", "", false, false
	}
	return key[:open], arg, true, true
}

func isLowerAlpha(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < 'a' || r > 'z' {
			return false
		}
	}
	return true
}

func unsupported(key string) *apperror.AppError {
	return badParam(key, fmt.Sprintf("This endpoint doesn't support the %q query parameter.", key))
}

func badParam(key, message string) *apperror.AppError {
	return apperror.New(apperror.CodeBadRequest, message, fmt.Errorf("query parameter %q: %s", key, message)).
		WithParameter(key)
}
