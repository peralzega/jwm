package project

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"jwm/internal/auth"
)

func TestEffective(t *testing.T) {
	tests := []struct {
		global auth.Role
		aktif  bool
		member Role
		want   Role
	}{
		// Admin dan PMO: manager di semua proyek, tanpa keanggotaan.
		{auth.RoleAdmin, true, RoleNone, RoleManager},
		{auth.RolePMO, true, RoleNone, RoleManager},
		{auth.RolePMO, true, RoleViewer, RoleManager},
		// Project Manager: mengikuti peran proyek.
		{auth.RoleProjectManager, true, RoleNone, RoleNone},
		{auth.RoleProjectManager, true, RoleManager, RoleManager},
		{auth.RoleProjectManager, true, RoleViewer, RoleViewer},
		// Member global dibatasi member walau tercatat manager.
		{auth.RoleMember, true, RoleManager, RoleMember},
		{auth.RoleMember, true, RoleMember, RoleMember},
		{auth.RoleMember, true, RoleViewer, RoleViewer},
		{auth.RoleMember, true, RoleNone, RoleNone},
		// Viewer global selalu paling tinggi viewer.
		{auth.RoleViewer, true, RoleManager, RoleViewer},
		{auth.RoleViewer, true, RoleMember, RoleViewer},
		{auth.RoleViewer, true, RoleNone, RoleNone},
		// Akun nonaktif tidak punya hak apa pun.
		{auth.RoleAdmin, false, RoleManager, RoleNone},
		{auth.RoleProjectManager, false, RoleManager, RoleNone},
		// Peran global tak dikenal tidak punya hak.
		{auth.Role("lain"), true, RoleManager, RoleNone},
	}
	for _, tt := range tests {
		got := Effective(tt.global, tt.aktif, tt.member)
		assert.Equal(t, tt.want, got, "global=%s aktif=%v proyek=%s", tt.global, tt.aktif, tt.member)
	}
}

func TestCapAndCreate(t *testing.T) {
	tests := []struct {
		global    auth.Role
		cap       Role
		canCreate bool
	}{
		{auth.RoleAdmin, RoleManager, true},
		{auth.RolePMO, RoleManager, true},
		{auth.RoleProjectManager, RoleManager, true},
		{auth.RoleMember, RoleMember, false},
		{auth.RoleViewer, RoleViewer, false},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.cap, Cap(tt.global), tt.global)
		assert.Equal(t, tt.canCreate, CanCreateProject(tt.global), tt.global)
	}
}

func TestRoleText(t *testing.T) {
	for _, s := range []string{"viewer", "member", "manager"} {
		var r Role
		assert.NoError(t, r.UnmarshalText([]byte(s)))
		b, _ := r.MarshalText()
		assert.Equal(t, s, string(b))
	}
	var r Role
	assert.ErrorIs(t, r.UnmarshalText([]byte("owner")), ErrInvalidProjectRole)
	assert.True(t, RoleViewer < RoleMember && RoleMember < RoleManager)
}
