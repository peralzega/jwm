package schedule_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"jwm/internal/auth"
	"jwm/internal/testutil"
)

func (fx *fixture) timelineColumn(board, judul string) string {
	fx.t.Helper()
	r := fx.manager.Do(http.MethodPost, "/api/v1/boards/"+board+"/columns", map[string]any{"tipe": "timeline", "judul": judul})
	require.Equal(fx.t, http.StatusCreated, r.Status, r.Raw)
	return r.Body["id"].(string)
}

func setTimeline(c *testutil.Client, item, col, mulai, selesai string) testutil.Resp {
	var nilai any
	if mulai != "" {
		nilai = map[string]string{"mulai": mulai, "selesai": selesai}
	}
	return c.Do(http.MethodPut, "/api/v1/items/"+item+"/values/"+col, map[string]any{"nilai": nilai})
}

func (fx *fixture) mustTimeline(item, col, mulai, selesai string) {
	fx.t.Helper()
	r := setTimeline(fx.manager, item, col, mulai, selesai)
	require.Equal(fx.t, http.StatusOK, r.Status, r.Raw)
}

// timeline membaca nilai timeline item sebagai "mulai..selesai" ("" bila kosong).
func (fx *fixture) timeline(item, col string) string {
	fx.t.Helper()
	var raw []byte
	err := fx.pool.QueryRow(context.Background(),
		"SELECT nilai_json FROM item_values WHERE item_id = $1 AND column_id = $2", item, col).Scan(&raw)
	if err != nil {
		return ""
	}
	var tl struct{ Mulai, Selesai string }
	require.NoError(fx.t, json.Unmarshal(raw, &tl))
	return tl.Mulai + ".." + tl.Selesai
}

func linkLag(t *testing.T, c *testutil.Client, board, pred, succ string, lag int) {
	t.Helper()
	r := c.Do(http.MethodPost, depsPath(board), map[string]any{"predecessor_id": pred, "successor_id": succ, "lag_hari": lag})
	require.Equal(t, http.StatusCreated, r.Status, r.Raw)
}

func schedulePath(board string) string { return "/api/v1/boards/" + board + "/schedule" }

// byItem mengubah items respons /schedule menjadi peta per item_id.
func byItem(t *testing.T, r testutil.Resp) map[string]map[string]any {
	t.Helper()
	require.Equal(t, http.StatusOK, r.Status, r.Raw)
	out := map[string]map[string]any{}
	for _, it := range r.Body["items"].([]any) {
		m := it.(map[string]any)
		out[m["item_id"].(string)] = m
	}
	return out
}

func (fx *fixture) auditCount(aksi, entitasID string) int {
	fx.t.Helper()
	var n int
	require.NoError(fx.t, fx.pool.QueryRow(context.Background(),
		"SELECT count(*) FROM audit_log WHERE aksi = $1 AND entitas_id = $2", aksi, entitasID).Scan(&n))
	return n
}

func TestScheduleEndpoint(t *testing.T) {
	fx := newFixture(t)
	col := fx.timelineColumn(fx.board, "Jadwal")
	a := fx.item(fx.manager, fx.board, fx.group, "A")
	b := fx.item(fx.manager, fx.board, fx.group, "B")
	u := fx.item(fx.manager, fx.board, fx.group, "U tanpa jadwal")
	c := fx.item(fx.manager, fx.board, fx.group, "C")
	x := fx.item(fx.manager, fx.board, fx.group, "X lepas")
	fx.mustTimeline(a, col, "2026-11-02", "2026-11-03")
	fx.mustTimeline(b, col, "2026-11-04", "2026-11-05")
	fx.mustTimeline(c, col, "2026-11-05", "2026-11-05") // melanggar: B selesai 5 Nov
	fx.mustTimeline(x, col, "2026-11-02", "2026-11-02")
	linkLag(t, fx.manager, fx.board, a, b, 0)
	linkLag(t, fx.manager, fx.board, b, u, 0)
	linkLag(t, fx.manager, fx.board, u, c, 0)

	r := fx.manager.Do(http.MethodGet, schedulePath(fx.board), nil)
	items := byItem(t, r)
	assert.Equal(t, col, r.Body["kolom_id"])
	assert.Equal(t, true, r.Body["kolom_utama"])
	assert.Equal(t, "2026-11-06", r.Body["selesai_proyek"])
	assert.Equal(t, true, items[a]["kritis"])
	assert.Equal(t, true, items[u]["tanpa_jadwal"])
	assert.Equal(t, "2026-11-06", items[u]["es"])
	assert.Equal(t, true, items[c]["melanggar_fs"])
	assert.Equal(t, "2026-11-06", items[c]["es"])
	assert.Equal(t, float64(4), items[x]["slack"])
	assert.Equal(t, false, items[x]["kritis"])
	require.Len(t, r.Body["dependensi_dilanggar"], 1)
	warn := r.Body["peringatan"].([]any)
	require.Len(t, warn, 1)
	assert.Equal(t, "kalender_kosong", warn[0].(map[string]any)["kode"])
	assert.Equal(t, float64(2026), warn[0].(map[string]any)["tahun"])

	// Libur 4 Nov dari kalender kerja: B (4–5 Nov) tinggal sehari kerja, slack X
	// berkurang, dan peringatan kalender hilang.
	admin := fx.login(fx.seedUser(auth.RoleAdmin))
	imp := admin.Do(http.MethodPost, "/api/v1/admin/work-calendar/import",
		map[string]any{"csv": "tanggal,keterangan\n2026-11-04,Libur uji\n"})
	require.Equal(t, http.StatusOK, imp.Status, imp.Raw)
	r = fx.manager.Do(http.MethodGet, schedulePath(fx.board), nil)
	items = byItem(t, r)
	assert.Equal(t, "2026-11-05", items[b]["es"])
	assert.Equal(t, "2026-11-05", items[b]["ef"])
	assert.Equal(t, "2026-11-06", items[c]["es"])
	assert.Equal(t, float64(3), items[x]["slack"])
	assert.Empty(t, r.Body["peringatan"])

	// Kolom timeline kedua: CPM tetap dihitung, tetapi bukan kolom utama.
	col2 := fx.timelineColumn(fx.board, "Rencana awal")
	r = fx.manager.Do(http.MethodGet, schedulePath(fx.board)+"?timeline="+col2, nil)
	require.Equal(t, http.StatusOK, r.Status, r.Raw)
	assert.Equal(t, false, r.Body["kolom_utama"])
	assert.Nil(t, r.Body["selesai_proyek"])

	r = fx.manager.Do(http.MethodGet, schedulePath(fx.board)+"?timeline="+fx.personCo, nil)
	assert.Equal(t, http.StatusUnprocessableEntity, r.Status, r.Raw)
	r = fx.manager.Do(http.MethodGet, schedulePath(fx.board)+"?timeline=bukan-uuid", nil)
	assert.Equal(t, http.StatusBadRequest, r.Status, r.Raw)
	r = fx.manager.Do(http.MethodGet, schedulePath(fx.board2), nil)
	assert.Equal(t, http.StatusUnprocessableEntity, r.Status, r.Raw)
	assert.Equal(t, "no_timeline", r.Body["error"])

	viewer := fx.seedUser(auth.RoleViewer)
	fx.addMember(viewer.ID, "viewer")
	assert.Equal(t, http.StatusOK, fx.login(viewer).Do(http.MethodGet, schedulePath(fx.board), nil).Status)
	outsider := fx.login(fx.seedUser(auth.RoleMember))
	assert.Equal(t, http.StatusNotFound, outsider.Do(http.MethodGet, schedulePath(fx.board), nil).Status)
}

func TestAutoShift(t *testing.T) {
	fx := newFixture(t)
	col := fx.timelineColumn(fx.board, "Jadwal")
	other := fx.timelineColumn(fx.board, "Rencana awal")
	a := fx.item(fx.manager, fx.board, fx.group, "A")
	b := fx.item(fx.manager, fx.board, fx.group, "B")
	c := fx.item(fx.manager, fx.board, fx.group, "C")
	fx.mustTimeline(a, col, "2026-11-02", "2026-11-03")
	fx.mustTimeline(b, col, "2026-11-04", "2026-11-05")
	fx.mustTimeline(c, col, "2026-11-06", "2026-11-06")
	linkLag(t, fx.manager, fx.board, a, b, 0)
	linkLag(t, fx.manager, fx.board, b, c, 1)

	// A diperpanjang dua hari: B dan C bergeser, durasi hari kerja tetap.
	r := setTimeline(fx.manager, a, col, "2026-11-02", "2026-11-05")
	require.Equal(t, http.StatusOK, r.Status, r.Raw)
	shifted := r.Body["geser_otomatis"].([]any)
	require.Len(t, shifted, 2)
	assert.Equal(t, "B", shifted[0].(map[string]any)["judul"])
	assert.Equal(t, "2026-11-06", shifted[0].(map[string]any)["mulai"])
	assert.Equal(t, "2026-11-09", shifted[0].(map[string]any)["selesai"])
	assert.Equal(t, "C", shifted[1].(map[string]any)["judul"])
	assert.Equal(t, b, shifted[1].(map[string]any)["predecessor_id"])
	assert.Equal(t, "2026-11-06..2026-11-09", fx.timeline(b, col))
	assert.Equal(t, "2026-11-11..2026-11-11", fx.timeline(c, col))

	// Audit mencatat pelaku asli dan item penyebab.
	var aktor, penyebab, judul string
	require.NoError(t, fx.pool.QueryRow(context.Background(),
		`SELECT u.email, l.sesudah->>'penyebab_id', l.sesudah->>'penyebab_judul'
		 FROM audit_log l JOIN users u ON u.id = l.aktor_id
		 WHERE l.aksi = 'geser_otomatis' AND l.entitas_id = $1`, c).Scan(&aktor, &penyebab, &judul))
	assert.Equal(t, fx.managerEmail(), aktor)
	assert.Equal(t, a, penyebab)
	assert.Equal(t, "A", judul)
	assert.Equal(t, 1, fx.auditCount("geser_otomatis", b))

	// Dipercepat: successor tidak ditarik mundur.
	r = setTimeline(fx.manager, a, col, "2026-11-02", "2026-11-02")
	require.Equal(t, http.StatusOK, r.Status, r.Raw)
	assert.Nil(t, r.Body["geser_otomatis"])
	assert.Equal(t, "2026-11-06..2026-11-09", fx.timeline(b, col))

	// Successor boleh dipindah mendahului predecessor; ditandai melanggar_fs.
	fx.mustTimeline(a, col, "2026-11-02", "2026-11-05")
	r = setTimeline(fx.manager, b, col, "2026-11-03", "2026-11-04")
	require.Equal(t, http.StatusOK, r.Status, r.Raw)
	assert.Nil(t, r.Body["geser_otomatis"], "C (11 Nov) masih memenuhi B+1")
	items := byItem(t, fx.manager.Do(http.MethodGet, schedulePath(fx.board), nil))
	assert.Equal(t, true, items[b]["melanggar_fs"])

	// Kolom timeline bukan utama tidak menggeser apa pun.
	fx.mustTimeline(b, other, "2026-11-02", "2026-11-02")
	fx.mustTimeline(c, other, "2026-11-02", "2026-11-02")
	r = setTimeline(fx.manager, b, other, "2026-11-02", "2026-11-20")
	require.Equal(t, http.StatusOK, r.Status, r.Raw)
	assert.Nil(t, r.Body["geser_otomatis"])
	assert.Equal(t, "2026-11-02..2026-11-02", fx.timeline(c, other))

	// Menghapus jadwal tidak menggeser apa pun.
	r = setTimeline(fx.manager, a, col, "", "")
	require.Equal(t, http.StatusOK, r.Status, r.Raw)
	assert.Nil(t, r.Body["geser_otomatis"])
}

func (fx *fixture) managerEmail() string {
	fx.t.Helper()
	r := fx.manager.Do(http.MethodGet, "/api/v1/auth/me", nil)
	require.Equal(fx.t, http.StatusOK, r.Status, r.Raw)
	if u, ok := r.Body["user"].(map[string]any); ok {
		return u["email"].(string)
	}
	return r.Body["email"].(string)
}

func TestAutoShiftPermissionsPerRole(t *testing.T) {
	type role struct {
		name   string
		global auth.Role
		proyek string // "" = bukan anggota proyek
	}
	tests := []struct {
		role
		// assignB dan assignC: pelaku ditugaskan pada successor tersebut.
		assignB, assignC bool
		status           int
		code             string
		mention          string // judul yang wajib disebut pesan 409
	}{
		{role: role{"admin", auth.RoleAdmin, ""}, status: http.StatusOK},
		{role: role{"pmo", auth.RolePMO, ""}, status: http.StatusOK},
		{role: role{"project manager", auth.RoleProjectManager, "manager"}, status: http.StatusOK},
		{role: role{"member tanpa hak atas successor", auth.RoleMember, "member"}, status: http.StatusConflict, code: "shift_forbidden", mention: `"B tugas lain"`},
		{role: role{"member ditugaskan sebagian rantai", auth.RoleMember, "member"}, assignB: true, status: http.StatusConflict, code: "shift_forbidden", mention: `"C tugas lain"`},
		{role: role{"member ditugaskan seluruh rantai", auth.RoleMember, "member"}, assignB: true, assignC: true, status: http.StatusOK},
		{role: role{"viewer", auth.RoleViewer, "viewer"}, status: http.StatusForbidden, code: "forbidden"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fx := newFixture(t)
			col := fx.timelineColumn(fx.board, "Jadwal")
			u := fx.seedUser(tt.global)
			if tt.proyek != "" {
				fx.addMember(u.ID, tt.proyek)
			}
			actor := fx.login(u)

			// A milik pelaku bila ia member (member hanya mengubah item buatannya
			// atau yang menugaskannya); selain itu dibuat manager.
			owner := fx.manager
			if tt.global == auth.RoleMember {
				owner = actor
			}
			a := fx.item(owner, fx.board, fx.group, "A milik pelaku")
			b := fx.item(fx.manager, fx.board, fx.group, "B tugas lain")
			c := fx.item(fx.manager, fx.board, fx.group, "C tugas lain")
			fx.mustTimeline(a, col, "2026-11-02", "2026-11-03")
			fx.mustTimeline(b, col, "2026-11-04", "2026-11-05")
			fx.mustTimeline(c, col, "2026-11-06", "2026-11-06")
			linkLag(t, fx.manager, fx.board, a, b, 0)
			linkLag(t, fx.manager, fx.board, b, c, 0)
			for item, on := range map[string]bool{b: tt.assignB, c: tt.assignC} {
				if on {
					r := fx.manager.Do(http.MethodPut, "/api/v1/items/"+item+"/values/"+fx.personCo,
						map[string]any{"nilai": []string{u.ID.String()}})
					require.Equal(t, http.StatusOK, r.Status, r.Raw)
				}
			}
			before := map[string]int{}
			for _, id := range []string{a, b, c} {
				before[id] = fx.auditCount("ubah_nilai", id) + fx.auditCount("geser_otomatis", id)
			}

			r := setTimeline(actor, a, col, "2026-11-02", "2026-11-04")
			require.Equal(t, tt.status, r.Status, r.Raw)
			if tt.code != "" {
				assert.Equal(t, tt.code, r.Body["error"])
			}
			if tt.mention != "" {
				assert.Contains(t, r.Body["message"], tt.mention)
			}
			if tt.status == http.StatusOK {
				assert.Equal(t, "2026-11-02..2026-11-04", fx.timeline(a, col))
				assert.Equal(t, "2026-11-05..2026-11-06", fx.timeline(b, col))
				assert.Equal(t, "2026-11-09..2026-11-09", fx.timeline(c, col))
				assert.Equal(t, 1, fx.auditCount("geser_otomatis", c))
				return
			}
			// Penolakan tidak menyisakan perubahan sebagian: nilai dan audit tetap.
			assert.Equal(t, "2026-11-02..2026-11-03", fx.timeline(a, col))
			assert.Equal(t, "2026-11-04..2026-11-05", fx.timeline(b, col))
			assert.Equal(t, "2026-11-06..2026-11-06", fx.timeline(c, col))
			for _, id := range []string{a, b, c} {
				assert.Equal(t, before[id], fx.auditCount("ubah_nilai", id)+fx.auditCount("geser_otomatis", id), id)
			}
		})
	}
}

// Perubahan timeline, pembuatan, dan penghapusan dependensi yang berjalan
// bersamaan tidak boleh deadlock atau gagal 500.
func TestAutoShiftConcurrentWithDependencies(t *testing.T) {
	fx := newFixture(t)
	col := fx.timelineColumn(fx.board, "Jadwal")
	ids := make([]string, 6)
	for i := range ids {
		ids[i] = fx.item(fx.manager, fx.board, fx.group, string(rune('A'+i)))
		fx.mustTimeline(ids[i], col, "2026-11-02", "2026-11-02")
	}
	linkLag(t, fx.manager, fx.board, ids[0], ids[1], 0)
	linkLag(t, fx.manager, fx.board, ids[1], ids[2], 0)
	for round := range 8 {
		var wg sync.WaitGroup
		statuses := make(chan int, 4)
		run := func(f func() testutil.Resp) {
			wg.Add(1)
			go func() { defer wg.Done(); statuses <- f().Status }()
		}
		end := []string{"2026-11-03", "2026-11-10"}[round%2]
		run(func() testutil.Resp { return setTimeline(fx.manager, ids[0], col, "2026-11-02", end) })
		run(func() testutil.Resp { return setTimeline(fx.manager, ids[1], col, "2026-11-02", end) })
		run(func() testutil.Resp { return link(fx.manager, fx.board, ids[2], ids[3+round%3]) })
		run(func() testutil.Resp {
			for _, d := range fx.manager.Do(http.MethodGet, depsPath(fx.board), nil).Body["items"].([]any) {
				m := d.(map[string]any)
				if m["predecessor_id"] == ids[2] {
					return fx.manager.Do(http.MethodDelete, "/api/v1/dependencies/"+m["id"].(string), nil)
				}
			}
			return testutil.Resp{Status: http.StatusNoContent}
		})
		wg.Wait()
		close(statuses)
		for s := range statuses {
			assert.Less(t, s, 500, "round %d", round)
		}
	}
}

func TestWorkCalendarAdmin(t *testing.T) {
	fx := newFixture(t)
	admin := fx.login(fx.seedUser(auth.RoleAdmin))
	pmo := fx.login(fx.seedUser(auth.RolePMO))
	const base = "/api/v1/admin/work-calendar"

	// Hanya Admin yang mengubah kalender; semua pengguna login dapat membaca.
	r := pmo.Do(http.MethodPut, base+"/2026-12-25", map[string]any{"keterangan": "Natal"})
	assert.Equal(t, http.StatusForbidden, r.Status, r.Raw)
	r = admin.Do(http.MethodPut, base+"/2026-12-25", map[string]any{"keterangan": "  Natal  "})
	require.Equal(t, http.StatusOK, r.Status, r.Raw)
	assert.Equal(t, "Natal", r.Body["keterangan"])
	r = admin.Do(http.MethodPut, base+"/2026-12-26", map[string]any{"hari_kerja": true, "keterangan": "Sabtu masuk"})
	require.Equal(t, http.StatusOK, r.Status, r.Raw)
	r = admin.Do(http.MethodPut, base+"/2026-02-30", map[string]any{"keterangan": "x"})
	assert.Equal(t, http.StatusUnprocessableEntity, r.Status, r.Raw)
	r = admin.Do(http.MethodPut, base+"/2026-12-24", map[string]any{"keterangan": strings.Repeat("x", 201)})
	assert.Equal(t, http.StatusUnprocessableEntity, r.Status, r.Raw)

	r = fx.manager.Do(http.MethodGet, "/api/v1/work-calendar?tahun=2026", nil)
	require.Equal(t, http.StatusOK, r.Status, r.Raw)
	require.Len(t, r.Body["items"], 2)
	assert.Equal(t, []any{float64(2026)}, r.Body["tahun_terisi"])
	assert.Equal(t, http.StatusUnprocessableEntity, fx.manager.Do(http.MethodGet, "/api/v1/work-calendar?tahun=99999", nil).Status)

	assert.Equal(t, http.StatusForbidden, pmo.Do(http.MethodDelete, base+"/2026-12-26", nil).Status)
	assert.Equal(t, http.StatusNoContent, admin.Do(http.MethodDelete, base+"/2026-12-26", nil).Status)
	assert.Equal(t, http.StatusNotFound, admin.Do(http.MethodDelete, base+"/2026-12-26", nil).Status)
	assert.Equal(t, 1, fx.auditCount("hapus", "2026-12-26"))
	assert.Equal(t, 2, fx.auditCount("ubah", "2026-12-26")+fx.auditCount("ubah", "2026-12-25"))
}

func TestWorkCalendarImport(t *testing.T) {
	fx := newFixture(t)
	admin := fx.login(fx.seedUser(auth.RoleAdmin))
	const path = "/api/v1/admin/work-calendar/import"
	year := func(y string) []any {
		r := fx.manager.Do(http.MethodGet, "/api/v1/work-calendar?tahun="+y, nil)
		require.Equal(t, http.StatusOK, r.Status, r.Raw)
		return r.Body["items"].([]any)
	}

	// Galat dikumpulkan per baris dan tidak ada yang tersimpan.
	bad := "tanggal,keterangan,hari_kerja\n2027-01-01,Tahun Baru,\n01/05/2027,Format salah,\n2027-01-01,Ganda,\n2027-03-20,Hari aneh,mungkin\n"
	r := admin.Do(http.MethodPost, path, map[string]any{"csv": bad})
	require.Equal(t, http.StatusUnprocessableEntity, r.Status, r.Raw)
	assert.Equal(t, "invalid_csv", r.Body["error"])
	msg := r.Body["message"].(string)
	assert.Contains(t, msg, "baris 3")
	assert.Contains(t, msg, "baris 4")
	assert.Contains(t, msg, "sudah ada di baris 2")
	assert.Contains(t, msg, "baris 5")
	assert.Empty(t, year("2027"))

	for _, tc := range []struct{ name, csv string }{
		{"tanpa header tanggal", "tgl,keterangan\n2027-01-01,x\n"},
		{"kolom tidak dikenal", "tanggal,catatan\n2027-01-01,x\n"},
		{"kosong", ""},
		{"hanya header", "tanggal\n"},
		{"tanda kutip rusak", "tanggal,keterangan\n2027-01-01,\"tidak ditutup\n"},
	} {
		r := admin.Do(http.MethodPost, path, map[string]any{"csv": tc.csv})
		assert.Equal(t, http.StatusUnprocessableEntity, r.Status, tc.name+": "+r.Raw)
	}
	assert.Equal(t, http.StatusForbidden,
		fx.manager.Do(http.MethodPost, path, map[string]any{"csv": "tanggal\n2027-01-01\n"}).Status)

	// Berkas Excel: BOM, titik koma, urutan kolom bebas, baris kosong.
	excel := string(rune(0xFEFF)) + "keterangan;tanggal;hari_kerja\r\nTahun Baru;2027-01-01;\r\n\r\nSabtu masuk;2027-01-09;ya\r\nNatal;2027-12-25;tidak\r\n"
	r = admin.Do(http.MethodPost, path, map[string]any{"csv": excel})
	require.Equal(t, http.StatusOK, r.Status, r.Raw)
	assert.Equal(t, float64(3), r.Body["jumlah"])
	got := year("2027")
	require.Len(t, got, 3)
	assert.Equal(t, map[string]any{"tanggal": "2027-01-09", "hari_kerja": true, "keterangan": "Sabtu masuk"}, got[1])

	// ganti = true menyamakan tahun 2027 dengan berkas; tahun lain tidak tersentuh.
	r = admin.Do(http.MethodPut, "/api/v1/admin/work-calendar/2028-01-01", map[string]any{"keterangan": "Tahun Baru"})
	require.Equal(t, http.StatusOK, r.Status, r.Raw)
	r = admin.Do(http.MethodPost, path, map[string]any{"csv": "tanggal,keterangan\n2027-03-10,Nyepi\n", "ganti": true})
	require.Equal(t, http.StatusOK, r.Status, r.Raw)
	assert.Equal(t, float64(3), r.Body["dihapus"])
	got = year("2027")
	require.Len(t, got, 1)
	assert.Equal(t, "2027-03-10", got[0].(map[string]any)["tanggal"])
	assert.Len(t, year("2028"), 1)

	var n int
	require.NoError(t, fx.pool.QueryRow(context.Background(),
		"SELECT count(*) FROM audit_log WHERE entitas = 'work_calendar' AND aksi = 'impor'").Scan(&n))
	assert.Equal(t, 2, n)
}
