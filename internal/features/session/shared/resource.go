// Package shared holds what the session slices (list, revoke) share: the
// "sessions" resource.
package shared

import "time"

// SessionsRT is the JSON:API type of a signed-in device (it is also the type
// of the login / refresh / logout request documents).
const SessionsRT = "sessions"

// NotFoundMessage is the 404 of a session that isn't the caller's or isn't
// live: someone else's reads as missing.
const NotFoundMessage = "That session doesn't exist."

// SessionResource is one signed-in device of the caller. id is the session
// (refresh-token family) id.
type SessionResource struct {
	ID         string    `json:"id" jsonapi:"id"`
	UserAgent  string    `json:"user_agent"`
	ClientIP   string    `json:"client_ip"`
	Trusted    bool      `json:"trusted"`
	CreatedAt  time.Time `json:"created_at"`
	LastUsedAt time.Time `json:"last_used_at"`
	// ExpiresAt is when an untrusted device's session ends; null for a
	// trusted one (no cap).
	ExpiresAt *time.Time `json:"expires_at"`
	// Current marks the session of the request's own access token.
	Current bool `json:"current"`
}

// Fieldsets is the sparse-fieldset map of the sessions endpoints.
var Fieldsets = map[string]any{SessionsRT: SessionResource{}}
