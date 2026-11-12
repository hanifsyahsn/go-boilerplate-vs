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
// A key present with no value at all (only another layer can leave one; the
// HTTP parser can't) counts as one empty value, never as absent.
func singleHeader(h http.Header, name string) (value string, count int) {
	values, present := h[http.CanonicalHeaderKey(name)]
	if !present {
		return "", 0
	}
	if len(values) == 0 {
		return "", 1
	}
	return values[0], len(values)
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
