package realtime_test

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
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
	"jwm/internal/realtime"
	"jwm/internal/schedule"
	"jwm/internal/server"
	"jwm/internal/testutil"
	"jwm/internal/user"
)

const goodPassword = "kuda-lari-di-sawah-42"

type fixture struct {
	t      *testing.T
	pool   *pgxpool.Pool
	hasher *auth.Hasher
	url    string
	hub    *realtime.Hub
	pm     *testutil.Client
}

func newFixture(t *testing.T, keepAlive, maxAge time.Duration) *fixture {
	t.Helper()
	pool := testutil.NewPool(t)
	hasher, err := auth.NewHasher(auth.Argon2Params{Memory: 1024, Time: 1, Threads: 1, SaltLen: 16, KeyLen: 32})
	require.NoError(t, err)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	authSvc := auth.NewService(pool, hasher, auth.Settings{
		SessionIdleTTL: time.Hour, SessionAbsoluteTTL: time.Hour, MaxFailures: 5,
		LockoutBase: time.Minute, LockoutMax: time.Hour,
	}, log)
	hub := realtime.NewHub(3)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go realtime.Listen(ctx, pool, hub, log)
	srv := httptest.NewServer(server.NewRouter(server.Deps{
		Config: config.Config{LoginRatePerMinute: 1000}, Pool: pool, Auth: authSvc,
		User: user.NewService(pool, hasher), Project: project.NewService(pool), Board: board.NewService(pool),
		Schedule: schedule.NewService(pool), Realtime: hub, StreamKeepAlive: keepAlive, StreamMaxAge: maxAge, Log: log,
	}))
	t.Cleanup(srv.Close)
	fx := &fixture{t: t, pool: pool, hasher: hasher, url: srv.URL, hub: hub}
	fx.pm = fx.login(fx.seedUser(auth.RoleProjectManager))
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

// projectWithItem membuat proyek, board, dan satu item; mengembalikan id proyek,
// id item, dan id kolom status.
func (fx *fixture) projectWithItem(nama string) (string, string, string) {
	fx.t.Helper()
	p := fx.must(fx.pm, http.MethodPost, "/api/v1/projects", map[string]any{"nama": nama}, http.StatusCreated).Body["id"].(string)
	b := fx.must(fx.pm, http.MethodPost, "/api/v1/projects/"+p+"/boards", map[string]any{"nama": "B"}, http.StatusCreated)
	var status string
	for _, c := range b.Body["kolom"].([]any) {
		if m := c.(map[string]any); m["tipe"] == "status" {
			status = m["id"].(string)
		}
	}
	group := b.Body["grup"].([]any)[0].(map[string]any)["id"].(string)
	item := fx.must(fx.pm, http.MethodPost, "/api/v1/boards/"+b.Body["id"].(string)+"/items",
		map[string]any{"group_id": group, "judul": "Item"}, http.StatusCreated).Body["id"].(string)
	return p, item, status
}

// sse membaca event dari stream di goroutine terpisah.
type sse struct {
	events chan string // "nama|data", komentar diawali ":"
	done   chan struct{}
	cancel context.CancelFunc
}

func openStream(t *testing.T, c *testutil.Client, path string) (*http.Response, *sse) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	res := c.Stream(ctx, path)
	s := &sse{events: make(chan string, 64), done: make(chan struct{}), cancel: cancel}
	t.Cleanup(func() { cancel(); _ = res.Body.Close() })
	if res.StatusCode != http.StatusOK {
		close(s.done)
		return res, s
	}
	go func() {
		defer close(s.done)
		sc := bufio.NewScanner(res.Body)
		var name, data string
		for sc.Scan() {
			line := sc.Text()
			switch {
			case strings.HasPrefix(line, ":"):
				s.events <- line
			case strings.HasPrefix(line, "event: "):
				name = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				data = strings.TrimPrefix(line, "data: ")
			case line == "" && name != "":
				s.events <- name + "|" + data
				name, data = "", ""
			}
		}
	}()
	return res, s
}

// next menunggu event bernama (melewati komentar) dalam batas waktu.
func (s *sse) next(t *testing.T, name string, within time.Duration) string {
	t.Helper()
	timeout := time.After(within)
	for {
		select {
		case e := <-s.events:
			if n, data, ok := strings.Cut(e, "|"); ok && n == name {
				return data
			}
		case <-s.done:
			t.Fatalf("stream tertutup sebelum event %q", name)
		case <-timeout:
			t.Fatalf("event %q tidak datang dalam %v", name, within)
		}
	}
}

func TestStreamDeliversProjectChanges(t *testing.T) {
	fx := newFixture(t, time.Hour, time.Hour)
	p, item, status := fx.projectWithItem("Alpha")
	q, itemQ, statusQ := fx.projectWithItem("Beta")

	res, s := openStream(t, fx.pm, "/api/v1/stream?project="+p)
	require.Equal(t, http.StatusOK, res.StatusCode)
	assert.Equal(t, "text/event-stream", res.Header.Get("Content-Type"))
	assert.Equal(t, "no", res.Header.Get("X-Accel-Buffering"))
	require.Equal(t, ": tersambung", <-s.events)

	// Perubahan proyek lain tidak dikirim; perubahan proyek ini dikirim.
	fx.must(fx.pm, http.MethodPut, "/api/v1/items/"+itemQ+"/values/"+statusQ, map[string]any{"nilai": "done"}, http.StatusOK)
	fx.must(fx.pm, http.MethodPut, "/api/v1/items/"+item+"/values/"+status, map[string]any{"nilai": "done"}, http.StatusOK)
	var e realtime.Event
	require.NoError(t, json.Unmarshal([]byte(s.next(t, "berubah", 5*time.Second)), &e))
	assert.Equal(t, realtime.Event{ProjectID: uuid.MustParse(p), Jenis: "item"}, e)

	fx.must(fx.pm, http.MethodPost, "/api/v1/projects/"+p+"/budget",
		map[string]any{"tanggal": "2026-11-02", "jenis": "anggaran", "jumlah": "10", "keterangan": ""}, http.StatusCreated)
	require.NoError(t, json.Unmarshal([]byte(s.next(t, "berubah", 5*time.Second)), &e))
	assert.Equal(t, "anggaran", e.Jenis)
	assert.NotEqual(t, q, e.ProjectID.String())
}

func TestStreamAllProjectsAndAccess(t *testing.T) {
	fx := newFixture(t, time.Hour, time.Hour)
	p, item, status := fx.projectWithItem("Alpha")

	// Tanpa parameter: semua proyek yang terlihat.
	_, s := openStream(t, fx.pm, "/api/v1/stream")
	require.Equal(t, ": tersambung", <-s.events)
	fx.must(fx.pm, http.MethodPut, "/api/v1/items/"+item+"/values/"+status, map[string]any{"nilai": "waiting"}, http.StatusOK)
	assert.Contains(t, s.next(t, "berubah", 5*time.Second), p)

	outsider := fx.login(fx.seedUser(auth.RoleMember))
	res, _ := openStream(t, outsider, "/api/v1/stream?project="+p)
	assert.Equal(t, http.StatusNotFound, res.StatusCode)
	res, _ = openStream(t, fx.pm, "/api/v1/stream?project=bukan-uuid")
	assert.Equal(t, http.StatusBadRequest, res.StatusCode)
	anon := testutil.NewClient(t, fx.url)
	res, _ = openStream(t, anon, "/api/v1/stream")
	assert.Equal(t, http.StatusUnauthorized, res.StatusCode)

	// Batas 3 koneksi per pengguna (sudah 1 terbuka).
	for range 2 {
		res, _ := openStream(t, fx.pm, "/api/v1/stream")
		require.Equal(t, http.StatusOK, res.StatusCode)
	}
	res, _ = openStream(t, fx.pm, "/api/v1/stream")
	assert.Equal(t, http.StatusTooManyRequests, res.StatusCode)
}

func TestStreamClosesOnLogoutMaxAgeAndShutdown(t *testing.T) {
	fx := newFixture(t, 150*time.Millisecond, 3*time.Second)
	p, _, _ := fx.projectWithItem("Alpha")

	// Sesi dicabut (logout): stream ditutup pada pemeriksaan berikutnya.
	other := fx.login(fx.seedUser(auth.RolePMO))
	_, s := openStream(t, other, "/api/v1/stream?project="+p)
	require.Equal(t, ": tersambung", <-s.events)
	fx.must(other, http.MethodPost, "/api/v1/auth/logout", nil, http.StatusNoContent)
	select {
	case <-s.done:
	case <-time.After(3 * time.Second):
		t.Fatal("stream tidak ditutup setelah logout")
	}

	// Umur maksimal: server mengirim "tutup" lalu menutup.
	_, s = openStream(t, fx.pm, "/api/v1/stream?project="+p)
	assert.Equal(t, "{}", s.next(t, "tutup", 5*time.Second))
	<-s.done

	// Shutdown: semua stream ditutup segera.
	var wg sync.WaitGroup
	for range 2 {
		_, s := openStream(t, fx.pm, "/api/v1/stream")
		require.Equal(t, ": tersambung", <-s.events)
		wg.Add(1)
		go func() { defer wg.Done(); <-s.done }()
	}
	fx.hub.Shutdown()
	waited := make(chan struct{})
	go func() { wg.Wait(); close(waited) }()
	select {
	case <-waited:
	case <-time.After(2 * time.Second):
		t.Fatal("stream tidak ditutup saat shutdown")
	}
	assert.Eventually(t, func() bool { return fx.hub.Count() == 0 }, time.Second, 20*time.Millisecond)
}
