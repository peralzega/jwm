package auth

// Role adalah peran global pengguna (SPEC bagian 2).
type Role string

// Peran global.
const (
	RoleAdmin          Role = "admin"
	RolePMO            Role = "pmo"
	RoleProjectManager Role = "project_manager"
	RoleMember         Role = "member"
	RoleViewer         Role = "viewer"
)

// Valid bernilai true bila r adalah peran yang dikenal.
func (r Role) Valid() bool {
	switch r {
	case RoleAdmin, RolePMO, RoleProjectManager, RoleMember, RoleViewer:
		return true
	}
	return false
}

// IsAdmin bernilai true untuk peran Admin.
func (r Role) IsAdmin() bool { return r == RoleAdmin }
