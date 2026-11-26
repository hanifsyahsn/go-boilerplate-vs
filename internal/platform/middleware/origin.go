package middleware

import (
	"fmt"
	"net/http"
	"strings"

	platformconstant "github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/constant"
)

// normalizeOrigin is the form origins are compared in, by CORS and the
// origin guard: trimmed, lowercase, no trailing /.
func normalizeOrigin(s string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(s)), "/")
}

// newOriginSet is ALLOWED_ORIGINS as a set of normalised origins, and whether
// CORS and the origin guard are on. An empty list is off (allowed in
// development only, decision 0021). platformconstant.NoBrowserOrigins
// ("none") is on with nothing in the set: no browser origin is allowed, and a
// request sending "Origin: none" matches nothing either.
func newOriginSet(origins []string) (set map[string]struct{}, on bool) {
	set = make(map[string]struct{}, len(origins))
	for _, o := range origins {
		if strings.EqualFold(strings.TrimSpace(o), platformconstant.NoBrowserOrigins) {
			continue
		}
		set[normalizeOrigin(o)] = struct{}{}
	}
	return set, len(origins) > 0
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
