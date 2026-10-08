// Package enum holds cross-cutting string enums (auth roles, …). Feature-
// local enums live under their feature package instead.
package enum

// Role is a user's system role. It rides in the access token's `role` claim
// and gates the admin routes via middleware.RequireRole.
type Role string

const (
	RoleUser  Role = "user"
	RoleAdmin Role = "admin"
)

func (r Role) String() string { return string(r) }

// Valid reports whether r is one of the known roles.
func (r Role) Valid() bool {
	switch r {
	case RoleUser, RoleAdmin:
		return true
	}
	return false
}

// Roles returns every known role, in a stable order.
func Roles() []Role { return []Role{RoleUser, RoleAdmin} }
