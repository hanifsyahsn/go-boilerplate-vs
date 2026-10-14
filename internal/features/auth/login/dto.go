package login

import (
	authshared "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/shared"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/jwttoken"
)

// Request is the attributes of the POST /api/auth/login document
// ({"data": {"type": "sessions", "attributes": {...}}}).
type Request struct {
	Email    string `json:"email" binding:"required,email,max=255" label:"Email"`
	Password string `json:"password" binding:"required,max=72" label:"Password"`
	// TrustDevice keeps the session sliding with no cap (a "remember this
	// device" box). Without it the session ends UNTRUSTED_SESSION_DURATION
	// after the login.
	TrustDevice bool `json:"trust_device" label:"Trust device"`
}

// Result is a successful login: the account and its fresh token pair.
type Result struct {
	User   authshared.UserResource
	Tokens jwttoken.Pair
}
