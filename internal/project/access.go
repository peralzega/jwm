// Package project menangani proyek, keanggotaan, dan hak akses per proyek.
package project

import "jwm/internal/auth"

// Role adalah peran di dalam satu proyek. Urutannya bermakna: nilai lebih
// besar berarti hak lebih luas.
type Role int

// Peran proyek (SPEC bagian 2).
const (
	RoleNone Role = iota
	RoleViewer
	RoleMember
	RoleManager
)

// ParseRole mengubah nilai kolom peran_proyek menjadi Role.
func ParseRole(s string) (Role, bool) {
	switch s {
	case "viewer":
		return RoleViewer, true
	case "member":
		return RoleMember, true
	case "manager":
		return RoleManager, true
	}
	return RoleNone, false
}

func (r Role) String() string {
	switch r {
	case RoleViewer:
		return "viewer"
	case RoleMember:
		return "member"
	case RoleManager:
		return "manager"
	}
	return ""
}

// MarshalText membuat Role tampil sebagai teks di JSON (kosong untuk RoleNone).
func (r Role) MarshalText() ([]byte, error) { return []byte(r.String()), nil }

// UnmarshalText menerima "viewer", "member", atau "manager".
func (r *Role) UnmarshalText(b []byte) error {
	v, ok := ParseRole(string(b))
	if !ok {
		return ErrInvalidProjectRole
	}
	*r = v
	return nil
}

// Cap adalah peran proyek tertinggi yang boleh dimiliki pengguna dengan peran global g.
// Viewer global hanya boleh viewer proyek; Member global paling tinggi member;
// Project Manager, PMO, dan Admin boleh menjadi manager.
func Cap(g auth.Role) Role {
	switch g {
	case auth.RoleAdmin, auth.RolePMO, auth.RoleProjectManager:
		return RoleManager
	case auth.RoleMember:
		return RoleMember
	case auth.RoleViewer:
		return RoleViewer
	}
	return RoleNone
}

// Effective menghitung hak efektif pengguna pada sebuah proyek.
// Admin dan PMO mengelola semua proyek tanpa perlu menjadi anggota.
// Peran lain: irisan peran global dan peran proyek (yang paling terbatas berlaku).
// member bernilai RoleNone bila pengguna bukan anggota proyek.
func Effective(g auth.Role, aktif bool, member Role) Role {
	if !aktif {
		return RoleNone
	}
	if g == auth.RoleAdmin || g == auth.RolePMO {
		return RoleManager
	}
	return min(Cap(g), member)
}

// CanCreateProject: hanya Admin, PMO, dan Project Manager yang dapat membuat proyek.
func CanCreateProject(g auth.Role) bool { return Cap(g) == RoleManager }
