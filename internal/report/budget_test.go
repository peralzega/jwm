package report_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"jwm/internal/auth"
	"jwm/internal/testutil"
)

func (fx *fixture) member(projectID string, peran auth.Role, peranProyek string) *testutil.Client {
	fx.t.Helper()
	u := fx.seedUser(peran)
	if peranProyek != "" {
		_, err := fx.pool.Exec(context.Background(),
			"INSERT INTO project_members (project_id, user_id, peran_proyek) VALUES ($1, $2, $3)", projectID, u.ID, peranProyek)
		require.NoError(fx.t, err)
	}
	return fx.login(u)
}

func entry(jenis, jumlah string) map[string]any {
	return map[string]any{"tanggal": "2026-11-02", "jenis": jenis, "jumlah": jumlah, "keterangan": "uji"}
}

func TestBudgetTotalsAndValidation(t *testing.T) {
	fx := newFixture(t)
	p := fx.project("Keuangan", nil)
	path := "/api/v1/projects/" + p.id + "/budget"

	fx.must(fx.pm, http.MethodPost, path, entry("anggaran", "1000000.50"), http.StatusCreated)
	fx.must(fx.pm, http.MethodPost, path, entry("anggaran", "500000"), http.StatusCreated)
	id := fx.must(fx.pm, http.MethodPost, path, entry("realisasi", "250000.25"), http.StatusCreated).Body["id"].(string)

	r := fx.must(fx.pm, http.MethodGet, path, nil, http.StatusOK)
	assert.Equal(t, map[string]any{"anggaran": "1500000.50", "realisasi": "250000.25", "sisa": "1250000.25"}, r.Body["total"])
	assert.Equal(t, true, r.Body["bisa_ubah"])
	require.Len(t, r.Body["items"], 3)

	// Realisasi melebihi anggaran: sisa negatif tetap dilaporkan apa adanya.
	fx.must(fx.pm, http.MethodPatch, "/api/v1/budget-entries/"+id, entry("realisasi", "2000000"), http.StatusNoContent)
	r = fx.must(fx.pm, http.MethodGet, path, nil, http.StatusOK)
	assert.Equal(t, "-499999.50", r.Body["total"].(map[string]any)["sisa"])

	bad := []map[string]any{
		{"tanggal": "2026-02-30", "jenis": "anggaran", "jumlah": "1", "keterangan": ""},
		{"tanggal": "2026-11-02", "jenis": "biaya", "jumlah": "1", "keterangan": ""},
		{"tanggal": "2026-11-02", "jenis": "anggaran", "jumlah": "0", "keterangan": ""},
		{"tanggal": "2026-11-02", "jenis": "anggaran", "jumlah": "0.00", "keterangan": ""},
		{"tanggal": "2026-11-02", "jenis": "anggaran", "jumlah": "-5", "keterangan": ""},
		{"tanggal": "2026-11-02", "jenis": "anggaran", "jumlah": "1.234", "keterangan": ""},
		{"tanggal": "2026-11-02", "jenis": "anggaran", "jumlah": "1.000.000", "keterangan": ""},
		{"tanggal": "2026-11-02", "jenis": "anggaran", "jumlah": "1e6", "keterangan": ""},
		{"tanggal": "2026-11-02", "jenis": "anggaran", "jumlah": "1", "keterangan": strings.Repeat("x", 501)},
	}
	for _, b := range bad {
		assert.Equal(t, http.StatusUnprocessableEntity, fx.pm.Do(http.MethodPost, path, b).Status, b)
	}
	// Jumlah sebagai angka JSON ditolak (harus string desimal).
	assert.Equal(t, http.StatusBadRequest, fx.pm.Do(http.MethodPost, path,
		map[string]any{"tanggal": "2026-11-02", "jenis": "anggaran", "jumlah": 5, "keterangan": ""}).Status)
}

func TestBudgetPermissions(t *testing.T) {
	fx := newFixture(t)
	p := fx.project("Hak", nil)
	path := "/api/v1/projects/" + p.id + "/budget"
	id := fx.must(fx.pm, http.MethodPost, path, entry("anggaran", "100"), http.StatusCreated).Body["id"].(string)
	entryPath := "/api/v1/budget-entries/" + id

	tests := []struct {
		name               string
		who                *testutil.Client
		read, create, edit int
		bisaUbah           bool
	}{
		{"PMO tanpa keanggotaan", fx.member(p.id, auth.RolePMO, ""), 200, 201, 204, true},
		{"Project Manager proyek", fx.member(p.id, auth.RoleProjectManager, "manager"), 200, 201, 204, true},
		{"Admin tanpa keanggotaan", fx.member(p.id, auth.RoleAdmin, ""), 200, 403, 403, false},
		{"Project Manager sebagai member", fx.member(p.id, auth.RoleProjectManager, "member"), 200, 403, 403, false},
		{"member", fx.member(p.id, auth.RoleMember, "member"), 200, 403, 403, false},
		{"viewer", fx.member(p.id, auth.RoleViewer, "viewer"), 200, 403, 403, false},
		{"bukan anggota", fx.member(p.id, auth.RoleMember, ""), 404, 404, 404, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := tt.who.Do(http.MethodGet, path, nil)
			require.Equal(t, tt.read, r.Status, r.Raw)
			if tt.read == http.StatusOK {
				assert.Equal(t, tt.bisaUbah, r.Body["bisa_ubah"])
			}
			assert.Equal(t, tt.create, tt.who.Do(http.MethodPost, path, entry("realisasi", "1")).Status)
			assert.Equal(t, tt.edit, tt.who.Do(http.MethodPatch, entryPath, entry("anggaran", "100")).Status)
		})
	}
	assert.Equal(t, http.StatusNotFound, fx.pm.Do(http.MethodPatch, "/api/v1/budget-entries/"+uuid.NewString(), entry("anggaran", "1")).Status)

	fx.must(fx.pm, http.MethodDelete, entryPath, nil, http.StatusNoContent)
	assert.Equal(t, http.StatusNotFound, fx.pm.Do(http.MethodDelete, entryPath, nil).Status)

	var n int
	require.NoError(t, fx.pool.QueryRow(context.Background(),
		"SELECT count(*) FROM audit_log WHERE entitas = 'budget_entry'").Scan(&n))
	// 1 buat awal + 2 buat (PMO, PM) + 2 ubah + 1 hapus.
	assert.Equal(t, 6, n)

	fx.must(fx.pm, http.MethodPost, "/api/v1/projects/"+p.id+"/archive", nil, http.StatusOK)
	assert.Equal(t, http.StatusConflict, fx.pm.Do(http.MethodPost, path, entry("anggaran", "1")).Status)
	r := fx.must(fx.pm, http.MethodGet, path, nil, http.StatusOK)
	assert.Equal(t, false, r.Body["bisa_ubah"])
}
