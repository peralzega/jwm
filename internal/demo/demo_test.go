package demo

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"jwm/internal/auth"
	"jwm/internal/board"
	"jwm/internal/db/gen"
	"jwm/internal/testutil"
)

func createUser(t *testing.T, q *dbgen.Queries, email string, peran auth.Role, aktif bool) dbgen.User {
	t.Helper()
	ctx := context.Background()
	// Hash tidak dipakai: tes ini tidak login.
	u, err := q.CreateUser(ctx, dbgen.CreateUserParams{
		Nama: "Uji", Email: email, PasswordHash: "$argon2id$tidak-dipakai", Peran: string(peran),
	})
	require.NoError(t, err)
	if !aktif {
		u, err = q.SetUserActive(ctx, dbgen.SetUserActiveParams{ID: u.ID, Aktif: false})
		require.NoError(t, err)
	}
	return u
}

func TestSeedRejects(t *testing.T) {
	pool := testutil.NewPool(t)
	q := dbgen.New(pool)
	createUser(t, q, "admin@jiep.test", auth.RoleAdmin, true)
	createUser(t, q, "viewer@jiep.test", auth.RoleViewer, true)
	createUser(t, q, "member@jiep.test", auth.RoleMember, true)
	createUser(t, q, "nonaktif@jiep.test", auth.RoleAdmin, false)

	tests := []struct {
		name  string
		email string
		o     Options
		want  error
	}{
		{"item nol", "admin@jiep.test", Options{Items: 0, Groups: 1}, ErrInvalidOptions},
		{"item melebihi batas", "admin@jiep.test", Options{Items: MaxItems + 1, Groups: 1}, ErrInvalidOptions},
		{"grup nol", "admin@jiep.test", Options{Items: 10, Groups: 0}, ErrInvalidOptions},
		{"grup melebihi batas", "admin@jiep.test", Options{Items: 10, Groups: MaxGroups + 1}, ErrInvalidOptions},
		{"email tidak ada", "tidak-ada@jiep.test", Options{Items: 10, Groups: 1}, ErrNotAllowed},
		{"viewer", "viewer@jiep.test", Options{Items: 10, Groups: 1}, ErrNotAllowed},
		{"member", "member@jiep.test", Options{Items: 10, Groups: 1}, ErrNotAllowed},
		{"akun nonaktif", "nonaktif@jiep.test", Options{Items: 10, Groups: 1}, ErrNotAllowed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Seed(context.Background(), pool, tt.email, tt.o)
			assert.ErrorIs(t, err, tt.want)
		})
	}
	var n int
	require.NoError(t, pool.QueryRow(context.Background(), "SELECT count(*) FROM projects").Scan(&n))
	assert.Zero(t, n, "penolakan tidak meninggalkan proyek")
}

// TestSeedAndListLargeBoard membuat board 5.000 item (SPEC M2) lalu membacanya
// lewat ListItems seperti yang dilakukan frontend, halaman demi halaman.
func TestSeedAndListLargeBoard(t *testing.T) {
	if testing.Short() {
		t.Skip("uji beban dilewati pada -short")
	}
	const n = 5000
	ctx := context.Background()
	pool := testutil.NewPool(t)
	q := dbgen.New(pool)
	u := createUser(t, q, "pm@jiep.test", auth.RoleProjectManager, true)

	res, err := Seed(ctx, pool, "pm@jiep.test", Options{Items: n, Groups: 5, Seed: 42})
	require.NoError(t, err)
	assert.Equal(t, n, res.Items)

	actor := auth.Principal{User: auth.User{ID: u.ID, Peran: auth.RoleProjectManager}}
	bs := board.NewService(pool)
	detail, err := bs.GetBoard(ctx, actor, res.BoardID)
	require.NoError(t, err)
	assert.Len(t, detail.Grup, 5)
	types := make([]board.ColumnType, len(detail.Kolom))
	for i, c := range detail.Kolom {
		types[i] = c.Tipe
	}
	for _, want := range []board.ColumnType{board.TypeStatus, board.TypePerson, board.TypePriority, board.TypeDate,
		board.TypeTimeline, board.TypeProgress, board.TypeNumber, board.TypeText} {
		assert.Contains(t, types, want)
	}

	var (
		seen      = map[uuid.UUID]bool{}
		durations []time.Duration
		page      = board.ItemFilter{Batas: board.MaxItemLimit}
		withValue int
	)
	for {
		start := time.Now()
		p, err := bs.ListItems(ctx, actor, res.BoardID, page)
		durations = append(durations, time.Since(start))
		require.NoError(t, err)
		for _, it := range p.Items {
			assert.False(t, seen[it.ID], "item ganda antar halaman")
			seen[it.ID] = true
			assert.True(t, it.BisaUbah && it.BisaHapus, "manager dapat mengubah dan menghapus")
			if len(it.Nilai) > 0 {
				withValue++
			}
		}
		if p.KursorBerikut == nil {
			break
		}
		page.Setelah = p.KursorBerikut
	}
	assert.Len(t, seen, n)
	assert.Greater(t, withValue, n*9/10, "hampir semua item berisi nilai")

	slices.Sort(durations)
	slowest := durations[len(durations)-1]
	t.Logf("ListItems %d halaman x %d item: tercepat %v, terlambat %v", len(durations), board.MaxItemLimit, durations[0], slowest)
	// ASSUMPTION: batas longgar agar tidak rapuh di CI; target p95 < 300 ms
	// (SPEC bagian 7) dicatat lewat log di atas dan diukur di lingkungan nyata.
	assert.Less(t, slowest, 2*time.Second, "satu halaman 1.000 item harus jauh di bawah 2 detik")
}
