package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"jwm/internal/db/gen"
	"jwm/internal/httpx"
	"jwm/internal/testutil"
)

type testEnv struct {
	pool   *pgxpool.Pool
	svc    *Service
	hasher *Hasher
	srv    *httptest.Server

	mu  sync.Mutex
	now time.Time
}

func (e *testEnv) advance(d time.Duration) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.now = e.now.Add(d)
}

func newTestEnv(t *testing.T, loginPerMinute int) *testEnv {
	t.Helper()
	pool := testutil.NewPool(t)
	hasher, err := NewHasher(testParams)
	require.NoError(t, err)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	e := &testEnv{pool: pool, hasher: hasher, now: time.Now()}
	e.svc = NewService(pool, hasher, Settings{
		SessionIdleTTL:     time.Hour,
		SessionAbsoluteTTL: 24 * time.Hour,
		MaxFailures:        3,
		LockoutBase:        time.Minute,
		LockoutMax:         4 * time.Minute,
	}, log)
	e.svc.now = func() time.Time {
		e.mu.Lock()
		defer e.mu.Unlock()
		return e.now
	}

	cookies := CookieConfig{Secure: false}
	r := chi.NewRouter()
	r.Use(httpx.WithClientIP(nil), httpx.CrossOrigin())
	r.Route("/api/v1", func(r chi.Router) {
		r.Route("/auth", NewHandler(e.svc, cookies, httpx.NewRateLimiter(loginPerMinute), log).Routes)
		r.Group(func(r chi.Router) {
			r.Use(RequireAuth(e.svc, cookies, log), RequirePasswordChanged)
			r.Get("/protected", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
			r.Post("/protected", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
		})
	})
	e.srv = httptest.NewServer(r)
	t.Cleanup(e.srv.Close)
	return e
}

func (e *testEnv) createUser(t *testing.T, email, password string, wajibGanti bool) dbgen.User {
	t.Helper()
	hash, err := e.hasher.Hash(password)
	require.NoError(t, err)
	u, err := dbgen.New(e.pool).CreateUser(context.Background(), dbgen.CreateUserParams{
		Nama: "Uji", Email: email, PasswordHash: hash, Peran: string(RoleMember), WajibGantiSandi: wajibGanti,
	})
	require.NoError(t, err)
	return u
}

type client struct {
	t    *testing.T
	base string
	http *http.Client
	csrf string
}

func (e *testEnv) client(t *testing.T) *client {
	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	return &client{t: t, base: e.srv.URL, http: &http.Client{Jar: jar}}
}

type resp struct {
	status int
	body   map[string]any
	raw    string
	header http.Header
}

func (c *client) do(method, path string, body any) resp {
	c.t.Helper()
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		require.NoError(c.t, err)
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.base+path, rdr)
	require.NoError(c.t, err)
	req.Header.Set("Content-Type", "application/json")
	if c.csrf != "" {
		req.Header.Set(CSRFHeader, c.csrf)
	}
	res, err := c.http.Do(req)
	require.NoError(c.t, err)
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	require.NoError(c.t, err)
	out := resp{status: res.StatusCode, raw: string(raw), header: res.Header}
	_ = json.Unmarshal(raw, &out.body)
	if tok, ok := out.body["csrf_token"].(string); ok {
		c.csrf = tok
	}
	return out
}

func (c *client) login(email, password string) resp {
	c.t.Helper()
	return c.do(http.MethodPost, "/api/v1/auth/login", map[string]string{"email": email, "password": password})
}

func auditActions(t *testing.T, pool *pgxpool.Pool, aksi string) int {
	t.Helper()
	var n int
	err := pool.QueryRow(context.Background(), "SELECT count(*) FROM audit_log WHERE aksi = $1", aksi).Scan(&n)
	require.NoError(t, err)
	return n
}

const goodPassword = "kuda-lari-di-sawah-42"

func TestLoginSuccess(t *testing.T) {
	e := newTestEnv(t, 100)
	u := e.createUser(t, "budi@jiep.co.id", goodPassword, false)

	var stored string
	require.NoError(t, e.pool.QueryRow(context.Background(),
		"SELECT password_hash FROM users WHERE id = $1", u.ID).Scan(&stored))
	assert.True(t, strings.HasPrefix(stored, "$argon2id$"), "hash disimpan dalam format argon2id")
	assert.NotContains(t, stored, goodPassword)

	c := e.client(t)
	r := c.login("  BUDI@jiep.co.id ", goodPassword)
	require.Equal(t, http.StatusOK, r.status, r.raw)
	assert.NotEmpty(t, c.csrf)
	assert.NotContains(t, r.raw, "password_hash")

	setCookie := r.header.Get("Set-Cookie")
	assert.Contains(t, setCookie, "HttpOnly")
	assert.Contains(t, setCookie, "SameSite=Lax")

	var tokenHashLen int
	require.NoError(t, e.pool.QueryRow(context.Background(),
		"SELECT length(token_hash) FROM sessions WHERE user_id = $1", u.ID).Scan(&tokenHashLen))
	assert.Equal(t, 32, tokenHashLen, "hanya hash SHA-256 token yang disimpan")

	assert.Equal(t, http.StatusOK, c.do(http.MethodGet, "/api/v1/auth/me", nil).status)
	assert.Equal(t, http.StatusOK, c.do(http.MethodGet, "/api/v1/protected", nil).status)
	assert.Equal(t, 1, auditActions(t, e.pool, "login_berhasil"))
}

func TestLoginFailureIsUniform(t *testing.T) {
	e := newTestEnv(t, 100)
	e.createUser(t, "ada@jiep.co.id", goodPassword, false)
	off := e.createUser(t, "off@jiep.co.id", goodPassword, false)
	_, err := e.pool.Exec(context.Background(), "UPDATE users SET aktif = false WHERE id = $1", off.ID)
	require.NoError(t, err)

	cases := map[string][2]string{
		"email tidak ada": {"tidakada@jiep.co.id", goodPassword},
		"sandi salah":     {"ada@jiep.co.id", "salah-sandi-panjang"},
		"akun nonaktif":   {"off@jiep.co.id", goodPassword},
		"kosong":          {"", ""},
	}
	var first *resp
	for name, cred := range cases {
		r := e.client(t).login(cred[0], cred[1])
		assert.Equal(t, http.StatusUnauthorized, r.status, name)
		if first == nil {
			first = &r
			continue
		}
		assert.Equal(t, first.raw, r.raw, "badan respons harus identik: %s", name)
	}
	assert.GreaterOrEqual(t, auditActions(t, e.pool, "login_gagal"), 3)
}

func TestLockoutAfterFailures(t *testing.T) {
	e := newTestEnv(t, 100)
	e.createUser(t, "kunci@jiep.co.id", goodPassword, false)
	c := e.client(t)

	for range 3 {
		require.Equal(t, http.StatusUnauthorized, c.login("kunci@jiep.co.id", "salah-sandi-panjang").status)
	}
	r := c.login("kunci@jiep.co.id", goodPassword)
	assert.Equal(t, http.StatusUnauthorized, r.status, "sandi benar tetap ditolak saat terkunci")

	var gagal int
	require.NoError(t, e.pool.QueryRow(context.Background(),
		"SELECT gagal_login FROM users WHERE email = 'kunci@jiep.co.id'").Scan(&gagal))
	assert.Equal(t, 3, gagal, "percobaan saat terkunci tidak menambah hitungan")

	e.advance(61 * time.Second)
	r = c.login("kunci@jiep.co.id", goodPassword)
	require.Equal(t, http.StatusOK, r.status, r.raw)
	require.NoError(t, e.pool.QueryRow(context.Background(),
		"SELECT gagal_login FROM users WHERE email = 'kunci@jiep.co.id'").Scan(&gagal))
	assert.Zero(t, gagal, "login berhasil mereset hitungan")
}

func TestLockoutIsProgressive(t *testing.T) {
	e := newTestEnv(t, 100)
	e.createUser(t, "prog@jiep.co.id", goodPassword, false)
	c := e.client(t)

	for range 3 {
		c.login("prog@jiep.co.id", "salah-sandi-panjang")
	}
	e.advance(61 * time.Second)
	for range 3 {
		c.login("prog@jiep.co.id", "salah-sandi-panjang")
	}
	// Kunci kedua 2 menit: setelah 61 detik masih terkunci.
	e.advance(61 * time.Second)
	assert.Equal(t, http.StatusUnauthorized, c.login("prog@jiep.co.id", goodPassword).status)
	e.advance(60 * time.Second)
	assert.Equal(t, http.StatusOK, c.login("prog@jiep.co.id", goodPassword).status)
}

func TestLoginRateLimitPerIP(t *testing.T) {
	e := newTestEnv(t, 2)
	c := e.client(t)
	assert.Equal(t, http.StatusUnauthorized, c.login("x@jiep.co.id", "salah-sandi-panjang").status)
	assert.Equal(t, http.StatusUnauthorized, c.login("y@jiep.co.id", "salah-sandi-panjang").status)
	r := c.login("z@jiep.co.id", "salah-sandi-panjang")
	assert.Equal(t, http.StatusTooManyRequests, r.status)
	assert.NotEmpty(t, r.header.Get("Retry-After"))
}

func TestMustChangePasswordOnFirstLogin(t *testing.T) {
	e := newTestEnv(t, 100)
	e.createUser(t, "baru@jiep.co.id", "sandi-sementara-001", true)
	c := e.client(t)

	r := c.login("baru@jiep.co.id", "sandi-sementara-001")
	require.Equal(t, http.StatusOK, r.status)
	assert.Equal(t, true, r.body["user"].(map[string]any)["wajib_ganti_sandi"])

	r = c.do(http.MethodGet, "/api/v1/protected", nil)
	assert.Equal(t, http.StatusForbidden, r.status)
	assert.Equal(t, "password_change_required", r.body["error"])
	assert.Equal(t, http.StatusOK, c.do(http.MethodGet, "/api/v1/auth/me", nil).status)

	change := map[string]string{"sandi_lama": "sandi-sementara-001", "sandi_baru": "sandi-sementara-001"}
	assert.Equal(t, http.StatusUnprocessableEntity, c.do(http.MethodPost, "/api/v1/auth/password", change).status)
	change["sandi_baru"] = "password1234"
	assert.Equal(t, http.StatusUnprocessableEntity, c.do(http.MethodPost, "/api/v1/auth/password", change).status)
	change["sandi_lama"], change["sandi_baru"] = "salah-sandi-panjang", goodPassword
	assert.Equal(t, http.StatusForbidden, c.do(http.MethodPost, "/api/v1/auth/password", change).status)

	change["sandi_lama"] = "sandi-sementara-001"
	r = c.do(http.MethodPost, "/api/v1/auth/password", change)
	require.Equal(t, http.StatusOK, r.status, r.raw)
	assert.Equal(t, false, r.body["user"].(map[string]any)["wajib_ganti_sandi"])
	assert.Equal(t, http.StatusOK, c.do(http.MethodGet, "/api/v1/protected", nil).status)

	assert.Equal(t, http.StatusUnauthorized, e.client(t).login("baru@jiep.co.id", "sandi-sementara-001").status)
	assert.Equal(t, http.StatusOK, e.client(t).login("baru@jiep.co.id", goodPassword).status)
}

func TestPreferences(t *testing.T) {
	e := newTestEnv(t, 100)
	e.createUser(t, "pref@jiep.co.id", goodPassword, false)
	c := e.client(t)
	r := c.login("pref@jiep.co.id", goodPassword)
	require.Equal(t, http.StatusOK, r.status)
	assert.Equal(t, "compact", r.body["user"].(map[string]any)["kepadatan"], "default compact")

	const path = "/api/v1/auth/me/preferensi"
	tests := []struct {
		name   string
		body   any
		status int
		code   string
		want   string // kepadatan di /auth/me setelah permintaan
	}{
		{"ubah ke normal", map[string]any{"kepadatan": "normal"}, http.StatusOK, "", "normal"},
		{"badan kosong tidak mengubah apa pun", map[string]any{}, http.StatusOK, "", "normal"},
		{"kembali ke compact", map[string]any{"kepadatan": "compact"}, http.StatusOK, "", "compact"},
		{"nilai tak dikenal", map[string]any{"kepadatan": "lebar"}, http.StatusUnprocessableEntity, "validation", "compact"},
		{"bidang tak dikenal", map[string]any{"peran": "admin"}, http.StatusBadRequest, "bad_request", "compact"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := c.do(http.MethodPatch, path, tt.body)
			require.Equal(t, tt.status, r.status, r.raw)
			if tt.code != "" {
				assert.Equal(t, tt.code, r.body["error"])
			} else {
				assert.Equal(t, tt.want, r.body["user"].(map[string]any)["kepadatan"])
			}
			me := c.do(http.MethodGet, "/api/v1/auth/me", nil)
			assert.Equal(t, tt.want, me.body["user"].(map[string]any)["kepadatan"])
		})
	}

	t.Run("tanpa CSRF ditolak", func(t *testing.T) {
		token := c.csrf
		c.csrf = ""
		defer func() { c.csrf = token }()
		assert.Equal(t, http.StatusForbidden, c.do(http.MethodPatch, path, map[string]any{"kepadatan": "normal"}).status)
	})

	t.Run("tanpa sesi ditolak", func(t *testing.T) {
		assert.Equal(t, http.StatusUnauthorized, e.client(t).do(http.MethodPatch, path, map[string]any{"kepadatan": "normal"}).status)
	})

	t.Run("wajib ganti sandi ditolak", func(t *testing.T) {
		e.createUser(t, "pref-baru@jiep.co.id", "sandi-sementara-002", true)
		nc := e.client(t)
		require.Equal(t, http.StatusOK, nc.login("pref-baru@jiep.co.id", "sandi-sementara-002").status)
		r := nc.do(http.MethodPatch, path, map[string]any{"kepadatan": "normal"})
		assert.Equal(t, http.StatusForbidden, r.status)
		assert.Equal(t, "password_change_required", r.body["error"])
	})
}

func TestCSRFRequiredForMutations(t *testing.T) {
	e := newTestEnv(t, 100)
	e.createUser(t, "csrf@jiep.co.id", goodPassword, false)
	c := e.client(t)
	require.Equal(t, http.StatusOK, c.login("csrf@jiep.co.id", goodPassword).status)
	token := c.csrf

	c.csrf = ""
	r := c.do(http.MethodPost, "/api/v1/protected", nil)
	assert.Equal(t, http.StatusForbidden, r.status)
	assert.Equal(t, "csrf", r.body["error"])

	c.csrf = "token-palsu"
	assert.Equal(t, http.StatusForbidden, c.do(http.MethodPost, "/api/v1/auth/logout", nil).status)

	c.csrf = token
	assert.Equal(t, http.StatusOK, c.do(http.MethodPost, "/api/v1/protected", nil).status)
}

func TestChangePasswordRevokesOtherSessions(t *testing.T) {
	e := newTestEnv(t, 100)
	e.createUser(t, "dua@jiep.co.id", goodPassword, false)
	a, b := e.client(t), e.client(t)
	require.Equal(t, http.StatusOK, a.login("dua@jiep.co.id", goodPassword).status)
	require.Equal(t, http.StatusOK, b.login("dua@jiep.co.id", goodPassword).status)

	r := a.do(http.MethodPost, "/api/v1/auth/password",
		map[string]string{"sandi_lama": goodPassword, "sandi_baru": "sandi-baru-yang-kuat"})
	require.Equal(t, http.StatusOK, r.status, r.raw)

	assert.Equal(t, http.StatusUnauthorized, b.do(http.MethodGet, "/api/v1/auth/me", nil).status, "sesi lain dicabut")
	assert.Equal(t, http.StatusOK, a.do(http.MethodGet, "/api/v1/auth/me", nil).status, "perangkat saat ini dapat sesi baru")

	var n int
	require.NoError(t, e.pool.QueryRow(context.Background(),
		"SELECT count(*) FROM sessions s JOIN users u ON u.id = s.user_id WHERE u.email = 'dua@jiep.co.id'").Scan(&n))
	assert.Equal(t, 1, n)
	assert.Equal(t, 1, auditActions(t, e.pool, "ganti_sandi"))
}

func TestDeactivationRevokesSessions(t *testing.T) {
	e := newTestEnv(t, 100)
	u := e.createUser(t, "off@jiep.co.id", goodPassword, false)
	c := e.client(t)
	require.Equal(t, http.StatusOK, c.login("off@jiep.co.id", goodPassword).status)

	ctx := context.Background()
	_, err := e.pool.Exec(ctx, "UPDATE users SET aktif = false WHERE id = $1", u.ID)
	require.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, c.do(http.MethodGet, "/api/v1/auth/me", nil).status,
		"akun nonaktif ditolak walau baris sesi masih ada")

	n, err := RevokeAllSessions(ctx, dbgen.New(e.pool), u.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(1), n)
}

func TestLogout(t *testing.T) {
	e := newTestEnv(t, 100)
	e.createUser(t, "keluar@jiep.co.id", goodPassword, false)
	c := e.client(t)
	require.Equal(t, http.StatusOK, c.login("keluar@jiep.co.id", goodPassword).status)
	assert.Equal(t, http.StatusNoContent, c.do(http.MethodPost, "/api/v1/auth/logout", nil).status)
	assert.Equal(t, http.StatusUnauthorized, c.do(http.MethodGet, "/api/v1/auth/me", nil).status)
	assert.Equal(t, 1, auditActions(t, e.pool, "logout"))
}

func TestSessionExpiry(t *testing.T) {
	tests := []struct {
		name   string
		column string
	}{
		{"idle", "kedaluwarsa"},
		{"mutlak", "kedaluwarsa_mutlak"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newTestEnv(t, 100)
			u := e.createUser(t, "exp@jiep.co.id", goodPassword, false)
			c := e.client(t)
			require.Equal(t, http.StatusOK, c.login("exp@jiep.co.id", goodPassword).status)

			_, err := e.pool.Exec(context.Background(),
				"UPDATE sessions SET "+tt.column+" = now() - interval '1 second' WHERE user_id = $1", u.ID)
			require.NoError(t, err)
			assert.Equal(t, http.StatusUnauthorized, c.do(http.MethodGet, "/api/v1/auth/me", nil).status)
		})
	}
}

func TestBootstrapAdmin(t *testing.T) {
	pool := testutil.NewPool(t)
	h, err := NewHasher(testParams)
	require.NoError(t, err)
	ctx := context.Background()

	_, err = BootstrapAdmin(ctx, pool, h, "Admin", "admin@jiep.co.id", "pendek")
	assert.ErrorIs(t, err, ErrPasswordTooShort)
	_, err = BootstrapAdmin(ctx, pool, h, "Admin", "bukan-email", goodPassword)
	assert.ErrorIs(t, err, ErrInvalidEmail)

	u, err := BootstrapAdmin(ctx, pool, h, "Admin", "Admin@JIEP.co.id", goodPassword)
	require.NoError(t, err)
	assert.Equal(t, RoleAdmin, u.Peran)
	assert.True(t, u.WajibGantiSandi)
	assert.Equal(t, "admin@jiep.co.id", u.Email)

	_, err = BootstrapAdmin(ctx, pool, h, "Admin 2", "admin2@jiep.co.id", goodPassword)
	assert.ErrorIs(t, err, ErrAdminExists)
	assert.Equal(t, 1, auditActions(t, pool, "bootstrap_admin"))
}
