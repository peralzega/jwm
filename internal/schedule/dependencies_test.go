package schedule_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
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
	"jwm/internal/schedule"
	"jwm/internal/server"
	"jwm/internal/testutil"
	"jwm/internal/user"
)

const goodPassword = "kuda-lari-di-sawah-42"

// fixture: satu proyek milik manager dengan dua board.
type fixture struct {
	t        *testing.T
	pool     *pgxpool.Pool
	hasher   *auth.Hasher
	url      string
	manager  *testutil.Client
	project  string
	board    string
	group    string
	board2   string
	group2   string
	personCo string
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
		Schedule: schedule.NewService(pool), Log: log,
	}))
	t.Cleanup(srv.Close)
	fx := &fixture{t: t, pool: pool, hasher: hasher, url: srv.URL}

	fx.manager = fx.login(fx.seedUser(auth.RoleProjectManager))
	r := fx.manager.Do(http.MethodPost, "/api/v1/projects", map[string]any{"nama": "Proyek"})
	require.Equal(t, http.StatusCreated, r.Status, r.Raw)
	fx.project = r.Body["id"].(string)
	fx.board, fx.group, fx.personCo = fx.createBoard("Board utama")
	fx.board2, fx.group2, _ = fx.createBoard("Board kedua")
	return fx
}

func (fx *fixture) createBoard(nama string) (boardID, groupID, personCol string) {
	fx.t.Helper()
	r := fx.manager.Do(http.MethodPost, "/api/v1/projects/"+fx.project+"/boards", map[string]any{"nama": nama})
	require.Equal(fx.t, http.StatusCreated, r.Status, r.Raw)
	for _, c := range r.Body["kolom"].([]any) {
		if m := c.(map[string]any); m["tipe"] == "person" {
			personCol = m["id"].(string)
		}
	}
	return r.Body["id"].(string), r.Body["grup"].([]any)[0].(map[string]any)["id"].(string), personCol
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

func (fx *fixture) addMember(userID uuid.UUID, role string) {
	fx.t.Helper()
	_, err := fx.pool.Exec(context.Background(),
		"INSERT INTO project_members (project_id, user_id, peran_proyek) VALUES ($1, $2, $3)", fx.project, userID, role)
	require.NoError(fx.t, err)
}

func (fx *fixture) item(c *testutil.Client, board, group, judul string) string {
	fx.t.Helper()
	r := c.Do(http.MethodPost, "/api/v1/boards/"+board+"/items", map[string]any{"group_id": group, "judul": judul})
	require.Equal(fx.t, http.StatusCreated, r.Status, r.Raw)
	return r.Body["id"].(string)
}

func depsPath(board string) string { return "/api/v1/boards/" + board + "/dependencies" }

func link(c *testutil.Client, board, pred, succ string) testutil.Resp {
	return c.Do(http.MethodPost, depsPath(board), map[string]any{"predecessor_id": pred, "successor_id": succ})
}

func listPairs(t *testing.T, c *testutil.Client, board string) [][2]string {
	t.Helper()
	r := c.Do(http.MethodGet, depsPath(board), nil)
	require.Equal(t, http.StatusOK, r.Status, r.Raw)
	var out [][2]string
	for _, d := range r.Body["items"].([]any) {
		m := d.(map[string]any)
		out = append(out, [2]string{m["predecessor_id"].(string), m["successor_id"].(string)})
	}
	return out
}

func TestDependencyLifecycleAndAudit(t *testing.T) {
	fx := newFixture(t)
	a := fx.item(fx.manager, fx.board, fx.group, "A")
	b := fx.item(fx.manager, fx.board, fx.group, "B")

	r := link(fx.manager, fx.board, a, b)
	require.Equal(t, http.StatusCreated, r.Status, r.Raw)
	assert.Equal(t, "FS", r.Body["tipe"], "tipe bawaan FS")
	assert.EqualValues(t, 0, r.Body["lag_hari"])
	id := r.Body["id"].(string)
	assert.Equal(t, [][2]string{{a, b}}, listPairs(t, fx.manager, fx.board))

	require.Equal(t, http.StatusNoContent, fx.manager.Do(http.MethodDelete, "/api/v1/dependencies/"+id, nil).Status)
	assert.Empty(t, listPairs(t, fx.manager, fx.board))
	assert.Equal(t, http.StatusNotFound, fx.manager.Do(http.MethodDelete, "/api/v1/dependencies/"+id, nil).Status)

	var aksi []string
	rows, err := fx.pool.Query(context.Background(),
		"SELECT aksi FROM audit_log WHERE entitas = 'dependency' AND entitas_id = $1 ORDER BY id", id)
	require.NoError(t, err)
	defer rows.Close()
	for rows.Next() {
		var s string
		require.NoError(t, rows.Scan(&s))
		aksi = append(aksi, s)
	}
	require.NoError(t, rows.Err())
	assert.Equal(t, []string{"buat", "hapus"}, aksi)
}

func TestDependencyValidation(t *testing.T) {
	fx := newFixture(t)
	a := fx.item(fx.manager, fx.board, fx.group, "A")
	b := fx.item(fx.manager, fx.board, fx.group, "B")
	c := fx.item(fx.manager, fx.board, fx.group, "C")
	d := fx.item(fx.manager, fx.board, fx.group, "D")
	other := fx.item(fx.manager, fx.board2, fx.group2, "Di board lain")

	require.Equal(t, http.StatusCreated, link(fx.manager, fx.board, a, b).Status)
	require.Equal(t, http.StatusCreated, link(fx.manager, fx.board, b, c).Status)

	cases := []struct {
		name       string
		body       map[string]any
		board      string
		wantStatus int
		wantCode   string
	}{
		{"diri sendiri", map[string]any{"predecessor_id": a, "successor_id": a}, fx.board, 422, "validation"},
		{"ganda", map[string]any{"predecessor_id": a, "successor_id": b}, fx.board, 409, "dependency_exists"},
		{"siklus langsung", map[string]any{"predecessor_id": b, "successor_id": a}, fx.board, 422, "dependency_cycle"},
		{"siklus tidak langsung", map[string]any{"predecessor_id": c, "successor_id": a}, fx.board, 422, "dependency_cycle"},
		{"antar-board", map[string]any{"predecessor_id": a, "successor_id": other}, fx.board, 422, "validation"},
		{"item di board lain lewat board-nya", map[string]any{"predecessor_id": a, "successor_id": other}, fx.board2, 422, "validation"},
		{"tipe belum didukung", map[string]any{"predecessor_id": c, "successor_id": d, "tipe": "SS"}, fx.board, 422, "validation"},
		{"lag di luar batas", map[string]any{"predecessor_id": c, "successor_id": d, "lag_hari": 400}, fx.board, 422, "validation"},
		{"id kosong", map[string]any{"predecessor_id": c}, fx.board, 400, "bad_request"},
		{"item tidak ada", map[string]any{"predecessor_id": c, "successor_id": uuid.NewString()}, fx.board, 404, "not_found"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := fx.manager.Do(http.MethodPost, depsPath(tc.board), tc.body)
			assert.Equal(t, tc.wantStatus, r.Status, r.Raw)
			assert.Equal(t, tc.wantCode, r.Body["error"], r.Raw)
		})
	}

	// Pesan siklus menyebut kedua item agar jelas bagi pengguna.
	r := link(fx.manager, fx.board, c, a)
	assert.Contains(t, r.Body["message"], `"A"`)
	assert.Contains(t, r.Body["message"], `"C"`)

	// Graf tak terhubung dan cabang paralel tetap boleh.
	assert.Equal(t, http.StatusCreated, link(fx.manager, fx.board, a, c).Status, "jalur paralel A->C")
	assert.Equal(t, http.StatusCreated, link(fx.manager, fx.board, d, b).Status, "D->B bukan siklus")
	assert.Len(t, listPairs(t, fx.manager, fx.board), 4)
}

func TestDependencyIgnoresDeletedItems(t *testing.T) {
	fx := newFixture(t)
	a := fx.item(fx.manager, fx.board, fx.group, "A")
	b := fx.item(fx.manager, fx.board, fx.group, "B")
	c := fx.item(fx.manager, fx.board, fx.group, "C")
	require.Equal(t, http.StatusCreated, link(fx.manager, fx.board, a, b).Status)
	require.Equal(t, http.StatusCreated, link(fx.manager, fx.board, b, c).Status)

	require.Equal(t, http.StatusNoContent, fx.manager.Do(http.MethodDelete, "/api/v1/items/"+b, nil).Status)
	assert.Empty(t, listPairs(t, fx.manager, fx.board), "dependensi dengan item terhapus tidak tampil")
	assert.Equal(t, http.StatusCreated, link(fx.manager, fx.board, c, a).Status,
		"jalur lewat item terhapus tidak dihitung sebagai siklus")
}

func TestDependencyPermissions(t *testing.T) {
	fx := newFixture(t)
	member := fx.seedUser(auth.RoleMember)
	viewer := fx.seedUser(auth.RoleViewer)
	fx.addMember(member.ID, "member")
	fx.addMember(viewer.ID, "viewer")
	mc, vc := fx.login(member), fx.login(viewer)
	outsider := fx.login(fx.seedUser(auth.RoleMember))

	own := fx.item(mc, fx.board, fx.group, "Milik member")
	own2 := fx.item(mc, fx.board, fx.group, "Milik member 2")
	managers := fx.item(fx.manager, fx.board, fx.group, "Milik manager")

	// Member hanya berhak atas salah satu item: ditolak, ke dua arah.
	assert.Equal(t, http.StatusForbidden, link(mc, fx.board, own, managers).Status)
	assert.Equal(t, http.StatusForbidden, link(mc, fx.board, managers, own).Status)
	r := link(mc, fx.board, own, own2)
	require.Equal(t, http.StatusCreated, r.Status, r.Raw)
	ownDep := r.Body["id"].(string)

	// Setelah ditugaskan pada item manager, member berhak atas keduanya.
	require.Equal(t, http.StatusOK, fx.manager.Do(http.MethodPut, "/api/v1/items/"+managers+"/values/"+fx.personCo,
		map[string]any{"nilai": []uuid.UUID{member.ID}}).Status)
	r = link(mc, fx.board, own2, managers)
	require.Equal(t, http.StatusCreated, r.Status, r.Raw)
	sharedDep := r.Body["id"].(string)
	require.Equal(t, http.StatusOK, fx.manager.Do(http.MethodPut, "/api/v1/items/"+managers+"/values/"+fx.personCo,
		map[string]any{"nilai": nil}).Status)
	assert.Equal(t, http.StatusForbidden, mc.Do(http.MethodDelete, "/api/v1/dependencies/"+sharedDep, nil).Status,
		"hapus juga butuh hak ubah kedua item")

	// Viewer: baca saja. Non-anggota: tidak tahu board atau dependensi itu ada.
	assert.Len(t, listPairs(t, vc, fx.board), 2)
	assert.Equal(t, http.StatusForbidden, link(vc, fx.board, own, own2).Status)
	assert.Equal(t, http.StatusForbidden, vc.Do(http.MethodDelete, "/api/v1/dependencies/"+ownDep, nil).Status)
	assert.Equal(t, http.StatusNotFound, outsider.Do(http.MethodGet, depsPath(fx.board), nil).Status)
	assert.Equal(t, http.StatusNotFound, link(outsider, fx.board, own, own2).Status)
	assert.Equal(t, http.StatusNotFound, outsider.Do(http.MethodDelete, "/api/v1/dependencies/"+ownDep, nil).Status)

	assert.Equal(t, http.StatusNoContent, mc.Do(http.MethodDelete, "/api/v1/dependencies/"+ownDep, nil).Status)
}

func TestDependencyArchivedProject(t *testing.T) {
	fx := newFixture(t)
	a := fx.item(fx.manager, fx.board, fx.group, "A")
	b := fx.item(fx.manager, fx.board, fx.group, "B")
	require.Equal(t, http.StatusOK, fx.manager.Do(http.MethodPost, "/api/v1/projects/"+fx.project+"/archive", nil).Status)
	r := link(fx.manager, fx.board, a, b)
	assert.Equal(t, http.StatusConflict, r.Status, r.Raw)
	assert.Equal(t, "project_archived", r.Body["error"])
}

// Dua permintaan berlawanan yang tiba bersamaan tidak boleh sama-sama lolos
// pemeriksaan siklus (kunci transaksi per board).
func TestDependencyConcurrentCycle(t *testing.T) {
	fx := newFixture(t)
	for round := 0; round < 10; round++ {
		a := fx.item(fx.manager, fx.board, fx.group, "A")
		b := fx.item(fx.manager, fx.board, fx.group, "B")
		statuses := make([]int, 2)
		var wg sync.WaitGroup
		for i, pair := range [][2]string{{a, b}, {b, a}} {
			wg.Add(1)
			go func() {
				defer wg.Done()
				statuses[i] = link(fx.manager, fx.board, pair[0], pair[1]).Status
			}()
		}
		wg.Wait()
		assert.ElementsMatch(t, []int{http.StatusCreated, http.StatusUnprocessableEntity}, statuses, "putaran %d", round)
	}
}
