package middleware

import (
	"fmt"
	"net/http"
	"strings"
)

// normalizeOrigin is the form origins are compared in, by CORS and the
// origin guard: trimmed, lowercase, no trailing /.
func normalizeOrigin(s string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(s)), "/")
}

// newOriginSet is ALLOWED_ORIGINS as a set of normalised origins.
func newOriginSet(origins []string) map[string]struct{} {
	set := make(map[string]struct{}, len(origins))
	for _, o := range origins {
		set[normalizeOrigin(o)] = struct{}{}
	}
	return set
}

// singleHeader reads a header that is accepted only when sent exactly once:
// its first value and how many values came. OriginGuard, CORS and APIKey read
// theirs through it, so absent / empty / repeated mean the same everywhere.
func singleHeader(h http.Header, name string) (value string, count int) {
	values := h.Values(name)
	if len(values) > 0 {
		value = values[0]
	}
	return value, len(values)
}

// describeOrigin names a refused Origin for the log: the value, or how many
// headers came (quoting only the first, maybe a listed one, would read as a
// configuration bug).
func describeOrigin(value string, count int) string {
	if count > 1 {
		return fmt.Sprintf("%d Origin headers", count)
	}
	return fmt.Sprintf("origin %q", value)
}
