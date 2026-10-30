package auth

import (
	"context"
	"errors"
	"fmt"

	"jwm/internal/db/gen"
)

// Density adalah kepadatan tampilan pilihan pengguna (SPEC M2).
type Density string

// Nilai kepadatan yang sah. Harus sama dengan CHECK pada kolom users.kepadatan.
const (
	DensityCompact Density = "compact"
	DensityNormal  Density = "normal"
)

// Valid melaporkan apakah d nilai kepadatan yang dikenal.
func (d Density) Valid() bool { return d == DensityCompact || d == DensityNormal }

// ErrInvalidDensity dikembalikan untuk nilai kepadatan yang tidak dikenal.
var ErrInvalidDensity = errors.New("kepadatan harus compact atau normal")

// Preferences adalah preferensi tampilan yang boleh diubah pengguna sendiri.
type Preferences struct {
	Kepadatan *Density
}

// UpdatePreferences mengubah preferensi milik p sendiri. Tidak menyentuh
// peran, status akun, atau sesi, sehingga tidak dicatat di audit_log.
func (s *Service) UpdatePreferences(ctx context.Context, p Principal, in Preferences) (User, error) {
	if in.Kepadatan == nil {
		return p.User, nil
	}
	if !in.Kepadatan.Valid() {
		return User{}, ErrInvalidDensity
	}
	u, err := s.q.UpdateUserDensity(ctx, dbgen.UpdateUserDensityParams{ID: p.User.ID, Kepadatan: string(*in.Kepadatan)})
	if err != nil {
		return User{}, fmt.Errorf("simpan preferensi: %w", err)
	}
	return toUser(u), nil
}
