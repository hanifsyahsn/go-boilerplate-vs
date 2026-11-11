package middleware

import "strings"

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
