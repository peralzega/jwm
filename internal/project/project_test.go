package project_test

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
		User: user.NewService(pool, hasher), Project: project.NewService(pool), Log: log,
	}))
	t.Cleanup(srv.Close)
	return &env{t: t, pool: pool, hasher: hasher, url: srv.URL}
}

func (e *env) seedUser(email string, peran auth.Role) dbgen.User {
	e.t.Helper()
	hash, err := e.hasher.Hash(goodPassword)
	require.NoError(e.t, err)
	u, err := dbgen.New(e.pool).CreateUser(context.Background(), dbgen.CreateUserParams{
		Nama: email, Email: email, PasswordHash: hash, Peran: string(peran), WajibGantiSandi: false,
	})
	require.NoError(e.t, err)
	return u
}

func (e *env) login(email string) *testutil.Client {
	e.t.Helper()
	c := testutil.NewClient(e.t, e.url)
	r := c.Login(email, goodPassword)
	require.Equal(e.t, http.StatusOK, r.Status, r.Raw)
	return c
}

// createProject membuat proyek lewat API dan mengembalikan ID-nya.
func createProject(t *testing.T, c *testutil.Client, nama string) uuid.UUID {
	t.Helper()
	r := c.Do(http.MethodPost, "/api/v1/projects", map[string]any{"nama": nama})
	require.Equal(t, http.StatusCreated, r.Status, r.Raw)
	return uuid.MustParse(r.Body["id"].(string))
}

// forceMember menambah anggota langsung di database, melewati validasi batas peran.
func (e *env) forceMember(projectID, userID uuid.UUID, role string) {
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

func projPath(id uuid.UUID, suffix string) string { return "/api/v1/projects/" + id.String() + suffix }

func memberPath(id, userID uuid.UUID) string { return projPath(id, "/members/"+userID.String()) }

func TestCreatePermissionByGlobalRole(t *testing.T) {
	e := newEnv(t)
	tests := []struct {
		role auth.Role
		want int
	}{
		{auth.RoleAdmin, http.StatusCreated},
		{auth.RolePMO, http.StatusCreated},
		{auth.RoleProjectManager, http.StatusCreated},
		{auth.RoleMember, http.StatusForbidden},
		{auth.RoleViewer, http.StatusForbidden},
	}
	for _, tt := range tests {
		t.Run(string(tt.role), func(t *testing.T) {
			u := e.seedUser(string(tt.role)+"@jiep.co.id", tt.role)
			c := e.login(u.Email)
			r := c.Do(http.MethodPost, "/api/v1/projects", map[string]any{"nama": "Proyek " + string(tt.role)})
			require.Equal(t, tt.want, r.Status, r.Raw)
			if tt.want != http.StatusCreated {
				return
			}
			assert.Equal(t, "manager", r.Body["peran_saya"])
			assert.Equal(t, u.ID.String(), r.Body["pemilik_id"])
			assert.Equal(t, "sedang", r.Body["prioritas"], "prioritas default")

			id := uuid.MustParse(r.Body["id"].(string))
			members := c.Do(http.MethodGet, projPath(id, "/members"), nil).Body["items"].([]any)
			require.Len(t, members, 1, "pembuat otomatis menjadi anggota")
			assert.Equal(t, "manager", members[0].(map[string]any)["peran_proyek"])
		})
	}
	assert.Equal(t, 3, e.count("SELECT count(*) FROM audit_log WHERE entitas = 'project' AND aksi = 'buat'"))
	assert.Equal(t, 3, e.count("SELECT count(*) FROM audit_log WHERE entitas = 'project_member' AND aksi = 'tambah'"))
}

func TestCreateValidation(t *testing.T) {
	e := newEnv(t)
	pm := e.seedUser("pm@jiep.co.id", auth.RoleProjectManager)
	pm2 := e.seedUser("pm2@jiep.co.id", auth.RoleProjectManager)
	mem := e.seedUser("mem@jiep.co.id", auth.RoleMember)
	c := e.login(pm.Email)

	tests := []struct {
		name string
		body map[string]any
		want int
	}{
		{"nama kosong", map[string]any{"nama": "  "}, http.StatusUnprocessableEntity},
		{"selesai sebelum mulai", map[string]any{"nama": "A", "mulai": "2026-05-10", "selesai": "2026-05-01"}, http.StatusUnprocessableEntity},
		{"format tanggal salah", map[string]any{"nama": "A", "mulai": "10/05/2026"}, http.StatusBadRequest},
		{"prioritas tak dikenal", map[string]any{"nama": "A", "prioritas": "darurat"}, http.StatusUnprocessableEntity},
		{"pemilik ber-peran Member", map[string]any{"nama": "A", "pemilik_id": mem.ID}, http.StatusUnprocessableEntity},
		{"pemilik tidak ada", map[string]any{"nama": "A", "pemilik_id": uuid.New()}, http.StatusUnprocessableEntity},
		{"field asing ditolak", map[string]any{"nama": "A", "anggaran": 100}, http.StatusBadRequest},
		{"valid lengkap", map[string]any{"nama": "B", "mulai": "2026-05-01", "selesai": "2026-05-01", "prioritas": "kritis"}, http.StatusCreated},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := c.Do(http.MethodPost, "/api/v1/projects", tt.body)
			assert.Equal(t, tt.want, r.Status, r.Raw)
		})
	}

	// Pemilik lain yang boleh menjadi manager ikut ditambahkan sebagai manager.
	r := c.Do(http.MethodPost, "/api/v1/projects", map[string]any{"nama": "C", "pemilik_id": pm2.ID})
	require.Equal(t, http.StatusCreated, r.Status, r.Raw)
	assert.Equal(t, pm2.ID.String(), r.Body["pemilik_id"])
	id := uuid.MustParse(r.Body["id"].(string))
	assert.Equal(t, 2, e.count("SELECT count(*) FROM project_members WHERE project_id = $1 AND peran_proyek = 'manager'", id))
}

func TestVisibility(t *testing.T) {
	e := newEnv(t)
	pmA := e.seedUser("pma@jiep.co.id", auth.RoleProjectManager)
	pmB := e.seedUser("pmb@jiep.co.id", auth.RoleProjectManager)
	viewer := e.seedUser("viewer@jiep.co.id", auth.RoleViewer)
	e.seedUser("admin@jiep.co.id", auth.RoleAdmin)
	e.seedUser("pmo@jiep.co.id", auth.RolePMO)

	a, b := e.login(pmA.Email), e.login(pmB.Email)
	p1, p2 := createProject(t, a, "P1"), createProject(t, b, "P2")
	e.forceMember(p1, viewer.ID, "viewer")

	names := func(c *testutil.Client) []string {
		var out []string
		for _, it := range c.Do(http.MethodGet, "/api/v1/projects", nil).Body["items"].([]any) {
			out = append(out, it.(map[string]any)["nama"].(string))
		}
		return out
	}
	assert.Equal(t, []string{"P1"}, names(a))
	assert.Equal(t, []string{"P2"}, names(b))
	assert.Equal(t, []string{"P1"}, names(e.login(viewer.Email)))
	assert.Equal(t, []string{"P1", "P2"}, names(e.login("admin@jiep.co.id")))
	assert.Equal(t, []string{"P1", "P2"}, names(e.login("pmo@jiep.co.id")))

	// Non-anggota mendapat 404, bukan 403, agar keberadaan proyek tidak bocor.
	assert.Equal(t, http.StatusNotFound, a.Do(http.MethodGet, projPath(p2, ""), nil).Status)
	assert.Equal(t, http.StatusNotFound, a.Do(http.MethodGet, projPath(p2, "/members"), nil).Status)
	assert.Equal(t, http.StatusNotFound, a.Do(http.MethodGet, projPath(uuid.New(), ""), nil).Status)
	assert.Equal(t, http.StatusBadRequest, a.Do(http.MethodGet, "/api/v1/projects/bukan-uuid", nil).Status)

	r := e.login("pmo@jiep.co.id").Do(http.MethodGet, projPath(p1, ""), nil)
	assert.Equal(t, "manager", r.Body["peran_saya"], "PMO mengelola semua proyek tanpa keanggotaan")
}

// Kriteria penerimaan M1: Member tanpa hak tidak dapat mengubah proyek lain.
func TestEditMatrix(t *testing.T) {
	e := newEnv(t)
	owner := e.seedUser("owner@jiep.co.id", auth.RoleProjectManager)
	p := createProject(t, e.login(owner.Email), "Target")

	other := e.seedUser("other@jiep.co.id", auth.RoleProjectManager)
	createProject(t, e.login(other.Email), "Proyek lain")

	tests := []struct {
		name        string
		global      auth.Role
		projectRole string // "" berarti bukan anggota
		wantPatch   int
		wantGet     int
	}{
		{"PM manager", auth.RoleProjectManager, "manager", http.StatusOK, http.StatusOK},
		{"PM anggota biasa", auth.RoleProjectManager, "member", http.StatusForbidden, http.StatusOK},
		{"PM viewer", auth.RoleProjectManager, "viewer", http.StatusForbidden, http.StatusOK},
		{"Member global sebagai member", auth.RoleMember, "member", http.StatusForbidden, http.StatusOK},
		{"Member global tercatat manager dibatasi", auth.RoleMember, "manager", http.StatusForbidden, http.StatusOK},
		{"Viewer global tercatat manager dibatasi", auth.RoleViewer, "manager", http.StatusForbidden, http.StatusOK},
		{"Member dari proyek lain", auth.RoleMember, "", http.StatusNotFound, http.StatusNotFound},
		{"PM dari proyek lain", auth.RoleProjectManager, "", http.StatusNotFound, http.StatusNotFound},
		{"Admin bukan anggota", auth.RoleAdmin, "", http.StatusOK, http.StatusOK},
		{"PMO bukan anggota", auth.RolePMO, "", http.StatusOK, http.StatusOK},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := e.seedUser(uuid.NewString()[:8]+"@jiep.co.id", tt.global)
			if tt.projectRole != "" {
				e.forceMember(p, u.ID, tt.projectRole)
			}
			c := e.login(u.Email)
			assert.Equal(t, tt.wantGet, c.Do(http.MethodGet, projPath(p, ""), nil).Status)

			r := c.Do(http.MethodPatch, projPath(p, ""), map[string]any{"nama": "Ubah " + string(rune('A'+i))})
			assert.Equal(t, tt.wantPatch, r.Status, r.Raw)

			if tt.wantPatch == http.StatusOK {
				return
			}
			// Tanpa hak manager, operasi pengelolaan lain juga ditolak.
			target := e.seedUser(uuid.NewString()[:8]+"@jiep.co.id", auth.RoleViewer)
			assert.Equal(t, tt.wantPatch, c.Do(http.MethodPost, projPath(p, "/members"),
				map[string]any{"user_id": target.ID, "peran_proyek": "viewer"}).Status)
			assert.Equal(t, tt.wantPatch, c.Do(http.MethodDelete, memberPath(p, owner.ID), nil).Status)
			assert.Equal(t, tt.wantPatch, c.Do(http.MethodPost, projPath(p, "/archive"), nil).Status)
		})
	}
	// Hanya 3 PATCH yang berhasil (PM manager, Admin, PMO).
	assert.Equal(t, 3, e.count("SELECT count(*) FROM audit_log WHERE entitas = 'project' AND aksi = 'ubah'"))
	assert.Equal(t, 1, e.count("SELECT count(*) FROM project_members WHERE project_id = $1 AND user_id = $2", p, owner.ID),
		"pemilik tidak terhapus oleh pengguna tanpa hak")
}

func TestGlobalDemotionCapsProjectRole(t *testing.T) {
	e := newEnv(t)
	owner := e.seedUser("owner@jiep.co.id", auth.RoleProjectManager)
	pm2 := e.seedUser("pm2@jiep.co.id", auth.RoleProjectManager)
	admin := e.seedUser("admin@jiep.co.id", auth.RoleAdmin)
	p := createProject(t, e.login(owner.Email), "P")
	o := e.login(owner.Email)
	require.Equal(t, http.StatusCreated, o.Do(http.MethodPost, projPath(p, "/members"),
		map[string]any{"user_id": pm2.ID, "peran_proyek": "manager"}).Status)

	c := e.login(pm2.Email)
	require.Equal(t, http.StatusOK, c.Do(http.MethodPatch, projPath(p, ""), map[string]any{"nama": "X"}).Status)

	// Admin menurunkan peran global pm2 menjadi Member; sesi pm2 tetap aktif.
	require.Equal(t, http.StatusOK, e.login(admin.Email).Do(http.MethodPatch,
		"/api/v1/admin/users/"+pm2.ID.String(), map[string]any{"peran": "member"}).Status)
	assert.Equal(t, http.StatusForbidden, c.Do(http.MethodPatch, projPath(p, ""), map[string]any{"nama": "Y"}).Status)

	members := o.Do(http.MethodGet, projPath(p, "/members"), nil).Body["items"].([]any)
	for _, m := range members {
		mm := m.(map[string]any)
		if mm["user_id"] == pm2.ID.String() {
			assert.Equal(t, "manager", mm["peran_proyek"])
			assert.Equal(t, "member", mm["peran_efektif"])
		}
	}
}

func TestMemberManagement(t *testing.T) {
	e := newEnv(t)
	owner := e.seedUser("owner@jiep.co.id", auth.RoleProjectManager)
	pm2 := e.seedUser("pm2@jiep.co.id", auth.RoleProjectManager)
	mem := e.seedUser("mem@jiep.co.id", auth.RoleMember)
	viewer := e.seedUser("viewer@jiep.co.id", auth.RoleViewer)
	off := e.seedUser("off@jiep.co.id", auth.RoleMember)
	_, err := e.pool.Exec(context.Background(), "UPDATE users SET aktif = false WHERE id = $1", off.ID)
	require.NoError(t, err)

	c := e.login(owner.Email)
	p := createProject(t, c, "P")
	add := func(userID uuid.UUID, role string) testutil.Resp {
		return c.Do(http.MethodPost, projPath(p, "/members"), map[string]any{"user_id": userID, "peran_proyek": role})
	}

	tests := []struct {
		name  string
		user  uuid.UUID
		role  string
		want  int
		error string
	}{
		{"Viewer global sebagai member", viewer.ID, "member", http.StatusUnprocessableEntity, "role_exceeds_global"},
		{"Member global sebagai manager", mem.ID, "manager", http.StatusUnprocessableEntity, "role_exceeds_global"},
		{"akun nonaktif", off.ID, "viewer", http.StatusUnprocessableEntity, "validation"},
		{"pengguna tidak ada", uuid.New(), "viewer", http.StatusUnprocessableEntity, "validation"},
		{"peran proyek tak dikenal", mem.ID, "owner", http.StatusUnprocessableEntity, "validation"},
		{"Viewer global sebagai viewer", viewer.ID, "viewer", http.StatusCreated, ""},
		{"Member global sebagai member", mem.ID, "member", http.StatusCreated, ""},
		{"duplikat", mem.ID, "viewer", http.StatusConflict, "already_member"},
		{"PM sebagai manager", pm2.ID, "manager", http.StatusCreated, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := add(tt.user, tt.role)
			assert.Equal(t, tt.want, r.Status, r.Raw)
			if tt.error != "" {
				assert.Equal(t, tt.error, r.Body["error"])
			}
		})
	}

	// Ubah peran, termasuk batas peran global.
	assert.Equal(t, http.StatusUnprocessableEntity, c.Do(http.MethodPatch, memberPath(p, mem.ID),
		map[string]any{"peran_proyek": "manager"}).Status)
	assert.Equal(t, http.StatusOK, c.Do(http.MethodPatch, memberPath(p, mem.ID),
		map[string]any{"peran_proyek": "viewer"}).Status)
	assert.Equal(t, http.StatusNotFound, c.Do(http.MethodPatch, memberPath(p, uuid.New()),
		map[string]any{"peran_proyek": "viewer"}).Status)

	// Member proyek (bukan manager) tidak dapat mengelola anggota.
	mc := e.login(mem.Email)
	assert.Equal(t, http.StatusForbidden, mc.Do(http.MethodDelete, memberPath(p, viewer.ID), nil).Status)

	// Pemilik harus tetap manager.
	r := c.Do(http.MethodPatch, memberPath(p, owner.ID), map[string]any{"peran_proyek": "member"})
	assert.Equal(t, http.StatusConflict, r.Status)
	assert.Equal(t, "owner_member", r.Body["error"])
	assert.Equal(t, http.StatusConflict, c.Do(http.MethodDelete, memberPath(p, owner.ID), nil).Status)

	// Setelah pemilik dipindah ke pm2, owner lama boleh keluar.
	require.Equal(t, http.StatusOK, c.Do(http.MethodPatch, projPath(p, ""), map[string]any{"pemilik_id": pm2.ID}).Status)
	assert.Equal(t, http.StatusUnprocessableEntity, c.Do(http.MethodPatch, projPath(p, ""),
		map[string]any{"pemilik_id": mem.ID}).Status, "pemilik harus anggota manager")
	assert.Equal(t, http.StatusNoContent, c.Do(http.MethodDelete, memberPath(p, owner.ID), nil).Status)
	assert.Equal(t, http.StatusNotFound, c.Do(http.MethodGet, projPath(p, ""), nil).Status, "mantan anggota kehilangan akses")

	assert.Equal(t, 1, e.count("SELECT count(*) FROM audit_log WHERE entitas = 'project_member' AND aksi = 'ubah_peran'"))
	assert.Equal(t, 1, e.count("SELECT count(*) FROM audit_log WHERE entitas = 'project_member' AND aksi = 'hapus'"))
}

func TestLastManagerProtected(t *testing.T) {
	e := newEnv(t)
	owner := e.seedUser("owner@jiep.co.id", auth.RoleProjectManager)
	admin := e.seedUser("admin@jiep.co.id", auth.RoleAdmin)
	p := createProject(t, e.login(owner.Email), "P")

	// Data tidak konsisten (pemilik bukan anggota): penjaga manager terakhir tetap berlaku.
	_, err := e.pool.Exec(context.Background(), "UPDATE projects SET pemilik_id = $2 WHERE id = $1", p, admin.ID)
	require.NoError(t, err)
	r := e.login(admin.Email).Do(http.MethodDelete, memberPath(p, owner.ID), nil)
	assert.Equal(t, http.StatusConflict, r.Status)
	assert.Equal(t, "last_manager", r.Body["error"])
}

func TestArchive(t *testing.T) {
	e := newEnv(t)
	owner := e.seedUser("owner@jiep.co.id", auth.RoleProjectManager)
	mem := e.seedUser("mem@jiep.co.id", auth.RoleMember)
	c := e.login(owner.Email)
	p := createProject(t, c, "P")
	e.forceMember(p, mem.ID, "member")

	assert.Equal(t, http.StatusForbidden, e.login(mem.Email).Do(http.MethodPost, projPath(p, "/archive"), nil).Status)
	r := c.Do(http.MethodPost, projPath(p, "/archive"), nil)
	require.Equal(t, http.StatusOK, r.Status, r.Raw)
	assert.NotNil(t, r.Body["diarsipkan_pada"])

	r = c.Do(http.MethodPatch, projPath(p, ""), map[string]any{"nama": "X"})
	assert.Equal(t, http.StatusConflict, r.Status)
	assert.Equal(t, "project_archived", r.Body["error"])
	assert.Equal(t, http.StatusConflict, c.Do(http.MethodPatch, memberPath(p, mem.ID), map[string]any{"peran_proyek": "viewer"}).Status)
	assert.Equal(t, http.StatusOK, c.Do(http.MethodGet, projPath(p, ""), nil).Status, "proyek arsip tetap dapat dibaca")

	assert.Empty(t, c.Do(http.MethodGet, "/api/v1/projects", nil).Body["items"])
	assert.Len(t, c.Do(http.MethodGet, "/api/v1/projects?arsip=true", nil).Body["items"], 1)

	require.Equal(t, http.StatusOK, c.Do(http.MethodPost, projPath(p, "/archive"), nil).Status, "idempoten")
	require.Equal(t, http.StatusOK, c.Do(http.MethodPost, projPath(p, "/unarchive"), nil).Status)
	assert.Equal(t, http.StatusOK, c.Do(http.MethodPatch, projPath(p, ""), map[string]any{"nama": "X"}).Status)
	assert.Equal(t, 1, e.count("SELECT count(*) FROM audit_log WHERE aksi = 'arsipkan'"))
	assert.Equal(t, 1, e.count("SELECT count(*) FROM audit_log WHERE aksi = 'pulihkan'"))
}

func TestUpdateFields(t *testing.T) {
	e := newEnv(t)
	owner := e.seedUser("owner@jiep.co.id", auth.RoleProjectManager)
	c := e.login(owner.Email)
	p := createProject(t, c, "P")

	r := c.Do(http.MethodPatch, projPath(p, ""), map[string]any{
		"mulai": "2026-11-02", "selesai": "2026-12-31", "prioritas": "tinggi",
	})
	require.Equal(t, http.StatusOK, r.Status, r.Raw)
	assert.Equal(t, "2026-11-02", r.Body["mulai"])
	assert.Equal(t, "tinggi", r.Body["prioritas"])
	assert.Equal(t, "P", r.Body["nama"], "field yang tidak dikirim tidak berubah")

	r = c.Do(http.MethodPatch, projPath(p, ""), map[string]any{"mulai": nil})
	require.Equal(t, http.StatusOK, r.Status, r.Raw)
	assert.Nil(t, r.Body["mulai"], "null menghapus tanggal")
	assert.Equal(t, "2026-12-31", r.Body["selesai"])

	tests := []struct {
		name string
		body map[string]any
		want int
	}{
		{"selesai sebelum mulai", map[string]any{"mulai": "2027-01-10"}, http.StatusUnprocessableEntity},
		{"nama null", map[string]any{"nama": nil}, http.StatusUnprocessableEntity},
		{"prioritas null", map[string]any{"prioritas": nil}, http.StatusUnprocessableEntity},
		{"pemilik null", map[string]any{"pemilik_id": nil}, http.StatusUnprocessableEntity},
		{"tanggal rusak", map[string]any{"selesai": "besok"}, http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, c.Do(http.MethodPatch, projPath(p, ""), tt.body).Status)
		})
	}

	var sebelum, sesudah string
	require.NoError(t, e.pool.QueryRow(context.Background(),
		"SELECT sebelum->>'prioritas', sesudah->>'prioritas' FROM audit_log WHERE entitas = 'project' AND aksi = 'ubah' ORDER BY id LIMIT 1").
		Scan(&sebelum, &sesudah))
	assert.Equal(t, "sedang", sebelum)
	assert.Equal(t, "tinggi", sesudah)
}
