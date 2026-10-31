package report_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"jwm/internal/auth"
	"jwm/internal/board"
	"jwm/internal/config"
	"jwm/internal/db/gen"
	"jwm/internal/project"
	"jwm/internal/report"
	"jwm/internal/schedule"
	"jwm/internal/server"
	"jwm/internal/testutil"
	"jwm/internal/user"
)

const goodPassword = "kuda-lari-di-sawah-42"

// Hari ini untuk semua tes: Kamis 5 November 2026.
var today = time.Date(2026, 11, 5, 9, 0, 0, 0, time.Local)

type fixture struct {
	t      *testing.T
	pool   *pgxpool.Pool
	hasher *auth.Hasher
	url    string
	pm     *testutil.Client
	pmUser dbgen.User
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	pool := testutil.NewPool(t)
	hasher, err := auth.NewHasher(auth.Argon2Params{Memory: 1024, Time: 1, Threads: 1, SaltLen: 16, KeyLen: 32})
	require.NoError(t, err)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	authSvc := auth.NewService(pool, hasher, auth.Settings{
		SessionIdleTTL: time.Hour, SessionAbsoluteTTL: time.Hour, MaxFailures: 5,
		LockoutBase: time.Minute, LockoutMax: time.Hour,
	}, log)
	rep := report.NewService(pool, report.DefaultThresholds())
	rep.SetClock(func() time.Time { return today })
	srv := httptest.NewServer(server.NewRouter(server.Deps{
		Config: config.Config{LoginRatePerMinute: 1000}, Pool: pool, Auth: authSvc,
		User: user.NewService(pool, hasher), Project: project.NewService(pool), Board: board.NewService(pool),
		Schedule: schedule.NewService(pool), Report: rep, Log: log,
	}))
	t.Cleanup(srv.Close)
	fx := &fixture{t: t, pool: pool, hasher: hasher, url: srv.URL}
	fx.pmUser = fx.seedUser(auth.RoleProjectManager)
	fx.pm = fx.login(fx.pmUser)
	return fx
}

func (fx *fixture) seedUser(peran auth.Role) dbgen.User {
	fx.t.Helper()
	hash, err := fx.hasher.Hash(goodPassword)
	require.NoError(fx.t, err)
	email := string(peran) + "-" + uuid.NewString()[:8] + "@jiep.co.id"
	u, err := dbgen.New(fx.pool).CreateUser(context.Background(), dbgen.CreateUserParams{
		Nama: email, Email: email, PasswordHash: hash, Peran: string(peran),
	})
	require.NoError(fx.t, err)
	return u
}

func (fx *fixture) login(u dbgen.User) *testutil.Client {
	fx.t.Helper()
	c := testutil.NewClient(fx.t, fx.url)
	r := c.Login(u.Email, goodPassword)
	require.Equal(fx.t, http.StatusOK, r.Status, r.Raw)
	return c
}

func (fx *fixture) must(c *testutil.Client, method, path string, body any, status int) testutil.Resp {
	fx.t.Helper()
	r := c.Do(method, path, body)
	require.Equal(fx.t, status, r.Status, r.Raw)
	return r
}

// proj adalah proyek uji dengan satu board berkolom status, person, progress, dan timeline.
type proj struct {
	id, board, group             string
	status, person, prog, timeln string
}

func (fx *fixture) project(nama string, extra map[string]any) proj {
	fx.t.Helper()
	body := map[string]any{"nama": nama}
	for k, v := range extra {
		body[k] = v
	}
	p := proj{id: fx.must(fx.pm, http.MethodPost, "/api/v1/projects", body, http.StatusCreated).Body["id"].(string)}
	b := fx.must(fx.pm, http.MethodPost, "/api/v1/projects/"+p.id+"/boards", map[string]any{"nama": "Board " + nama}, http.StatusCreated)
	p.board = b.Body["id"].(string)
	p.group = b.Body["grup"].([]any)[0].(map[string]any)["id"].(string)
	for _, c := range b.Body["kolom"].([]any) {
		m := c.(map[string]any)
		switch m["tipe"] {
		case "status":
			p.status = m["id"].(string)
		case "person":
			p.person = m["id"].(string)
		}
	}
	col := func(tipe string) string {
		return fx.must(fx.pm, http.MethodPost, "/api/v1/boards/"+p.board+"/columns",
			map[string]any{"tipe": tipe, "judul": tipe}, http.StatusCreated).Body["id"].(string)
	}
	p.prog, p.timeln = col("progress"), col("timeline")
	return p
}

// item membuat item dengan nilai: status, progres (-1 = kosong), timeline, dan penanggung jawab.
func (fx *fixture) item(p proj, judul, status string, progres int, mulai, selesai string, assignee *uuid.UUID) string {
	fx.t.Helper()
	id := fx.must(fx.pm, http.MethodPost, "/api/v1/boards/"+p.board+"/items",
		map[string]any{"group_id": p.group, "judul": judul}, http.StatusCreated).Body["id"].(string)
	set := func(col string, v any) {
		fx.must(fx.pm, http.MethodPut, "/api/v1/items/"+id+"/values/"+col, map[string]any{"nilai": v}, http.StatusOK)
	}
	if status != "" {
		set(p.status, status)
	}
	if progres >= 0 {
		set(p.prog, progres)
	}
	if mulai != "" {
		set(p.timeln, map[string]string{"mulai": mulai, "selesai": selesai})
	}
	if assignee != nil {
		set(p.person, []string{assignee.String()})
	}
	return id
}

func dashboard(t *testing.T, c *testutil.Client, query string) map[string]map[string]any {
	t.Helper()
	r := c.Do(http.MethodGet, "/api/v1/portfolio"+query, nil)
	require.Equal(t, http.StatusOK, r.Status, r.Raw)
	out := map[string]map[string]any{}
	for _, it := range r.Body["items"].([]any) {
		m := it.(map[string]any)
		out[m["nama"].(string)] = m
	}
	return out
}

func TestDashboardComputesHealth(t *testing.T) {
	fx := newFixture(t)
	pmID := fx.pmUser.ID
	p := fx.project("Jaringan", map[string]any{"selesai": "2026-11-20", "prioritas": "tinggi"})
	// Done tanpa nilai progres: 100%. Durasi 2–6 Nov = 5 hari kerja.
	fx.item(p, "Survei", "done", -1, "2026-11-02", "2026-11-06", &pmID)
	// Lewat tenggat, tanpa penanggung jawab.
	fx.item(p, "Pengadaan", "in_progress", 30, "2026-11-02", "2026-11-04", nil)
	// Tenggat Senin (sisa 2 hari kerja) dan progres 20%.
	fx.item(p, "Instalasi", "", 20, "2026-11-04", "2026-11-09", &pmID)
	// Selesai sehari sebelum tenggat proyek (slack 1) dan tanpa jadwal: tidak berisiko.
	fx.item(p, "Uji", "", 0, "2026-11-16", "2026-11-19", &pmID)
	fx.item(p, "Catatan", "", -1, "", "", &pmID)

	rows := dashboard(t, fx.pm, "")
	j := rows["Jaringan"]
	require.NotNil(t, j)
	assert.Equal(t, float64(5), j["jumlah_item"])
	assert.Equal(t, float64(2), j["item_berisiko"])
	assert.Equal(t, map[string]any{
		"lewat_tenggat": float64(1), "tenggat_dekat": float64(1), "tanpa_penanggung_jawab": float64(1),
		"predecessor_terlambat": float64(0), "melewati_tenggat_proyek": float64(0),
	}, j["per_aturan"])
	// 2 dari 5 item berisiko (40%) > 25%: Merah.
	assert.Equal(t, "merah", j["health"])
	assert.Equal(t, "merah", j["health_otomatis"])
	assert.Contains(t, j["health_alasan"], "40% item berisiko (Merah bila > 25%).")
	// Bobot: Survei 5×100, Pengadaan 3×30, Instalasi 4×20, Uji 4×0, Catatan 1×0 → 670/17.
	assert.InDelta(t, 39.4, j["progres"], 0.05)
	assert.Equal(t, "tinggi", j["prioritas"])
	assert.Equal(t, "2026-11-20", j["selesai"])
	assert.Nil(t, j["status_manual"])

	// Rincian risiko per item, dengan nama board dan alasan.
	r := fx.must(fx.pm, http.MethodGet, "/api/v1/portfolio/projects/"+p.id+"/risks", nil, http.StatusOK)
	items := r.Body["items"].([]any)
	require.Len(t, items, 2)
	byName := map[string]map[string]any{}
	for _, it := range items {
		m := it.(map[string]any)
		byName[m["judul"].(string)] = m
		assert.Equal(t, "Board Jaringan", m["board_nama"])
	}
	alasan := byName["Pengadaan"]["alasan"].([]any)
	require.Len(t, alasan, 2)
	assert.Equal(t, "lewat_tenggat", alasan[0].(map[string]any)["kode"])
	assert.Equal(t, "Lewat tenggat 2026-11-04 dan belum Done.", alasan[0].(map[string]any)["pesan"])
	assert.Equal(t, "tanpa_penanggung_jawab", alasan[1].(map[string]any)["kode"])
	assert.Contains(t, byName["Instalasi"]["alasan"].([]any)[0].(map[string]any)["pesan"], "tinggal 2 hari kerja")
}

func TestDashboardCriticalPathRules(t *testing.T) {
	fx := newFixture(t)
	pmID := fx.pmUser.ID
	p := fx.project("Gedung", map[string]any{"selesai": "2026-11-13"})
	a := fx.item(p, "Fondasi", "", 50, "2026-11-02", "2026-11-04", &pmID) // lewat tenggat
	b := fx.item(p, "Struktur", "", 0, "2026-11-05", "2026-11-13", &pmID)
	fx.must(fx.pm, http.MethodPost, "/api/v1/boards/"+p.board+"/dependencies",
		map[string]any{"predecessor_id": a, "successor_id": b}, http.StatusCreated)

	j := dashboard(t, fx.pm, "")["Gedung"]
	// Fondasi kritis dan terlambat: Merah. Struktur kritis dengan predecessor
	// terlambat dan berakhir tepat di tenggat proyek (slack 0).
	assert.Equal(t, "merah", j["health"])
	assert.Equal(t, float64(1), j["kritis_terlambat"])
	per := j["per_aturan"].(map[string]any)
	assert.Equal(t, float64(1), per["predecessor_terlambat"])
	// Hanya Struktur (selesai tepat di tenggat); Fondasi masih punya slack.
	assert.Equal(t, float64(1), per["melewati_tenggat_proyek"])
	assert.Contains(t, j["health_alasan"], "1 item di jalur kritis terlambat.")
}

func TestDashboardVisibilityAndFilters(t *testing.T) {
	fx := newFixture(t)
	pf := fx.must(fx.login(fx.seedUser(auth.RolePMO)), http.MethodPost, "/api/v1/portfolios",
		map[string]any{"nama": "Infrastruktur"}, http.StatusCreated).Body["id"].(string)
	a := fx.project("Alpha", map[string]any{"prioritas": "kritis"})
	b := fx.project("Beta", map[string]any{"prioritas": "rendah"})
	arsip := fx.project("Arsip", nil)
	fx.must(fx.pm, http.MethodPatch, "/api/v1/projects/"+a.id, map[string]any{"portfolio_id": pf}, http.StatusOK)
	fx.must(fx.pm, http.MethodPost, "/api/v1/projects/"+arsip.id+"/archive", nil, http.StatusOK)
	fx.item(b, "Telat", "", 0, "2026-11-02", "2026-11-03", nil) // Beta Merah

	member := fx.seedUser(auth.RoleMember)
	_, err := fx.pool.Exec(context.Background(),
		"INSERT INTO project_members (project_id, user_id, peran_proyek) VALUES ($1, $2, 'member')", b.id, member.ID)
	require.NoError(t, err)

	tests := []struct {
		name  string
		who   *testutil.Client
		query string
		want  []string
	}{
		{"Admin melihat semua proyek aktif", fx.login(fx.seedUser(auth.RoleAdmin)), "", []string{"Alpha", "Beta"}},
		{"PMO melihat semua proyek aktif", fx.login(fx.seedUser(auth.RolePMO)), "", []string{"Alpha", "Beta"}},
		{"member hanya proyeknya", fx.login(member), "", []string{"Beta"}},
		{"non-anggota tidak melihat apa pun", fx.login(fx.seedUser(auth.RoleProjectManager)), "", nil},
		{"filter prioritas", fx.pm, "?prioritas=kritis,tinggi", []string{"Alpha"}},
		{"filter pemilik", fx.pm, "?pemilik=" + fx.pmUser.ID.String(), []string{"Alpha", "Beta"}},
		{"filter status", fx.pm, "?status=merah", []string{"Beta"}},
		{"filter portofolio", fx.pm, "?portofolio=" + pf, []string{"Alpha"}},
		{"tanpa portofolio", fx.pm, "?portofolio=tanpa", []string{"Beta"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows := dashboard(t, tt.who, tt.query)
			var got []string
			for n := range rows {
				got = append(got, n)
			}
			assert.ElementsMatch(t, tt.want, got)
		})
	}
	assert.Equal(t, "Infrastruktur", dashboard(t, fx.pm, "")["Alpha"]["portfolio_nama"])
	for _, q := range []string{"?prioritas=darurat", "?status=biru", "?pemilik=x", "?portofolio=x"} {
		assert.Equal(t, http.StatusBadRequest, fx.pm.Do(http.MethodGet, "/api/v1/portfolio"+q, nil).Status, q)
	}
	outsider := fx.login(fx.seedUser(auth.RoleMember))
	assert.Equal(t, http.StatusNotFound, outsider.Do(http.MethodGet, "/api/v1/portfolio/projects/"+a.id+"/risks", nil).Status)
}

func TestManualStatusOverride(t *testing.T) {
	fx := newFixture(t)
	p := fx.project("Gamma", nil)
	fx.item(p, "Telat", "", 0, "2026-11-02", "2026-11-03", nil)
	path := "/api/v1/projects/" + p.id + "/status-manual"
	pmoUser := fx.seedUser(auth.RolePMO)
	pmo := fx.login(pmoUser)
	viewer := fx.seedUser(auth.RoleViewer)
	_, err := fx.pool.Exec(context.Background(),
		"INSERT INTO project_members (project_id, user_id, peran_proyek) VALUES ($1, $2, 'viewer')", p.id, viewer.ID)
	require.NoError(t, err)

	tests := []struct {
		name   string
		who    *testutil.Client
		body   map[string]any
		status int
	}{
		{"Project Manager tidak boleh", fx.pm, map[string]any{"status": "hijau", "alasan": "x"}, http.StatusForbidden},
		{"viewer tidak boleh", fx.login(viewer), map[string]any{"status": "hijau", "alasan": "x"}, http.StatusForbidden},
		{"status tidak dikenal", pmo, map[string]any{"status": "biru", "alasan": "x"}, http.StatusUnprocessableEntity},
		{"alasan kosong", pmo, map[string]any{"status": "hijau", "alasan": "  "}, http.StatusUnprocessableEntity},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.status, tt.who.Do(http.MethodPut, path, tt.body).Status)
		})
	}

	fx.must(pmo, http.MethodPut, path, map[string]any{"status": "hijau", "alasan": "Vendor sudah konfirmasi jadwal baru."}, http.StatusNoContent)
	g := dashboard(t, fx.pm, "")["Gamma"]
	assert.Equal(t, "hijau", g["health"])
	assert.Equal(t, "merah", g["health_otomatis"])
	sm := g["status_manual"].(map[string]any)
	assert.Equal(t, "Vendor sudah konfirmasi jadwal baru.", sm["alasan"])
	assert.Equal(t, pmoUser.Nama, sm["oleh_nama"])
	assert.ElementsMatch(t, []string{"Gamma"}, keys(dashboard(t, fx.pm, "?status=hijau")))

	admin := fx.login(fx.seedUser(auth.RoleAdmin))
	fx.must(admin, http.MethodDelete, path, nil, http.StatusNoContent)
	g = dashboard(t, fx.pm, "")["Gamma"]
	assert.Equal(t, "merah", g["health"])
	assert.Nil(t, g["status_manual"])

	var n int
	require.NoError(t, fx.pool.QueryRow(context.Background(),
		"SELECT count(*) FROM audit_log WHERE entitas_id = $1 AND aksi IN ('timpa_status', 'hapus_timpa_status')", p.id).Scan(&n))
	assert.Equal(t, 2, n)

	fx.must(fx.pm, http.MethodPost, "/api/v1/projects/"+p.id+"/archive", nil, http.StatusOK)
	assert.Equal(t, http.StatusConflict, pmo.Do(http.MethodPut, path, map[string]any{"status": "hijau", "alasan": "x"}).Status)
}

func keys(m map[string]map[string]any) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestPortfolioCRUD(t *testing.T) {
	fx := newFixture(t)
	pmo := fx.login(fx.seedUser(auth.RolePMO))
	const base = "/api/v1/portfolios"

	assert.Equal(t, http.StatusForbidden, fx.pm.Do(http.MethodPost, base, map[string]any{"nama": "X"}).Status)
	id := fx.must(pmo, http.MethodPost, base, map[string]any{"nama": " Transformasi Digital "}, http.StatusCreated).Body["id"].(string)
	assert.Equal(t, http.StatusConflict, pmo.Do(http.MethodPost, base, map[string]any{"nama": "transformasi digital"}).Status)
	assert.Equal(t, http.StatusUnprocessableEntity, pmo.Do(http.MethodPost, base, map[string]any{"nama": ""}).Status)
	fx.must(pmo, http.MethodPatch, base+"/"+id, map[string]any{"nama": "Digital"}, http.StatusOK)
	assert.Equal(t, http.StatusForbidden, fx.pm.Do(http.MethodPatch, base+"/"+id, map[string]any{"nama": "Y"}).Status)

	// Project Manager memilih portofolio proyeknya; perubahan lain tidak melepasnya.
	p := fx.project("Delta", nil)
	assert.Equal(t, http.StatusUnprocessableEntity,
		fx.pm.Do(http.MethodPatch, "/api/v1/projects/"+p.id, map[string]any{"portfolio_id": uuid.NewString()}).Status)
	fx.must(fx.pm, http.MethodPatch, "/api/v1/projects/"+p.id, map[string]any{"portfolio_id": id}, http.StatusOK)
	r := fx.must(fx.pm, http.MethodPatch, "/api/v1/projects/"+p.id, map[string]any{"prioritas": "tinggi"}, http.StatusOK)
	assert.Equal(t, id, r.Body["portfolio_id"])
	r = fx.must(fx.pm, http.MethodGet, "/api/v1/projects/"+p.id, nil, http.StatusOK)
	assert.Equal(t, "Digital", r.Body["portfolio_nama"])

	list := fx.must(fx.pm, http.MethodGet, base, nil, http.StatusOK).Body["items"].([]any)
	require.Len(t, list, 1)
	assert.Equal(t, float64(1), list[0].(map[string]any)["jumlah_proyek"])

	// Menghapus portofolio melepas proyeknya.
	assert.Equal(t, http.StatusForbidden, fx.pm.Do(http.MethodDelete, base+"/"+id, nil).Status)
	fx.must(pmo, http.MethodDelete, base+"/"+id, nil, http.StatusNoContent)
	assert.Equal(t, http.StatusNotFound, pmo.Do(http.MethodDelete, base+"/"+id, nil).Status)
	r = fx.must(fx.pm, http.MethodGet, "/api/v1/projects/"+p.id, nil, http.StatusOK)
	assert.Nil(t, r.Body["portfolio_id"])
	fx.must(fx.pm, http.MethodPatch, "/api/v1/projects/"+p.id, map[string]any{"portfolio_id": nil}, http.StatusOK)

	var n int
	require.NoError(t, fx.pool.QueryRow(context.Background(),
		"SELECT count(*) FROM audit_log WHERE entitas = 'portfolio'").Scan(&n))
	assert.Equal(t, 3, n)
}
