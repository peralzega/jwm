package automation_test

import (
	"context"
	"encoding/json"
	"fmt"
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
	"jwm/internal/automation"
	"jwm/internal/board"
	"jwm/internal/config"
	"jwm/internal/db/gen"
	"jwm/internal/project"
	"jwm/internal/server"
	"jwm/internal/testutil"
	"jwm/internal/user"
)

const goodPassword = "kuda-lari-di-sawah-42"

type fixture struct {
	t       *testing.T
	pool    *pgxpool.Pool
	hasher  *auth.Hasher
	url     string
	worker  *automation.Worker
	pm      *testutil.Client
	project string
	board   string
	group   string
	status  string
	person  string
	prio    string
	date    string
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
	srv := httptest.NewServer(server.NewRouter(server.Deps{
		Config: config.Config{LoginRatePerMinute: 1000}, Pool: pool, Auth: authSvc,
		User: user.NewService(pool, hasher), Project: project.NewService(pool), Board: board.NewService(pool),
		Automation: automation.NewService(pool), Log: log,
	}))
	t.Cleanup(srv.Close)
	fx := &fixture{t: t, pool: pool, hasher: hasher, url: srv.URL, worker: automation.NewWorker(pool, log, automation.DefaultLimits())}
	fx.worker.Now = func() time.Time { return time.Date(2026, 11, 5, 9, 0, 0, 0, time.Local) }
	pmUser := fx.seedUser(auth.RoleProjectManager)
	fx.pm = fx.login(pmUser)
	fx.project = fx.must(fx.pm, http.MethodPost, "/api/v1/projects", map[string]any{"nama": "Otomasi"}, http.StatusCreated).Body["id"].(string)
	b := fx.must(fx.pm, http.MethodPost, "/api/v1/projects/"+fx.project+"/boards", map[string]any{"nama": "Board"}, http.StatusCreated)
	fx.board = b.Body["id"].(string)
	fx.group = b.Body["grup"].([]any)[0].(map[string]any)["id"].(string)
	for _, c := range b.Body["kolom"].([]any) {
		m := c.(map[string]any)
		switch m["tipe"] {
		case "status":
			fx.status = m["id"].(string)
		case "person":
			fx.person = m["id"].(string)
		}
	}
	fx.prio = fx.column("priority")
	fx.date = fx.column("date")
	return fx
}

func (fx *fixture) column(tipe string) string {
	fx.t.Helper()
	return fx.must(fx.pm, http.MethodPost, "/api/v1/boards/"+fx.board+"/columns",
		map[string]any{"tipe": tipe, "judul": tipe}, http.StatusCreated).Body["id"].(string)
}

func (fx *fixture) newGroup(judul string) string {
	fx.t.Helper()
	return fx.must(fx.pm, http.MethodPost, "/api/v1/boards/"+fx.board+"/groups",
		map[string]any{"judul": judul}, http.StatusCreated).Body["id"].(string)
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

func (fx *fixture) member(peran auth.Role, peranProyek string) (dbgen.User, *testutil.Client) {
	fx.t.Helper()
	u := fx.seedUser(peran)
	if peranProyek != "" {
		_, err := fx.pool.Exec(context.Background(),
			"INSERT INTO project_members (project_id, user_id, peran_proyek) VALUES ($1, $2, $3)", fx.project, u.ID, peranProyek)
		require.NoError(fx.t, err)
	}
	return u, fx.login(u)
}

func (fx *fixture) must(c *testutil.Client, method, path string, body any, status int) testutil.Resp {
	fx.t.Helper()
	r := c.Do(method, path, body)
	require.Equal(fx.t, status, r.Status, r.Raw)
	return r
}

func (fx *fixture) item(judul string) string {
	fx.t.Helper()
	return fx.must(fx.pm, http.MethodPost, "/api/v1/boards/"+fx.board+"/items",
		map[string]any{"group_id": fx.group, "judul": judul}, http.StatusCreated).Body["id"].(string)
}

func (fx *fixture) set(item, col string, v any) {
	fx.t.Helper()
	fx.must(fx.pm, http.MethodPut, "/api/v1/items/"+item+"/values/"+col, map[string]any{"nilai": v}, http.StatusOK)
}

func (fx *fixture) rule(c *testutil.Client, body map[string]any) string {
	fx.t.Helper()
	if _, ok := body["aktif"]; !ok {
		body["aktif"] = true
	}
	return fx.must(c, http.MethodPost, "/api/v1/boards/"+fx.board+"/automations", body, http.StatusCreated).Body["id"].(string)
}

// drain memproses antrean sampai kosong (dengan batas agar loop tak terbatas gagal).
func (fx *fixture) drain() int {
	fx.t.Helper()
	total := 0
	for range 100 {
		n, err := fx.worker.ProcessBatch(context.Background())
		require.NoError(fx.t, err)
		if n == 0 {
			return total
		}
		total += n
	}
	fx.t.Fatal("antrean otomasi tidak kunjung kosong")
	return total
}

func (fx *fixture) itemState(item string) (group string, values map[string]any) {
	fx.t.Helper()
	r := fx.must(fx.pm, http.MethodGet, "/api/v1/items/"+item, nil, http.StatusOK)
	return r.Body["group_id"].(string), r.Body["nilai"].(map[string]any)
}

func (fx *fixture) runs(ruleID string) []map[string]any {
	fx.t.Helper()
	r := fx.must(fx.pm, http.MethodGet, "/api/v1/automations/"+ruleID+"/runs", nil, http.StatusOK)
	var out []map[string]any
	for _, x := range r.Body["items"].([]any) {
		out = append(out, x.(map[string]any))
	}
	return out
}

func statusTrigger(ke string) map[string]any {
	return map[string]any{"jenis": "status_berubah", "ke": ke}
}

func TestRuleCRUDValidationAndPermissions(t *testing.T) {
	fx := newFixture(t)
	selesai := fx.newGroup("Selesai")
	base := "/api/v1/boards/" + fx.board + "/automations"
	valid := map[string]any{
		"nama": "Done pindah ke Selesai", "pemicu": statusTrigger("done"),
		"kondisi": []any{}, "aksi": []any{map[string]any{"jenis": "pindah_grup", "group_id": selesai}},
	}
	id := fx.rule(fx.pm, valid)

	r := fx.must(fx.pm, http.MethodGet, base, nil, http.StatusOK)
	assert.Equal(t, true, r.Body["bisa_kelola"])
	require.Len(t, r.Body["items"], 1)
	got := r.Body["items"].([]any)[0].(map[string]any)
	assert.Equal(t, "Done pindah ke Selesai", got["nama"])
	assert.Equal(t, map[string]any{"jenis": "status_berubah", "ke": "done"}, got["pemicu"])

	otherBoard := fx.must(fx.pm, http.MethodPost, "/api/v1/projects/"+fx.project+"/boards", map[string]any{"nama": "Lain"}, http.StatusCreated)
	otherGroup := otherBoard.Body["grup"].([]any)[0].(map[string]any)["id"].(string)
	bad := []map[string]any{
		{"nama": "", "pemicu": statusTrigger("done"), "aksi": valid["aksi"]},
		{"nama": "x", "pemicu": map[string]any{"jenis": "entah"}, "aksi": valid["aksi"]},
		{"nama": "x", "pemicu": statusTrigger("label-palsu"), "aksi": valid["aksi"]},
		{"nama": "x", "pemicu": map[string]any{"jenis": "status_berubah", "column_id": fx.person}, "aksi": valid["aksi"]},
		{"nama": "x", "pemicu": map[string]any{"jenis": "tanggal_tiba"}, "aksi": valid["aksi"]},
		{"nama": "x", "pemicu": statusTrigger("done"), "aksi": []any{}},
		{"nama": "x", "pemicu": statusTrigger("done"), "aksi": []any{map[string]any{"jenis": "pindah_grup", "group_id": otherGroup}}},
		{"nama": "x", "pemicu": statusTrigger("done"), "aksi": []any{map[string]any{"jenis": "ubah_kolom", "column_id": fx.prio, "nilai": "darurat"}}},
		{"nama": "x", "pemicu": statusTrigger("done"), "aksi": []any{map[string]any{"jenis": "ubah_kolom", "column_id": fx.person, "nilai": []string{uuid.NewString()}}}},
		{"nama": "x", "pemicu": statusTrigger("done"), "kondisi": []any{map[string]any{"jenis": "di_grup", "group_id": otherGroup}}, "aksi": valid["aksi"]},
	}
	for i, b := range bad {
		assert.Equal(t, http.StatusUnprocessableEntity, fx.pm.Do(http.MethodPost, base, b).Status, "kasus %d", i)
	}
	assert.Equal(t, http.StatusBadRequest, fx.pm.Do(http.MethodPost, base, map[string]any{"nama": "x", "skrip": "rm -rf"}).Status)

	type role struct {
		name               string
		peran              auth.Role
		proyek             string
		list, create, edit int
		kelola             bool
	}
	for _, tt := range []role{
		{"Admin", auth.RoleAdmin, "", 200, 201, 200, true},
		{"PMO", auth.RolePMO, "", 200, 201, 200, true},
		{"Project Manager anggota manager", auth.RoleProjectManager, "manager", 200, 201, 200, true},
		{"member", auth.RoleMember, "member", 200, 403, 403, false},
		{"viewer", auth.RoleViewer, "viewer", 200, 403, 403, false},
		{"bukan anggota", auth.RoleMember, "", 404, 404, 404, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, c := fx.member(tt.peran, tt.proyek)
			r := c.Do(http.MethodGet, base, nil)
			require.Equal(t, tt.list, r.Status)
			if r.Status == http.StatusOK {
				assert.Equal(t, tt.kelola, r.Body["bisa_kelola"])
			}
			assert.Equal(t, tt.create, c.Do(http.MethodPost, base, valid).Status)
			assert.Equal(t, tt.edit, c.Do(http.MethodPatch, "/api/v1/automations/"+id, valid).Status)
			assert.Equal(t, tt.list, c.Do(http.MethodGet, "/api/v1/automations/"+id+"/runs", nil).Status)
		})
	}

	off := map[string]any{"nama": "Nonaktif", "pemicu": valid["pemicu"], "kondisi": []any{}, "aksi": valid["aksi"], "aktif": false}
	r = fx.must(fx.pm, http.MethodPatch, "/api/v1/automations/"+id, off, http.StatusOK)
	assert.Equal(t, false, r.Body["aktif"])

	fx.must(fx.pm, http.MethodDelete, "/api/v1/automations/"+id, nil, http.StatusNoContent)
	assert.Equal(t, http.StatusNotFound, fx.pm.Do(http.MethodDelete, "/api/v1/automations/"+id, nil).Status)

	var n int
	require.NoError(t, fx.pool.QueryRow(context.Background(),
		"SELECT count(*) FROM audit_log WHERE entitas = 'automation_rule' AND entitas_id = $1", id).Scan(&n))
	assert.GreaterOrEqual(t, n, 3) // buat, ubah (beberapa), hapus

	fx.must(fx.pm, http.MethodPost, "/api/v1/projects/"+fx.project+"/archive", nil, http.StatusOK)
	assert.Equal(t, http.StatusConflict, fx.pm.Do(http.MethodPost, base, valid).Status)
}

func TestRuleLimitPerBoard(t *testing.T) {
	fx := newFixture(t)
	body := func(i int) map[string]any {
		return map[string]any{"nama": fmt.Sprintf("Aturan %d", i), "pemicu": map[string]any{"jenis": "item_dibuat"},
			"aksi": []any{map[string]any{"jenis": "ubah_kolom", "column_id": fx.prio, "nilai": "rendah"}}, "aktif": false}
	}
	for i := range automation.MaxRulesPerBoard {
		fx.rule(fx.pm, body(i))
	}
	r := fx.pm.Do(http.MethodPost, "/api/v1/boards/"+fx.board+"/automations", body(99))
	assert.Equal(t, http.StatusConflict, r.Status)
	assert.Equal(t, "too_many_rules", r.Body["error"])
}

func TestRuleExecution(t *testing.T) {
	fx := newFixture(t)
	selesai := fx.newGroup("Selesai")
	pmMember, _ := fx.member(auth.RoleMember, "member")

	move := fx.rule(fx.pm, map[string]any{
		"nama": "Done ke Selesai", "pemicu": statusTrigger("done"),
		"aksi": []any{
			map[string]any{"jenis": "pindah_grup", "group_id": selesai},
			map[string]any{"jenis": "ubah_kolom", "column_id": fx.prio, "nilai": "rendah"},
		},
	})
	created := fx.rule(fx.pm, map[string]any{
		"nama": "Item baru: status awal", "pemicu": map[string]any{"jenis": "item_dibuat"},
		"aksi": []any{map[string]any{"jenis": "ubah_kolom", "column_id": fx.status, "nilai": "ready_to_start"}},
	})
	assigned := fx.rule(fx.pm, map[string]any{
		"nama": "Ditugaskan ke member: prioritas tinggi", "pemicu": map[string]any{"jenis": "penugasan_berubah"},
		"kondisi": []any{map[string]any{"jenis": "person_termasuk", "column_id": fx.person, "user_id": pmMember.ID.String()}},
		"aksi":    []any{map[string]any{"jenis": "ubah_kolom", "column_id": fx.prio, "nilai": "tinggi"}},
	})

	a := fx.item("A")
	fx.drain()
	_, vals := fx.itemState(a)
	assert.Equal(t, "ready_to_start", vals[fx.status], "pemicu item dibuat")
	require.Len(t, fx.runs(created), 1)
	assert.Equal(t, "berhasil", fx.runs(created)[0]["hasil"])

	fx.set(a, fx.status, "done")
	fx.drain()
	group, vals := fx.itemState(a)
	assert.Equal(t, selesai, group)
	assert.Equal(t, "rendah", vals[fx.prio])
	run := fx.runs(move)[0]
	assert.Equal(t, "berhasil", run["hasil"])
	assert.Contains(t, run["pesan"], `dipindah ke grup "Selesai"`)
	assert.Equal(t, "A", run["item_judul"])

	// Audit: aktor sistem (null) dan asal aturan tercatat.
	var aktor *string
	var rule string
	require.NoError(t, fx.pool.QueryRow(context.Background(), `
		SELECT aktor_id::text, sesudah -> 'otomasi' ->> 'rule_id' FROM audit_log
		WHERE entitas = 'item' AND entitas_id = $1 AND aksi = 'pindah' ORDER BY id DESC LIMIT 1`, a).Scan(&aktor, &rule))
	assert.Nil(t, aktor)
	assert.Equal(t, move, rule)

	// Kondisi tidak terpenuhi: dilewati beserta alasannya.
	fx.set(a, fx.person, []string{})
	fx.drain()
	pm2, _ := fx.member(auth.RoleMember, "member")
	fx.set(a, fx.person, []string{pm2.ID.String()})
	fx.drain()
	runs := fx.runs(assigned)
	assert.Equal(t, "dilewati", runs[0]["hasil"])
	assert.Contains(t, runs[0]["pesan"], "penanggung jawab")
	fx.set(a, fx.person, []string{pmMember.ID.String()})
	fx.drain()
	assert.Equal(t, "berhasil", fx.runs(assigned)[0]["hasil"])
	_, vals = fx.itemState(a)
	assert.Equal(t, "tinggi", vals[fx.prio])

	// Item sudah di grup Selesai dan prioritasnya rendah: aturan tidak menulis apa pun.
	fx.set(a, fx.prio, "rendah")
	fx.set(a, fx.status, "waiting")
	fx.set(a, fx.status, "done")
	fx.drain()
	assert.Contains(t, fx.runs(move)[0]["pesan"], "tidak ada perubahan")
}

func TestLoopPrevention(t *testing.T) {
	fx := newFixture(t)
	r1 := fx.rule(fx.pm, map[string]any{
		"nama": "done -> in_progress", "pemicu": statusTrigger("done"),
		"aksi": []any{map[string]any{"jenis": "ubah_kolom", "column_id": fx.status, "nilai": "in_progress"}},
	})
	r2 := fx.rule(fx.pm, map[string]any{
		"nama": "in_progress -> done", "pemicu": statusTrigger("in_progress"),
		"aksi": []any{map[string]any{"jenis": "ubah_kolom", "column_id": fx.status, "nilai": "done"}},
	})
	a := fx.item("Pingpong")
	fx.set(a, fx.status, "done")
	fx.drain() // berhenti karena pencegahan loop

	count := func(rule, hasil string) int {
		n := 0
		for _, r := range fx.runs(rule) {
			if r["hasil"] == hasil {
				n++
			}
		}
		return n
	}
	// Rantai: r1, r2, r1, r2, r1 (kedalaman 5), lalu r2 diblokir.
	assert.Equal(t, 3, count(r1, "berhasil"))
	assert.Equal(t, 2, count(r2, "berhasil"))
	assert.Equal(t, 1, count(r2, "diblokir_loop"))
	assert.Contains(t, fx.runs(r2)[0]["pesan"], "batas 5")
	var pending int
	require.NoError(t, fx.pool.QueryRow(context.Background(),
		"SELECT count(*) FROM automation_events WHERE diproses_pada IS NULL").Scan(&pending))
	assert.Zero(t, pending)
}

func TestDateArrivedTrigger(t *testing.T) {
	fx := newFixture(t)
	rule := fx.rule(fx.pm, map[string]any{
		"nama": "Tenggat hari ini: prioritas kritis", "pemicu": map[string]any{"jenis": "tanggal_tiba", "column_id": fx.date},
		"aksi": []any{map[string]any{"jenis": "ubah_kolom", "column_id": fx.prio, "nilai": "kritis"}},
	})
	a, b := fx.item("Hari ini"), fx.item("Besok")
	fx.set(a, fx.date, "2026-11-05")
	fx.set(b, fx.date, "2026-11-06")

	require.NoError(t, fx.worker.EnqueueDue(context.Background()))
	require.NoError(t, fx.worker.EnqueueDue(context.Background())) // tidak ganda
	fx.drain()
	_, vals := fx.itemState(a)
	assert.Equal(t, "kritis", vals[fx.prio])
	_, vals = fx.itemState(b)
	assert.Nil(t, vals[fx.prio])
	require.Len(t, fx.runs(rule), 1)

	// Hari berikutnya: item kedua.
	fx.worker.Now = func() time.Time { return time.Date(2026, 11, 6, 8, 0, 0, 0, time.Local) }
	require.NoError(t, fx.worker.EnqueueDue(context.Background()))
	fx.drain()
	_, vals = fx.itemState(b)
	assert.Equal(t, "kritis", vals[fx.prio])
}

func TestEventsOnlyWithActiveRulesAndFailures(t *testing.T) {
	fx := newFixture(t)
	a := fx.item("Tanpa aturan")
	fx.set(a, fx.status, "done")
	var n int
	require.NoError(t, fx.pool.QueryRow(context.Background(), "SELECT count(*) FROM automation_events").Scan(&n))
	assert.Zero(t, n, "board tanpa aturan aktif tidak mengisi antrean")

	// Aksi gagal (grup tujuan dihapus): run "gagal", item tidak berubah.
	tmp := fx.newGroup("Sementara")
	rule := fx.rule(fx.pm, map[string]any{
		"nama": "Pindah ke sementara", "pemicu": statusTrigger("waiting"),
		"aksi": []any{
			map[string]any{"jenis": "ubah_kolom", "column_id": fx.prio, "nilai": "tinggi"},
			map[string]any{"jenis": "pindah_grup", "group_id": tmp},
		},
	})
	fx.must(fx.pm, http.MethodDelete, "/api/v1/groups/"+tmp, nil, http.StatusNoContent)
	fx.set(a, fx.status, "waiting")
	fx.drain()
	run := fx.runs(rule)[0]
	assert.Equal(t, "gagal", run["hasil"])
	assert.Contains(t, run["pesan"], "grup tujuan sudah dihapus")
	_, vals := fx.itemState(a)
	assert.Nil(t, vals[fx.prio], "aksi pertama ikut dibatalkan (savepoint)")

	// Proyek diarsipkan: dilewati.
	fx.set(a, fx.status, "in_progress")
	fx.set(a, fx.status, "waiting")
	fx.must(fx.pm, http.MethodPost, "/api/v1/projects/"+fx.project+"/archive", nil, http.StatusOK)
	fx.drain()
	run = fx.runs(rule)[0]
	assert.Equal(t, "dilewati", run["hasil"])
	assert.Equal(t, "proyek diarsipkan", run["pesan"])

	raw, _ := json.Marshal(fx.runs(rule))
	assert.NotContains(t, string(raw), "panic")
}
