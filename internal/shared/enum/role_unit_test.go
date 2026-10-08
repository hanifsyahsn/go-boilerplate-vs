package enum

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRole_Valid_U(t *testing.T) {
	testCases := []struct {
		name string
		role Role
		want bool
	}{
		{name: "user", role: RoleUser, want: true},
		{name: "admin", role: RoleAdmin, want: true},
		{name: "empty", role: Role(""), want: false},
		{name: "unknown", role: Role("superadmin"), want: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, tc.role.Valid())
		})
	}
}

func TestRoles_U(t *testing.T) {
	require.Equal(t, []Role{RoleUser, RoleAdmin}, Roles())
}
