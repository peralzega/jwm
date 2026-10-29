package board

import "github.com/google/uuid"

// positionStep adalah jarak urutan standar antar elemen.
const positionStep = 1024

// minGap adalah celah terkecil sebelum urutan grup perlu ditata ulang.
const minGap = 1e-6

// between menghitung urutan di antara prev dan next (nil berarti tidak ada
// tetangga di sisi itu). ok=false bila celahnya terlalu sempit dan grup
// perlu ditata ulang terlebih dahulu.
func between(prev, next *float64) (float64, bool) {
	switch {
	case prev == nil && next == nil:
		return positionStep, true
	case prev == nil:
		return *next - positionStep, true
	case next == nil:
		return *prev + positionStep, true
	}
	if *next-*prev < minGap {
		return 0, false
	}
	return *prev + (*next-*prev)/2, true
}

// canEditItem: manager mengubah semua item; member hanya item yang ia buat
// atau yang menugaskannya (keputusan pengguna untuk M1d); viewer tidak.
func canEditItem(eff ProjectRole, createdBy, actor uuid.UUID, assigned bool) bool {
	switch {
	case eff >= RoleManager:
		return true
	case eff >= RoleMember:
		return createdBy == actor || assigned
	}
	return false
}

// canDeleteItem: manager menghapus semua item; member hanya item buatannya.
func canDeleteItem(eff ProjectRole, createdBy, actor uuid.UUID) bool {
	switch {
	case eff >= RoleManager:
		return true
	case eff >= RoleMember:
		return createdBy == actor
	}
	return false
}
