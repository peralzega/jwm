package report_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"jwm/internal/auth"
)

// setStatus mengubah status lewat API (tercatat di audit), lalu menetapkan
// waktu catatan audit terakhir item agar tren dapat diuji tanpa bergantung jam.
func (fx *fixture) setStatus(p proj, item, label string, at time.Time) {
	fx.t.Helper()
	fx.must(fx.pm, http.MethodPut, "/api/v1/items/"+item+"/values/"+p.status, map[string]any{"nilai": label}, http.StatusOK)
	_, err := fx.pool.Exec(context.Background(), `
		UPDATE audit_log SET waktu = $2
		WHERE id = (SELECT max(id) FROM audit_log WHERE entitas = 'item' AND entitas_id = $1 AND aksi = 'ubah_nilai')`, item, at)
	require.NoError(fx.t, err)
}

func at(s string) time.Time {
	t, err := time.ParseInLocation("2006-01-02 15:04", s, time.Local)
	if err != nil {
		panic(err)
	}
	return t
}

func TestDashboardPanels(t *testing.T) {
	fx := newFixture(t)
	pmID := fx.pmUser.ID
	p := fx.project("Panel", nil)
	q := fx.project("Lain", nil)
	mem := fx.seedUser(auth.RoleMember)
	_, err := fx.pool.Exec(context.Background(),
		"INSERT INTO project_members (project_id, user_id, peran_proyek) VALUES ($1, $2, 'member')", p.id, mem.ID)
	require.NoError(t, err)

	a := fx.item(p, "A", "", -1, "", "", nil)
	fx.setStatus(p, a, "done", at("2026-11-03 10:00"))
	// B selesai, dibuka lagi, lalu selesai lagi: hanya transisi terakhir dihitung.
	b := fx.item(p, "B", "", -1, "", "", nil)
	fx.setStatus(p, b, "done", at("2026-10-06 09:00"))
	fx.setStatus(p, b, "in_progress", at("2026-10-07 09:00"))
	fx.setStatus(p, b, "done", at("2026-10-20 09:00"))
	// C pernah selesai tetapi dibuka lagi: tidak masuk tren.
	c := fx.item(p, "C", "", -1, "", "", nil)
	fx.setStatus(p, c, "done", at("2026-10-27 09:00"))
	fx.setStatus(p, c, "in_progress", at("2026-10-28 09:00"))
	fx.item(p, "D", "in_progress", -1, "2026-11-02", "2026-11-04", &pmID) // terlambat, 3 hari kerja
	e := fx.item(p, "E", "", -1, "2026-11-09", "2026-11-13", &pmID)       // 5 hari kerja
	fx.must(fx.pm, http.MethodPut, "/api/v1/items/"+e+"/values/"+p.person,
		map[string]any{"nilai": []string{pmID.String(), mem.ID.String()}}, http.StatusOK)
	fx.item(p, "F", "waiting", -1, "", "", nil)
	fx.item(q, "Q1", "done", -1, "", "", &pmID)

	budget := "/api/v1/projects/" + p.id + "/budget"
	fx.must(fx.pm, http.MethodPost, budget, map[string]any{"tanggal": "2026-11-01", "jenis": "anggaran", "jumlah": "100", "keterangan": ""}, http.StatusCreated)
	fx.must(fx.pm, http.MethodPost, budget, map[string]any{"tanggal": "2026-11-02", "jenis": "realisasi", "jumlah": "40.5", "keterangan": ""}, http.StatusCreated)

	r := fx.must(fx.pm, http.MethodGet, "/api/v1/dashboard?project="+p.id, nil, http.StatusOK)
	assert.Equal(t, p.id, r.Body["project_id"])
	assert.Equal(t, "2026-11-05", r.Body["hari_ini"])
	assert.Equal(t, float64(6), r.Body["jumlah_item"])
	assert.Equal(t, []any{
		map[string]any{"label": "Done", "warna": "hijau", "jumlah": float64(2)},
		map[string]any{"label": "In progress", "warna": "oranye", "jumlah": float64(2)},
		map[string]any{"label": "Waiting", "warna": "merah", "jumlah": float64(1)},
	}, r.Body["status"])
	assert.Equal(t, float64(1), r.Body["tanpa_status"])

	beban := r.Body["beban"].([]any)
	require.Len(t, beban, 2)
	assert.Equal(t, map[string]any{"user_id": pmID.String(), "nama": fx.pmUser.Nama,
		"item_terbuka": float64(2), "hari_kerja": float64(8), "terlambat": float64(1)}, beban[0])
	assert.Equal(t, float64(1), beban[1].(map[string]any)["item_terbuka"])
	assert.Equal(t, float64(5), beban[1].(map[string]any)["hari_kerja"])
	assert.Equal(t, float64(2), r.Body["belum_ditugaskan"]) // C dan F

	tren := r.Body["tren"].([]any)
	require.Len(t, tren, 12)
	assert.Equal(t, "2026-08-17", tren[0].(map[string]any)["minggu_mulai"])
	assert.Equal(t, "2026-11-02", tren[11].(map[string]any)["minggu_mulai"])
	counts := map[string]float64{}
	for _, w := range tren {
		m := w.(map[string]any)
		if n := m["selesai"].(float64); n > 0 {
			counts[m["minggu_mulai"].(string)] = n
		}
	}
	assert.Equal(t, map[string]float64{"2026-11-02": 1, "2026-10-19": 1}, counts)

	assert.Equal(t, []any{map[string]any{
		"project_id": p.id, "nama": "Panel", "anggaran": "100.00", "realisasi": "40.50", "sisa": "59.50", "jumlah_entri": float64(2),
	}}, r.Body["keuangan"])

	// Semua proyek: status digabung lintas proyek, keuangan per proyek.
	r = fx.must(fx.pm, http.MethodGet, "/api/v1/dashboard", nil, http.StatusOK)
	assert.Nil(t, r.Body["project_id"])
	assert.Equal(t, float64(7), r.Body["jumlah_item"])
	assert.Equal(t, float64(3), r.Body["status"].([]any)[0].(map[string]any)["jumlah"])
	assert.Len(t, r.Body["keuangan"], 2)

	// Hak akses: non-anggota tidak melihat proyek; parameter rusak ditolak.
	outsider := fx.login(fx.seedUser(auth.RoleMember))
	assert.Equal(t, http.StatusNotFound, outsider.Do(http.MethodGet, "/api/v1/dashboard?project="+p.id, nil).Status)
	r = fx.must(outsider, http.MethodGet, "/api/v1/dashboard", nil, http.StatusOK)
	assert.Equal(t, float64(0), r.Body["jumlah_item"])
	assert.Empty(t, r.Body["keuangan"])
	assert.Equal(t, http.StatusBadRequest, fx.pm.Do(http.MethodGet, "/api/v1/dashboard?project=x", nil).Status)
	assert.Equal(t, http.StatusNotFound, fx.pm.Do(http.MethodGet, "/api/v1/dashboard?project="+uuid.NewString(), nil).Status)
}
