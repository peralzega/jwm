package board

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func f(v float64) *float64 { return &v }

func TestBetween(t *testing.T) {
	tests := []struct {
		name   string
		prev   *float64
		next   *float64
		want   float64
		wantOK bool
	}{
		{"grup kosong", nil, nil, 1024, true},
		{"di awal", nil, f(1024), 0, true},
		{"di akhir", f(2048), nil, 3072, true},
		{"di tengah", f(1024), f(2048), 1536, true},
		{"negatif", f(-1024), f(0), -512, true},
		{"celah sempit", f(1), f(1 + 1e-7), 0, false},
	}
	for _, tt := range tests {
		got, ok := between(tt.prev, tt.next)
		assert.Equal(t, tt.wantOK, ok, tt.name)
		if ok {
			assert.InDelta(t, tt.want, got, 1e-9, tt.name)
		}
	}

	// Penyisipan berulang di titik yang sama akhirnya meminta penataan ulang.
	prev, next := 1024.0, 2048.0
	n := 0
	for {
		mid, ok := between(&prev, &next)
		if !ok {
			break
		}
		next = mid
		n++
	}
	assert.Greater(t, n, 20)
}

func TestItemPermissions(t *testing.T) {
	me, other := uuid.New(), uuid.New()
	tests := []struct {
		name       string
		eff        ProjectRole
		createdBy  uuid.UUID
		assigned   bool
		wantEdit   bool
		wantDelete bool
	}{
		{"manager item orang lain", RoleManager, other, false, true, true},
		{"member item sendiri", RoleMember, me, false, true, true},
		{"member ditugaskan", RoleMember, other, true, true, false},
		{"member item orang lain", RoleMember, other, false, false, false},
		{"viewer item sendiri", RoleViewer, me, true, false, false},
		{"tanpa akses", RoleNone, me, true, false, false},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.wantEdit, canEditItem(tt.eff, tt.createdBy, me, tt.assigned), tt.name)
		assert.Equal(t, tt.wantDelete, canDeleteItem(tt.eff, tt.createdBy, me), tt.name)
	}
}
