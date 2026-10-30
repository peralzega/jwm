package user_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"jwm/internal/auth"
	"jwm/internal/config"
	"jwm/internal/db/gen"
	"jwm/internal/server"
	"jwm/internal/testutil"
	"jwm/internal/user"
)

const goodPassword = "kuda-lari-di-sawah-42"

type env struct {
	pool   *pgxpool.Pool
	hasher *auth.Hasher
	url    string
}

func newEnv(t *testing.T, adminCIDRs ...string) *env {
	t.Helper()
	pool := testutil.NewPool(t)
	hasher, err := auth.NewHasher(auth.Argon2Params{Memory: 1024, Time: 1, Threads: 1, SaltLen: 16, KeyLen: 32})
	require.NoError(t, err)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := config.Config{LoginRatePerMinute: 1000}
	for _, c := range adminCIDRs {
		cfg.AdminAllowedCIDRs = append(cfg.AdminAllowedCIDRs, netip.MustParsePrefix(c))
	}
	authSvc := auth.NewService(pool, hasher, auth.Settings{
		SessionIdleTTL: time.Hour, SessionAbsoluteTTL: time.Hour, MaxFailures: 3,
		LockoutBase: time.Minute, LockoutMax: time.Hour,
	}, log)
	srv := httptest.NewServer(server.NewRouter(server.Deps{
		Config: cfg, Pool: pool, Auth: authSvc, User: user.NewService(pool, hasher), Log: log,
	}))
	t.Cleanup(srv.Close)
	return &env{pool: pool, hasher: hasher, url: srv.URL}
}

// seedUser membuat pengguna langsung di database (sandi sudah diganti).
func (e *env) seedUser(t *testing.T, email string, peran auth.Role) dbgen.User {
	t.Helper()
	hash, err := e.hasher.Hash(goodPassword)
	require.NoError(t, err)
	u, err := dbgen.New(e.pool).CreateUser(context.Background(), dbgen.CreateUserParams{
		Nama: "Uji " + string(peran), Email: email, PasswordHash: hash, Peran: string(peran), WajibGantiSandi: false,
	})
	require.NoError(t, err)
	return u
}

func (e *env) loggedIn(t *testing.T, email string) *testutil.Client {
	t.Helper()
	c := testutil.NewClient(t, e.url)
	r := c.Login(email, goodPassword)
	require.Equal(t, http.StatusOK, r.Status, r.Raw)
	return c
}

func (e *env) count(t *testing.T, sql string, args ...any) int {
	t.Helper()
	var n int
	require.NoError(t, e.pool.QueryRow(context.Background(), sql, args...).Scan(&n))
	return n
}

func userPath(id uuid.UUID, suffix string) string {
	return "/api/v1/admin/users/" + id.String() + suffix
}

func TestOnlyAdminCanManageUsers(t *testing.T) {
	e := newEnv(t)
	target := e.seedUser(t, "target@jiep.co.id", auth.RoleViewer)
	for _, role := range []auth.Role{auth.RolePMO, auth.RoleProjectManager, auth.RoleMember, auth.RoleViewer} {
		t.Run(string(role), func(t *testing.T) {
			e.seedUser(t, string(role)+"@jiep.co.id", role)
			c := e.loggedIn(t, string(role)+"@jiep.co.id")
			calls := []struct{ method, path string }{
				{http.MethodGet, "/api/v1/admin/users"},
				{http.MethodPost, "/api/v1/admin/users"},
				{http.MethodGet, userPath(target.ID, "")},
				{http.MethodPatch, userPath(target.ID, "")},
				{http.MethodPost, userPath(target.ID, "/deactivate")},
				{http.MethodPost, userPath(target.ID, "/reset-password")},
				{http.MethodPost, userPath(target.ID, "/unlock")},
				{http.MethodGet, "/api/v1/admin/audit-log"},
			}
			for _, call := range calls {
				var body any
				if call.method == http.MethodPost && call.path == "/api/v1/admin/users" {
					body = map[string]string{"nama": "X", "email": "x@jiep.co.id", "peran": "admin"}
				}
				if call.method == http.MethodPatch {
					body = map[string]string{"peran": "admin"}
				}
				r := c.Do(call.method, call.path, body)
				assert.Equal(t, http.StatusForbidden, r.Status, "%s %s", call.method, call.path)
			}
		})
	}
	assert.Zero(t, e.count(t, "SELECT count(*) FROM users WHERE email = 'x@jiep.co.id'"))
	assert.Equal(t, "viewer", func() string {
		var p string
		require.NoError(t, e.pool.QueryRow(context.Background(), "SELECT peran FROM users WHERE id = $1", target.ID).Scan(&p))
		return p
	}())
}

func TestCreateUser(t *testing.T) {
	e := newEnv(t)
	e.seedUser(t, "admin@jiep.co.id", auth.RoleAdmin)
	admin := e.loggedIn(t, "admin@jiep.co.id")

	r := admin.Do(http.MethodPost, "/api/v1/admin/users", map[string]string{"nama": " Siti ", "email": "Siti@JIEP.co.id"})
	require.Equal(t, http.StatusCreated, r.Status, r.Raw)
	u := r.Body["user"].(map[string]any)
	assert.Equal(t, "viewer", u["peran"], "peran default Viewer")
	assert.Equal(t, "Siti", u["nama"])
	assert.Equal(t, "siti@jiep.co.id", u["email"])
	assert.Equal(t, true, u["wajib_ganti_sandi"])
	assert.NotContains(t, r.Raw, "password_hash")
	temp, _ := r.Body["sandi_sementara"].(string)
	require.Len(t, temp, 26)

	// Sandi sementara bisa dipakai login, tetapi wajib diganti dulu.
	c := testutil.NewClient(t, e.url)
	require.Equal(t, http.StatusOK, c.Login("siti@jiep.co.id", temp).Status)
	assert.Equal(t, http.StatusForbidden, c.Do(http.MethodGet, "/api/v1/users?q=si", nil).Status)

	tests := []struct {
		name string
		body map[string]string
		want int
	}{
		{"email duplikat beda huruf", map[string]string{"nama": "Siti 2", "email": "SITI@jiep.co.id"}, http.StatusConflict},
		{"email tidak valid", map[string]string{"nama": "A", "email": "bukan-email"}, http.StatusUnprocessableEntity},
		{"peran tidak dikenal", map[string]string{"nama": "A", "email": "a@jiep.co.id", "peran": "root"}, http.StatusUnprocessableEntity},
		{"nama kosong", map[string]string{"nama": "  ", "email": "b@jiep.co.id"}, http.StatusUnprocessableEntity},
		{"peran eksplisit", map[string]string{"nama": "PM", "email": "pm@jiep.co.id", "peran": "project_manager"}, http.StatusCreated},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := admin.Do(http.MethodPost, "/api/v1/admin/users", tt.body)
			assert.Equal(t, tt.want, r.Status, r.Raw)
		})
	}
	assert.Equal(t, 2, e.count(t, "SELECT count(*) FROM audit_log WHERE entitas = 'user' AND aksi = 'buat'"))
}

func TestChangeRoleIsAudited(t *testing.T) {
	e := newEnv(t)
	adminRow := e.seedUser(t, "admin@jiep.co.id", auth.RoleAdmin)
	m := e.seedUser(t, "member@jiep.co.id", auth.RoleMember)
	admin := e.loggedIn(t, "admin@jiep.co.id")

	r := admin.Do(http.MethodPatch, userPath(m.ID, ""), map[string]string{"peran": "pmo"})
	require.Equal(t, http.StatusOK, r.Status, r.Raw)
	assert.Equal(t, "pmo", r.Body["peran"])

	var aktor uuid.UUID
	var sebelum, sesudah string
	require.NoError(t, e.pool.QueryRow(context.Background(),
		"SELECT aktor_id, sebelum->>'peran', sesudah->>'peran' FROM audit_log WHERE aksi = 'ubah_peran' AND entitas_id = $1",
		m.ID.String()).Scan(&aktor, &sebelum, &sesudah))
	assert.Equal(t, adminRow.ID, aktor)
	assert.Equal(t, "member", sebelum)
	assert.Equal(t, "pmo", sesudah)

	r = admin.Do(http.MethodPatch, userPath(m.ID, ""), map[string]string{"nama": "Nama Baru"})
	require.Equal(t, http.StatusOK, r.Status)
	assert.Equal(t, 1, e.count(t, "SELECT count(*) FROM audit_log WHERE aksi = 'ubah_profil'"))
	assert.Equal(t, 1, e.count(t, "SELECT count(*) FROM audit_log WHERE aksi = 'ubah_peran'"), "ubah nama tidak mencatat ubah_peran")

	assert.Equal(t, http.StatusNotFound, admin.Do(http.MethodPatch, userPath(uuid.New(), ""), map[string]string{"nama": "x"}).Status)
	assert.Equal(t, http.StatusBadRequest, admin.Do(http.MethodPatch, "/api/v1/admin/users/bukan-uuid", map[string]string{"nama": "x"}).Status)
}

func TestLastActiveAdminProtected(t *testing.T) {
	e := newEnv(t)
	a1 := e.seedUser(t, "a1@jiep.co.id", auth.RoleAdmin)
	c1 := e.loggedIn(t, "a1@jiep.co.id")

	r := c1.Do(http.MethodPatch, userPath(a1.ID, ""), map[string]string{"peran": "pmo"})
	assert.Equal(t, http.StatusConflict, r.Status)
	assert.Equal(t, "last_admin", r.Body["error"])
	assert.Equal(t, http.StatusConflict, c1.Do(http.MethodPost, userPath(a1.ID, "/deactivate"), nil).Status,
		"menonaktifkan diri sendiri ditolak")

	// Admin kedua yang nonaktif tidak dihitung.
	a2 := e.seedUser(t, "a2@jiep.co.id", auth.RoleAdmin)
	require.Equal(t, http.StatusOK, c1.Do(http.MethodPost, userPath(a2.ID, "/deactivate"), nil).Status)
	assert.Equal(t, http.StatusConflict, c1.Do(http.MethodPatch, userPath(a1.ID, ""), map[string]string{"peran": "pmo"}).Status)

	// Setelah a2 aktif kembali, a1 boleh turun peran; a2 menjadi Admin terakhir.
	require.Equal(t, http.StatusOK, c1.Do(http.MethodPost, userPath(a2.ID, "/activate"), nil).Status)
	require.Equal(t, http.StatusOK, c1.Do(http.MethodPatch, userPath(a1.ID, ""), map[string]string{"peran": "pmo"}).Status)

	c2 := e.loggedIn(t, "a2@jiep.co.id")
	assert.Equal(t, http.StatusConflict, c2.Do(http.MethodPatch, userPath(a2.ID, ""), map[string]string{"peran": "viewer"}).Status)
	assert.Equal(t, 1, e.count(t, "SELECT count(*) FROM users WHERE peran = 'admin' AND aktif"))
}

func TestConcurrentDemotionKeepsOneAdmin(t *testing.T) {
	e := newEnv(t)
	a1 := e.seedUser(t, "a1@jiep.co.id", auth.RoleAdmin)
	a2 := e.seedUser(t, "a2@jiep.co.id", auth.RoleAdmin)
	c1, c2 := e.loggedIn(t, "a1@jiep.co.id"), e.loggedIn(t, "a2@jiep.co.id")

	var wg sync.WaitGroup
	statuses := make([]int, 2)
	for i, pair := range []struct {
		c  *testutil.Client
		id uuid.UUID
	}{{c1, a2.ID}, {c2, a1.ID}} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			statuses[i] = pair.c.Do(http.MethodPatch, userPath(pair.id, ""), map[string]string{"peran": "viewer"}).Status
		}()
	}
	wg.Wait()
	// Satu berhasil. Yang lain ditolak: 409 bila pelaku masih Admin saat dikunci,
	// 403 bila pelaku sudah diturunkan sebelum permintaannya diproses.
	slices.Sort(statuses)
	assert.Equal(t, http.StatusOK, statuses[0])
	assert.Contains(t, []int{http.StatusForbidden, http.StatusConflict}, statuses[1])
	assert.Equal(t, 1, e.count(t, "SELECT count(*) FROM users WHERE peran = 'admin' AND aktif"))
}

func TestDemotedAdminLosesAccessMidSession(t *testing.T) {
	e := newEnv(t)
	a1 := e.seedUser(t, "a1@jiep.co.id", auth.RoleAdmin)
	e.seedUser(t, "a2@jiep.co.id", auth.RoleAdmin)
	c1, c2 := e.loggedIn(t, "a1@jiep.co.id"), e.loggedIn(t, "a2@jiep.co.id")

	require.Equal(t, http.StatusOK, c2.Do(http.MethodPatch, userPath(a1.ID, ""), map[string]string{"peran": "member"}).Status)
	r := c1.Do(http.MethodPost, "/api/v1/admin/users", map[string]string{"nama": "X", "email": "x@jiep.co.id"})
	assert.Equal(t, http.StatusForbidden, r.Status)
	assert.Zero(t, e.count(t, "SELECT count(*) FROM users WHERE email = 'x@jiep.co.id'"))
}

// Uji langsung pada service: Principal basi (masih tercatat Admin) ditolak di dalam transaksi.
func TestStalePrincipalRejected(t *testing.T) {
	e := newEnv(t)
	a1 := e.seedUser(t, "a1@jiep.co.id", auth.RoleAdmin)
	e.seedUser(t, "a2@jiep.co.id", auth.RoleAdmin)
	m := e.seedUser(t, "m@jiep.co.id", auth.RoleMember)
	_, err := e.pool.Exec(context.Background(), "UPDATE users SET peran = 'member' WHERE id = $1", a1.ID)
	require.NoError(t, err)

	stale := auth.Principal{User: auth.User{ID: a1.ID, Peran: auth.RoleAdmin}}
	svc := user.NewService(e.pool, e.hasher)
	ctx := context.Background()
	_, err = svc.Create(ctx, stale, user.CreateInput{Nama: "X", Email: "x@jiep.co.id"}, auth.RequestMeta{})
	assert.ErrorIs(t, err, user.ErrForbidden)
	_, err = svc.ResetPassword(ctx, stale, m.ID, auth.RequestMeta{})
	assert.ErrorIs(t, err, user.ErrForbidden)
	_, err = svc.Unlock(ctx, stale, m.ID, auth.RequestMeta{})
	assert.ErrorIs(t, err, user.ErrForbidden)
	_, err = svc.SetActive(ctx, stale, m.ID, false, auth.RequestMeta{})
	assert.ErrorIs(t, err, user.ErrForbidden)
	r := auth.RoleAdmin
	_, err = svc.Update(ctx, stale, m.ID, user.UpdateInput{Peran: &r}, auth.RequestMeta{})
	assert.ErrorIs(t, err, user.ErrForbidden)
}

func TestDeactivateRevokesSessions(t *testing.T) {
	e := newEnv(t)
	e.seedUser(t, "admin@jiep.co.id", auth.RoleAdmin)
	m := e.seedUser(t, "member@jiep.co.id", auth.RoleMember)
	admin := e.loggedIn(t, "admin@jiep.co.id")
	member := e.loggedIn(t, "member@jiep.co.id")
	member2 := e.loggedIn(t, "member@jiep.co.id")

	r := admin.Do(http.MethodPost, userPath(m.ID, "/deactivate"), nil)
	require.Equal(t, http.StatusOK, r.Status, r.Raw)
	assert.Equal(t, false, r.Body["aktif"])

	assert.Equal(t, http.StatusUnauthorized, member.Do(http.MethodGet, "/api/v1/auth/me", nil).Status)
	assert.Equal(t, http.StatusUnauthorized, member2.Do(http.MethodGet, "/api/v1/auth/me", nil).Status)
	assert.Zero(t, e.count(t, "SELECT count(*) FROM sessions WHERE user_id = $1", m.ID))
	assert.Equal(t, http.StatusUnauthorized, testutil.NewClient(t, e.url).Login("member@jiep.co.id", goodPassword).Status)

	var revoked int
	require.NoError(t, e.pool.QueryRow(context.Background(),
		"SELECT (sesudah->>'sesi_dicabut')::int FROM audit_log WHERE aksi = 'nonaktifkan'").Scan(&revoked))
	assert.Equal(t, 2, revoked)

	require.Equal(t, http.StatusOK, admin.Do(http.MethodPost, userPath(m.ID, "/activate"), nil).Status)
	assert.Equal(t, http.StatusOK, testutil.NewClient(t, e.url).Login("member@jiep.co.id", goodPassword).Status)
}

func TestResetPassword(t *testing.T) {
	e := newEnv(t)
	adminRow := e.seedUser(t, "admin@jiep.co.id", auth.RoleAdmin)
	m := e.seedUser(t, "member@jiep.co.id", auth.RoleMember)
	admin := e.loggedIn(t, "admin@jiep.co.id")
	member := e.loggedIn(t, "member@jiep.co.id")

	// Kunci akun lewat 3 kali gagal.
	for range 3 {
		testutil.NewClient(t, e.url).Login("member@jiep.co.id", "salah-sandi-panjang")
	}
	require.Equal(t, 1, e.count(t, "SELECT count(*) FROM users WHERE id = $1 AND terkunci_hingga > now()", m.ID))

	r := admin.Do(http.MethodPost, userPath(m.ID, "/reset-password"), nil)
	require.Equal(t, http.StatusOK, r.Status, r.Raw)
	temp := r.Body["sandi_sementara"].(string)
	assert.Equal(t, true, r.Body["user"].(map[string]any)["wajib_ganti_sandi"])

	assert.Equal(t, http.StatusUnauthorized, member.Do(http.MethodGet, "/api/v1/auth/me", nil).Status, "sesi lama dicabut")
	assert.Equal(t, http.StatusUnauthorized, testutil.NewClient(t, e.url).Login("member@jiep.co.id", goodPassword).Status)
	c := testutil.NewClient(t, e.url)
	r = c.Login("member@jiep.co.id", temp)
	require.Equal(t, http.StatusOK, r.Status, "kunci dibuka dan sandi sementara berlaku")
	assert.Equal(t, true, r.Body["user"].(map[string]any)["wajib_ganti_sandi"])

	assert.Equal(t, http.StatusConflict, admin.Do(http.MethodPost, userPath(adminRow.ID, "/reset-password"), nil).Status,
		"Admin mengganti sandinya sendiri lewat /auth/password")
	assert.Equal(t, 1, e.count(t, "SELECT count(*) FROM audit_log WHERE aksi = 'reset_sandi'"))
	assert.Zero(t, e.count(t, "SELECT count(*) FROM audit_log WHERE sesudah::text ILIKE '%' || $1 || '%'", temp),
		"sandi sementara tidak boleh masuk audit")
}

func TestUnlock(t *testing.T) {
	e := newEnv(t)
	e.seedUser(t, "admin@jiep.co.id", auth.RoleAdmin)
	m := e.seedUser(t, "member@jiep.co.id", auth.RoleMember)
	admin := e.loggedIn(t, "admin@jiep.co.id")
	for range 3 {
		testutil.NewClient(t, e.url).Login("member@jiep.co.id", "salah-sandi-panjang")
	}
	require.Equal(t, http.StatusUnauthorized, testutil.NewClient(t, e.url).Login("member@jiep.co.id", goodPassword).Status)

	r := admin.Do(http.MethodPost, userPath(m.ID, "/unlock"), nil)
	require.Equal(t, http.StatusOK, r.Status, r.Raw)
	assert.Nil(t, r.Body["terkunci_hingga"])
	assert.Equal(t, http.StatusOK, testutil.NewClient(t, e.url).Login("member@jiep.co.id", goodPassword).Status)
	assert.Equal(t, 1, e.count(t, "SELECT count(*) FROM audit_log WHERE aksi = 'buka_kunci'"))
}

func TestListAndAuditLog(t *testing.T) {
	e := newEnv(t)
	e.seedUser(t, "admin@jiep.co.id", auth.RoleAdmin)
	m := e.seedUser(t, "member@jiep.co.id", auth.RoleMember)
	e.seedUser(t, "viewer_1@jiep.co.id", auth.RoleViewer)
	admin := e.loggedIn(t, "admin@jiep.co.id")

	items := func(r testutil.Resp) []any { return r.Body["items"].([]any) }
	assert.Len(t, items(admin.Do(http.MethodGet, "/api/v1/admin/users", nil)), 3)
	assert.Len(t, items(admin.Do(http.MethodGet, "/api/v1/admin/users?peran=member", nil)), 1)
	assert.Len(t, items(admin.Do(http.MethodGet, "/api/v1/admin/users?q=member", nil)), 1)
	assert.Len(t, items(admin.Do(http.MethodGet, "/api/v1/admin/users?q=_", nil)), 1, "wildcard LIKE diloloskan")
	assert.Equal(t, http.StatusUnprocessableEntity, admin.Do(http.MethodGet, "/api/v1/admin/users?peran=root", nil).Status)
	assert.Equal(t, http.StatusBadRequest, admin.Do(http.MethodGet, "/api/v1/admin/users?aktif=mungkin", nil).Status)

	for _, nama := range []string{"A", "B", "C"} {
		require.Equal(t, http.StatusOK, admin.Do(http.MethodPatch, userPath(m.ID, ""), map[string]string{"nama": nama}).Status)
	}
	r := admin.Do(http.MethodGet, "/api/v1/admin/audit-log?entitas=user&entitas_id="+m.ID.String()+"&batas=2", nil)
	require.Equal(t, http.StatusOK, r.Status, r.Raw)
	page1 := items(r)
	require.Len(t, page1, 2)
	cursor, ok := r.Body["kursor_berikut"].(float64)
	require.True(t, ok)
	assert.Equal(t, "C", page1[0].(map[string]any)["sesudah"].(map[string]any)["nama"], "terbaru lebih dulu")

	r = admin.Do(http.MethodGet, "/api/v1/admin/audit-log?entitas=user&entitas_id="+m.ID.String()+
		"&batas=2&kursor="+strconv.FormatInt(int64(cursor), 10), nil)
	require.Equal(t, http.StatusOK, r.Status)
	assert.Len(t, items(r), 1)
	assert.Nil(t, r.Body["kursor_berikut"])
}

func TestSearchUsers(t *testing.T) {
	e := newEnv(t)
	e.seedUser(t, "admin@jiep.co.id", auth.RoleAdmin)
	e.seedUser(t, "budi.member@jiep.co.id", auth.RoleMember)
	off := e.seedUser(t, "budi.off@jiep.co.id", auth.RoleMember)
	_, err := e.pool.Exec(context.Background(), "UPDATE users SET aktif = false WHERE id = $1", off.ID)
	require.NoError(t, err)

	e.seedUser(t, "viewer@jiep.co.id", auth.RoleViewer)
	c := e.loggedIn(t, "viewer@jiep.co.id")
	r := c.Do(http.MethodGet, "/api/v1/users?q=budi", nil)
	require.Equal(t, http.StatusOK, r.Status, r.Raw)
	list := r.Body["items"].([]any)
	require.Len(t, list, 1, "akun nonaktif tidak muncul")
	got := list[0].(map[string]any)
	assert.Equal(t, "budi.member@jiep.co.id", got["email"])
	assert.NotContains(t, got, "peran")
	assert.NotContains(t, got, "aktif")
	assert.Empty(t, c.Do(http.MethodGet, "/api/v1/users?q=b", nil).Body["items"], "minimal 2 karakter")
	assert.Equal(t, http.StatusUnauthorized, testutil.NewClient(t, e.url).Do(http.MethodGet, "/api/v1/users?q=budi", nil).Status)
}

func TestAdminCIDRRestriction(t *testing.T) {
	e := newEnv(t, "10.0.0.0/8")
	e.seedUser(t, "admin@jiep.co.id", auth.RoleAdmin)
	admin := e.loggedIn(t, "admin@jiep.co.id")

	r := admin.Do(http.MethodGet, "/api/v1/admin/users", nil)
	assert.Equal(t, http.StatusForbidden, r.Status)
	assert.Equal(t, "forbidden_network", r.Body["error"])
	assert.Equal(t, http.StatusOK, admin.Do(http.MethodGet, "/api/v1/auth/me", nil).Status, "rute non-admin tidak terpengaruh")
}
