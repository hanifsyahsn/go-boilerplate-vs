package constant

import "strings"

// NoBrowserOrigins is the ALLOWED_ORIGINS value of a service with no browser
// front end (decision 0021): CORS and the origin guard stay on with no origin
// allowed — a write carrying an Origin header is a 403, one without (a server,
// curl) passes. Config reads it case-insensitively, alone; the middleware
// never treats it as an origin.
const NoBrowserOrigins = "none"

// IsNoBrowserOrigins reports whether an ALLOWED_ORIGINS entry is
// NoBrowserOrigins, as config and the middleware read it: trimmed, any case.
func IsNoBrowserOrigins(entry string) bool {
	return strings.EqualFold(strings.TrimSpace(entry), NoBrowserOrigins)
}
