package board_test

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
	"jwm/internal/server"
	"jwm/internal/testutil"
	"jwm/internal/user"
)

const goodPassword = "kuda-lari-di-sawah-42"

type env struct {
	t      *testing.T
	pool   *pgxpool.Pool
	hasher *auth.Hasher
	url    string
}

func newEnv(t *testing.T) *env {
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
		User: user.NewService(pool, hasher), Project: project.NewService(pool), Board: board.NewService(pool), Log: log,
	}))
	t.Cleanup(srv.Close)
	return &env{t: t, pool: pool, hasher: hasher, url: srv.URL}
}

func (e *env) seedUser(peran auth.Role) dbgen.User {
	e.t.Helper()
	hash, err := e.hasher.Hash(goodPassword)
	require.NoError(e.t, err)
	email := string(peran) + "-" + uuid.NewString()[:8] + "@jiep.co.id"
	u, err := dbgen.New(e.pool).CreateUser(context.Background(), dbgen.CreateUserParams{
		Nama: email, Email: email, PasswordHash: hash, Peran: string(peran), WajibGantiSandi: false,
	})
	require.NoError(e.t, err)
	return u
}

func (e *env) login(u dbgen.User) *testutil.Client {
	e.t.Helper()
	c := testutil.NewClient(e.t, e.url)
	r := c.Login(u.Email, goodPassword)
	require.Equal(e.t, http.StatusOK, r.Status, r.Raw)
	return c
}

func (e *env) addMember(projectID, userID uuid.UUID, role string) {
	e.t.Helper()
	_, err := e.pool.Exec(context.Background(),
		"INSERT INTO project_members (project_id, user_id, peran_proyek) VALUES ($1, $2, $3)", projectID, userID, role)
	require.NoError(e.t, err)
}

func (e *env) count(sql string, args ...any) int {
	e.t.Helper()
	var n int
	require.NoError(e.t, e.pool.QueryRow(context.Background(), sql, args...).Scan(&n))
	return n
}

// fixture: satu proyek milik manager dengan satu board bawaan.
type fixture struct {
	*env
	manager   *testutil.Client
	managerID uuid.UUID
	projectID uuid.UUID
	boardID   uuid.UUID
	groupID   uuid.UUID
	statusCol uuid.UUID
	personCol uuid.UUID
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	e := newEnv(t)
	pm := e.seedUser(auth.RoleProjectManager)
	c := e.login(pm)
	r := c.Do(http.MethodPost, "/api/v1/projects", map[string]any{"nama": "Proyek"})
	require.Equal(t, http.StatusCreated, r.Status, r.Raw)
	pid := uuid.MustParse(r.Body["id"].(string))

	r = c.Do(http.MethodPost, "/api/v1/projects/"+pid.String()+"/boards", map[string]any{"nama": "Board utama"})
	require.Equal(t, http.StatusCreated, r.Status, r.Raw)
	fx := &fixture{env: e, manager: c, managerID: pm.ID, projectID: pid, boardID: uuid.MustParse(r.Body["id"].(string))}
	fx.groupID = uuid.MustParse(r.Body["grup"].([]any)[0].(map[string]any)["id"].(string))
	for _, col := range r.Body["kolom"].([]any) {
		m := col.(map[string]any)
		switch m["tipe"] {
		case "status":
			fx.statusCol = uuid.MustParse(m["id"].(string))
		case "person":
			fx.personCol = uuid.MustParse(m["id"].(string))
		}
	}
	return fx
}

func (fx *fixture) boardPath(suffix string) string {
	return "/api/v1/boards/" + fx.boardID.String() + suffix
}

func itemPath(id uuid.UUID, suffix string) string { return "/api/v1/items/" + id.String() + suffix }

func createItem(t *testing.T, c *testutil.Client, fx *fixture, groupID uuid.UUID, judul string) uuid.UUID {
	t.Helper()
	r := c.Do(http.MethodPost, fx.boardPath("/items"), map[string]any{"group_id": groupID, "judul": judul})
	require.Equal(t, http.StatusCreated, r.Status, r.Raw)
	return uuid.MustParse(r.Body["id"].(string))
}

func (fx *fixture) createGroup(judul string) uuid.UUID {
	fx.t.Helper()
	r := fx.manager.Do(http.MethodPost, fx.boardPath("/groups"), map[string]any{"judul": judul})
	require.Equal(fx.t, http.StatusCreated, r.Status, r.Raw)
	return uuid.MustParse(r.Body["id"].(string))
}

func TestCreateBoardDefaults(t *testing.T) {
	fx := newFixture(t)
	r := fx.manager.Do(http.MethodGet, fx.boardPath(""), nil)
	require.Equal(t, http.StatusOK, r.Status, r.Raw)
	assert.Len(t, r.Body["grup"], 1)
	cols := r.Body["kolom"].([]any)
	require.Len(t, cols, 2)
	status := cols[0].(map[string]any)
	assert.Equal(t, "status", status["tipe"])
	var labels []string
	for _, l := range status["konfigurasi"].(map[string]any)["labels"].([]any) {
		labels = append(labels, l.(map[string]any)["judul"].(string))
	}
	assert.Equal(t, []string{"Ready to start", "In progress", "Waiting", "Done"}, labels)
	assert.Equal(t, "manager", r.Body["peran_saya"])

	// Rute /projects/{id} tetap berfungsi berdampingan dengan /projects/{id}/boards.
	assert.Equal(t, http.StatusOK, fx.manager.Do(http.MethodGet, "/api/v1/projects/"+fx.projectID.String(), nil).Status)
	assert.Len(t, fx.manager.Do(http.MethodGet, "/api/v1/projects/"+fx.projectID.String()+"/boards", nil).Body["items"], 1)

	member := fx.seedUser(auth.RoleMember)
	fx.addMember(fx.projectID, member.ID, "member")
	mc := fx.login(member)
	assert.Equal(t, http.StatusForbidden, mc.Do(http.MethodPost, "/api/v1/projects/"+fx.projectID.String()+"/boards",
		map[string]any{"nama": "X"}).Status, "hanya manager mengelola board")
	assert.Equal(t, http.StatusForbidden, mc.Do(http.MethodPost, fx.boardPath("/columns"),
		map[string]any{"tipe": "text", "judul": "X"}).Status)
	assert.Equal(t, http.StatusForbidden, mc.Do(http.MethodPost, fx.boardPath("/groups"),
		map[string]any{"judul": "X"}).Status)

	outsider := fx.login(fx.seedUser(auth.RoleProjectManager))
	assert.Equal(t, http.StatusNotFound, outsider.Do(http.MethodGet, fx.boardPath(""), nil).Status)
	assert.Equal(t, http.StatusNotFound, outsider.Do(http.MethodGet, fx.boardPath("/items"), nil).Status)
	assert.Equal(t, 1, fx.count("SELECT count(*) FROM audit_log WHERE entitas = 'board' AND aksi = 'buat'"))
}

// Kriteria penerimaan M1: item dapat dibuat, diubah, dipindah grup, dan dihapus lunak;
// setiap perubahan tercatat di audit_log.
func TestItemLifecycle(t *testing.T) {
	fx := newFixture(t)
	g2 := fx.createGroup("Selesai")
	member := fx.seedUser(auth.RoleMember)
	fx.addMember(fx.projectID, member.ID, "member")
	c := fx.login(member)

	id := createItem(t, c, fx, fx.groupID, "Siapkan dokumen")
	r := c.Do(http.MethodGet, itemPath(id, ""), nil)
	require.Equal(t, http.StatusOK, r.Status)
	assert.Equal(t, true, r.Body["bisa_ubah"])
	assert.Equal(t, true, r.Body["bisa_hapus"])

	r = c.Do(http.MethodPatch, itemPath(id, ""), map[string]any{"judul": "Siapkan dokumen final"})
	require.Equal(t, http.StatusOK, r.Status, r.Raw)
	assert.Equal(t, "Siapkan dokumen final", r.Body["judul"])

	r = c.Do(http.MethodPut, itemPath(id, "/values/"+fx.statusCol.String()), map[string]any{"nilai": "in_progress"})
	require.Equal(t, http.StatusOK, r.Status, r.Raw)
	assert.Equal(t, "in_progress", r.Body["nilai"].(map[string]any)[fx.statusCol.String()])

	r = c.Do(http.MethodPost, itemPath(id, "/move"), map[string]any{"group_id": g2})
	require.Equal(t, http.StatusOK, r.Status, r.Raw)
	assert.Equal(t, g2.String(), r.Body["group_id"])

	require.Equal(t, http.StatusNoContent, c.Do(http.MethodDelete, itemPath(id, ""), nil).Status)
	assert.Equal(t, http.StatusNotFound, c.Do(http.MethodGet, itemPath(id, ""), nil).Status)
	assert.Empty(t, c.Do(http.MethodGet, fx.boardPath("/items"), nil).Body["items"])
	assert.Equal(t, 1, fx.count("SELECT count(*) FROM items WHERE id = $1 AND dihapus_pada IS NOT NULL", id),
		"hapus lunak: baris tetap ada")

	var aksi []string
	rows, err := fx.pool.Query(context.Background(),
		"SELECT aksi FROM audit_log WHERE entitas = 'item' AND entitas_id = $1 ORDER BY id", id.String())
	require.NoError(t, err)
	for rows.Next() {
		var a string
		require.NoError(t, rows.Scan(&a))
		aksi = append(aksi, a)
	}
	require.NoError(t, rows.Err())
	assert.Equal(t, []string{"buat", "ubah_judul", "ubah_nilai", "pindah", "hapus"}, aksi)
}

func TestItemPermissionMatrix(t *testing.T) {
	fx := newFixture(t)
	member := fx.seedUser(auth.RoleMember)
	viewer := fx.seedUser(auth.RoleViewer)
	fx.addMember(fx.projectID, member.ID, "member")
	fx.addMember(fx.projectID, viewer.ID, "viewer")
	mc, vc := fx.login(member), fx.login(viewer)
	outsider := fx.login(fx.seedUser(auth.RoleMember))

	byManager := createItem(t, fx.manager, fx, fx.groupID, "Milik manager")
	byMember := createItem(t, mc, fx, fx.groupID, "Milik member")
	rename := func(c *testutil.Client, id uuid.UUID) int {
		return c.Do(http.MethodPatch, itemPath(id, ""), map[string]any{"judul": "Ganti"}).Status
	}

	// Member: item orang lain hanya-baca sampai ia ditugaskan.
	r := mc.Do(http.MethodGet, itemPath(byManager, ""), nil)
	assert.Equal(t, false, r.Body["bisa_ubah"])
	assert.Equal(t, http.StatusForbidden, rename(mc, byManager))
	assert.Equal(t, http.StatusForbidden, mc.Do(http.MethodPost, itemPath(byManager, "/move"), map[string]any{"group_id": fx.groupID}).Status)
	assert.Equal(t, http.StatusForbidden, mc.Do(http.MethodPut, itemPath(byManager, "/values/"+fx.statusCol.String()),
		map[string]any{"nilai": "done"}).Status)

	require.Equal(t, http.StatusOK, fx.manager.Do(http.MethodPut, itemPath(byManager, "/values/"+fx.personCol.String()),
		map[string]any{"nilai": []uuid.UUID{member.ID}}).Status)
	r = mc.Do(http.MethodGet, itemPath(byManager, ""), nil)
	assert.Equal(t, true, r.Body["bisa_ubah"])
	assert.Equal(t, false, r.Body["bisa_hapus"])
	assert.Equal(t, http.StatusOK, rename(mc, byManager), "member yang ditugaskan boleh mengubah")
	assert.Equal(t, http.StatusForbidden, mc.Do(http.MethodDelete, itemPath(byManager, ""), nil).Status,
		"member yang ditugaskan tidak boleh menghapus item orang lain")

	// Viewer: hanya baca, termasuk membuat item.
	assert.Equal(t, http.StatusForbidden, rename(vc, byMember))
	assert.Equal(t, http.StatusForbidden, vc.Do(http.MethodPost, fx.boardPath("/items"),
		map[string]any{"group_id": fx.groupID, "judul": "X"}).Status)
	assert.Equal(t, http.StatusOK, vc.Do(http.MethodGet, itemPath(byMember, ""), nil).Status)

	// Non-anggota: tidak tahu item itu ada.
	assert.Equal(t, http.StatusNotFound, outsider.Do(http.MethodGet, itemPath(byMember, ""), nil).Status)
	assert.Equal(t, http.StatusNotFound, rename(outsider, byMember))

	// Hapus: member untuk item buatannya; manager untuk semua.
	assert.Equal(t, http.StatusNoContent, mc.Do(http.MethodDelete, itemPath(byMember, ""), nil).Status)
	assert.Equal(t, http.StatusNoContent, fx.manager.Do(http.MethodDelete, itemPath(byManager, ""), nil).Status)
}

func TestValueValidationAndClearing(t *testing.T) {
	fx := newFixture(t)
	id := createItem(t, fx.manager, fx, fx.groupID, "Item")
	put := func(col uuid.UUID, nilai any) testutil.Resp {
		return fx.manager.Do(http.MethodPut, itemPath(id, "/values/"+col.String()), map[string]any{"nilai": nilai})
	}
	newCol := func(tipe string) uuid.UUID {
		r := fx.manager.Do(http.MethodPost, fx.boardPath("/columns"), map[string]any{"tipe": tipe, "judul": tipe})
		require.Equal(t, http.StatusCreated, r.Status, r.Raw)
		return uuid.MustParse(r.Body["id"].(string))
	}
	timeline, progress, dep := newCol("timeline"), newCol("progress"), newCol("dependency")
	outsider := fx.seedUser(auth.RoleMember)

	tests := []struct {
		name  string
		col   uuid.UUID
		nilai any
		want  int
	}{
		{"status valid", fx.statusCol, "done", http.StatusOK},
		{"status asing", fx.statusCol, "selesai", http.StatusUnprocessableEntity},
		{"person anggota", fx.personCol, []uuid.UUID{fx.managerID}, http.StatusOK},
		{"person bukan anggota", fx.personCol, []uuid.UUID{outsider.ID}, http.StatusUnprocessableEntity},
		{"timeline valid", timeline, map[string]string{"mulai": "2026-11-02", "selesai": "2026-11-20"}, http.StatusOK},
		{"timeline terbalik", timeline, map[string]string{"mulai": "2026-11-20", "selesai": "2026-11-02"}, http.StatusUnprocessableEntity},
		{"progres valid", progress, 40, http.StatusOK},
		{"progres 150", progress, 150, http.StatusUnprocessableEntity},
		{"dependency hanya-baca", dep, []string{}, http.StatusUnprocessableEntity},
		{"kolom tidak ada", uuid.New(), "x", http.StatusUnprocessableEntity},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := put(tt.col, tt.nilai)
			assert.Equal(t, tt.want, r.Status, r.Raw)
		})
	}

	r := fx.manager.Do(http.MethodGet, itemPath(id, ""), nil)
	nilai := r.Body["nilai"].(map[string]any)
	assert.Equal(t, "done", nilai[fx.statusCol.String()])
	assert.EqualValues(t, 40, nilai[progress.String()])

	r = put(fx.statusCol, nil)
	require.Equal(t, http.StatusOK, r.Status, r.Raw)
	assert.NotContains(t, r.Body["nilai"], fx.statusCol.String(), "null menghapus sel")

	// Kolom dari board lain ditolak.
	r = fx.manager.Do(http.MethodPost, "/api/v1/projects/"+fx.projectID.String()+"/boards", map[string]any{"nama": "Lain"})
	require.Equal(t, http.StatusCreated, r.Status)
	otherStatus := r.Body["kolom"].([]any)[0].(map[string]any)["id"].(string)
	assert.Equal(t, http.StatusUnprocessableEntity, put(uuid.MustParse(otherStatus), "done").Status)

	var sebelum, sesudah *string
	require.NoError(t, fx.pool.QueryRow(context.Background(),
		`SELECT sebelum->>'nilai', sesudah->>'nilai' FROM audit_log
		 WHERE aksi = 'ubah_nilai' AND sesudah->>'column_id' = $1 ORDER BY id DESC LIMIT 1`, fx.statusCol.String()).
		Scan(&sebelum, &sesudah))
	require.NotNil(t, sebelum)
	assert.Equal(t, "done", *sebelum)
	assert.Nil(t, sesudah, "penghapusan sel tercatat dengan nilai sesudah null")
}

func orderOf(t *testing.T, c *testutil.Client, fx *fixture, groupID uuid.UUID) []string {
	t.Helper()
	r := c.Do(http.MethodGet, fx.boardPath("/items?group_id="+groupID.String()), nil)
	require.Equal(t, http.StatusOK, r.Status)
	type it struct {
		judul  string
		urutan float64
	}
	var items []it
	for _, x := range r.Body["items"].([]any) {
		m := x.(map[string]any)
		items = append(items, it{m["judul"].(string), m["urutan"].(float64)})
	}
	// Urutkan tampilan menurut urutan, seperti yang dilakukan klien.
	for i := range items {
		for j := i + 1; j < len(items); j++ {
			if items[j].urutan < items[i].urutan {
				items[i], items[j] = items[j], items[i]
			}
		}
	}
	out := make([]string, len(items))
	for i, x := range items {
		out[i] = x.judul
	}
	return out
}

func TestMoveOrdering(t *testing.T) {
	fx := newFixture(t)
	g2 := fx.createGroup("B")
	a := createItem(t, fx.manager, fx, fx.groupID, "A")
	createItem(t, fx.manager, fx, fx.groupID, "B")
	c := createItem(t, fx.manager, fx, fx.groupID, "C")
	move := func(id, group uuid.UUID, after *uuid.UUID) testutil.Resp {
		body := map[string]any{"group_id": group}
		if after != nil {
			body["setelah_item_id"] = after
		}
		return fx.manager.Do(http.MethodPost, itemPath(id, "/move"), body)
	}

	require.Equal(t, http.StatusOK, move(c, fx.groupID, &a).Status)
	assert.Equal(t, []string{"A", "C", "B"}, orderOf(t, fx.manager, fx, fx.groupID))
	require.Equal(t, http.StatusOK, move(c, fx.groupID, nil).Status)
	assert.Equal(t, []string{"C", "A", "B"}, orderOf(t, fx.manager, fx, fx.groupID))

	// Sisip berulang di titik yang sama memaksa penataan ulang urutan grup.
	x := createItem(t, fx.manager, fx, fx.groupID, "X")
	y := createItem(t, fx.manager, fx, fx.groupID, "Y")
	for range 60 {
		require.Equal(t, http.StatusOK, move(x, fx.groupID, &a).Status)
		require.Equal(t, http.StatusOK, move(y, fx.groupID, &a).Status)
	}
	got := orderOf(t, fx.manager, fx, fx.groupID)
	assert.Equal(t, []string{"C", "A", "Y", "X", "B"}, got)

	require.Equal(t, http.StatusOK, move(a, g2, nil).Status)
	assert.Equal(t, []string{"A"}, orderOf(t, fx.manager, fx, g2))

	tests := []struct {
		name  string
		group uuid.UUID
		after *uuid.UUID
	}{
		{"setelah dirinya sendiri", g2, &a},
		{"acuan di grup lain", g2, &c},
		{"grup tidak ada", uuid.New(), nil},
	}
	for _, tt := range tests {
		assert.Equal(t, http.StatusUnprocessableEntity, move(a, tt.group, tt.after).Status, tt.name)
	}

	r := fx.manager.Do(http.MethodPost, "/api/v1/projects/"+fx.projectID.String()+"/boards", map[string]any{"nama": "Lain"})
	otherGroup := uuid.MustParse(r.Body["grup"].([]any)[0].(map[string]any)["id"].(string))
	assert.Equal(t, http.StatusUnprocessableEntity, move(a, otherGroup, nil).Status, "grup board lain")
}

func TestColumnsAndGroups(t *testing.T) {
	fx := newFixture(t)
	for _, tipe := range []string{"text", "status", "person", "date", "timeline", "number", "progress", "priority", "dependency"} {
		r := fx.manager.Do(http.MethodPost, fx.boardPath("/columns"), map[string]any{"tipe": tipe, "judul": "Kolom " + tipe})
		assert.Equal(t, http.StatusCreated, r.Status, "%s: %s", tipe, r.Raw)
	}
	assert.Equal(t, http.StatusUnprocessableEntity, fx.manager.Do(http.MethodPost, fx.boardPath("/columns"),
		map[string]any{"tipe": "formula", "judul": "X"}).Status)

	// Label status yang masih dipakai tidak boleh dihapus dari konfigurasi.
	id := createItem(t, fx.manager, fx, fx.groupID, "Item")
	require.Equal(t, http.StatusOK, fx.manager.Do(http.MethodPut, itemPath(id, "/values/"+fx.statusCol.String()),
		map[string]any{"nilai": "waiting"}).Status)
	onlyDone := map[string]any{"konfigurasi": map[string]any{"labels": []map[string]string{{"id": "done", "judul": "Done", "warna": "hijau"}}}}
	r := fx.manager.Do(http.MethodPatch, "/api/v1/columns/"+fx.statusCol.String(), onlyDone)
	assert.Equal(t, http.StatusConflict, r.Status)
	assert.Equal(t, "label_in_use", r.Body["error"])
	keepWaiting := map[string]any{"judul": "Status kerja", "konfigurasi": map[string]any{"labels": []map[string]string{
		{"id": "waiting", "judul": "Menunggu", "warna": "kuning"}, {"id": "done", "judul": "Selesai", "warna": "hijau"}}}}
	r = fx.manager.Do(http.MethodPatch, "/api/v1/columns/"+fx.statusCol.String(), keepWaiting)
	require.Equal(t, http.StatusOK, r.Status, r.Raw)
	assert.Equal(t, "Status kerja", r.Body["judul"])
	assert.Equal(t, "status", r.Body["tipe"])

	// Kolom terhapus menyembunyikan nilainya dari item.
	require.Equal(t, http.StatusNoContent, fx.manager.Do(http.MethodDelete, "/api/v1/columns/"+fx.statusCol.String(), nil).Status)
	r = fx.manager.Do(http.MethodGet, itemPath(id, ""), nil)
	assert.NotContains(t, r.Body["nilai"], fx.statusCol.String())
	assert.Equal(t, 1, fx.count("SELECT count(*) FROM item_values WHERE column_id = $1", fx.statusCol), "nilai tetap tersimpan")

	// Grup: tidak boleh dihapus bila berisi item atau bila grup terakhir.
	r = fx.manager.Do(http.MethodDelete, "/api/v1/groups/"+fx.groupID.String(), nil)
	assert.Equal(t, http.StatusConflict, r.Status)
	assert.Equal(t, "group_not_empty", r.Body["error"])
	require.Equal(t, http.StatusNoContent, fx.manager.Do(http.MethodDelete, itemPath(id, ""), nil).Status)
	r = fx.manager.Do(http.MethodDelete, "/api/v1/groups/"+fx.groupID.String(), nil)
	assert.Equal(t, http.StatusConflict, r.Status)
	assert.Equal(t, "last_group", r.Body["error"])

	g2 := fx.createGroup("Kosong")
	assert.Equal(t, http.StatusUnprocessableEntity, fx.manager.Do(http.MethodPatch, "/api/v1/groups/"+g2.String(),
		map[string]any{"warna": "#000"}).Status)
	r = fx.manager.Do(http.MethodPatch, "/api/v1/groups/"+g2.String(), map[string]any{"judul": "Arsip", "warna": "abu", "urutan": 1.5})
	require.Equal(t, http.StatusOK, r.Status, r.Raw)
	assert.Equal(t, http.StatusNoContent, fx.manager.Do(http.MethodDelete, "/api/v1/groups/"+g2.String(), nil).Status)
	assert.Equal(t, http.StatusUnprocessableEntity, fx.manager.Do(http.MethodPost, fx.boardPath("/items"),
		map[string]any{"group_id": g2, "judul": "X"}).Status, "grup terhapus tidak dapat diisi")
}

func TestArchivedProjectIsReadOnly(t *testing.T) {
	fx := newFixture(t)
	id := createItem(t, fx.manager, fx, fx.groupID, "Item")
	require.Equal(t, http.StatusOK, fx.manager.Do(http.MethodPost, "/api/v1/projects/"+fx.projectID.String()+"/archive", nil).Status)

	assert.Equal(t, http.StatusConflict, fx.manager.Do(http.MethodPost, fx.boardPath("/items"),
		map[string]any{"group_id": fx.groupID, "judul": "X"}).Status)
	assert.Equal(t, http.StatusConflict, fx.manager.Do(http.MethodPatch, itemPath(id, ""), map[string]any{"judul": "X"}).Status)
	assert.Equal(t, http.StatusConflict, fx.manager.Do(http.MethodPost, fx.boardPath("/groups"), map[string]any{"judul": "X"}).Status)
	assert.Equal(t, http.StatusOK, fx.manager.Do(http.MethodGet, itemPath(id, ""), nil).Status)
}

func TestItemPaginationAndDeletedBoard(t *testing.T) {
	fx := newFixture(t)
	want := map[string]bool{}
	for i := range 5 {
		id := createItem(t, fx.manager, fx, fx.groupID, "Item "+string(rune('A'+i)))
		want[id.String()] = true
	}
	got := map[string]bool{}
	cursor := ""
	pages := 0
	for {
		path := fx.boardPath("/items?batas=2")
		if cursor != "" {
			path += "&kursor=" + cursor
		}
		r := fx.manager.Do(http.MethodGet, path, nil)
		require.Equal(t, http.StatusOK, r.Status, r.Raw)
		for _, it := range r.Body["items"].([]any) {
			got[it.(map[string]any)["id"].(string)] = true
		}
		pages++
		next, ok := r.Body["kursor_berikut"].(string)
		if !ok {
			break
		}
		cursor = next
	}
	assert.Equal(t, want, got)
	assert.Equal(t, 3, pages)
	assert.Equal(t, http.StatusBadRequest, fx.manager.Do(http.MethodGet, fx.boardPath("/items?batas=nol"), nil).Status)

	require.Equal(t, http.StatusNoContent, fx.manager.Do(http.MethodDelete, fx.boardPath(""), nil).Status)
	assert.Equal(t, http.StatusNotFound, fx.manager.Do(http.MethodGet, fx.boardPath(""), nil).Status)
	assert.Equal(t, http.StatusNotFound, fx.manager.Do(http.MethodGet, fx.boardPath("/items"), nil).Status)
	assert.Empty(t, fx.manager.Do(http.MethodGet, "/api/v1/projects/"+fx.projectID.String()+"/boards", nil).Body["items"])
}
