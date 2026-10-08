package refresh

import "github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/jwttoken"

// Result is a fresh token pair: the refresh token rotates on every refresh.
// (The optional request body is authshared.RefreshTokenRequest.)
type Result struct {
	Tokens jwttoken.Pair
}
