package schedule_test

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"jwm/internal/demo"
	"jwm/internal/testutil"
)

// Uji beban opsional (JWM_PERF=1): 5.000 item, 2.500 dependensi berantai rapat.
// Mengukur GET /schedule dan pergeseran berantai 2.499 successor dalam satu PUT.
func TestSchedulePerformance5000(t *testing.T) {
	if os.Getenv("JWM_PERF") != "1" {
		t.Skip("set JWM_PERF=1 untuk uji beban")
	}
	fx := newFixture(t)
	ctx := context.Background()
	res, err := demo.Seed(ctx, fx.pool, fx.managerEmail(), demo.Options{Items: 5000, Groups: 5, Seed: 42})
	require.NoError(t, err)
	b := res.BoardID.String()

	var col string
	require.NoError(t, fx.pool.QueryRow(ctx,
		"SELECT id FROM board_columns WHERE board_id = $1 AND tipe = 'timeline' ORDER BY urutan, id LIMIT 1", b).Scan(&col))
	rows, err := fx.pool.Query(ctx, "SELECT id FROM items WHERE board_id = $1 ORDER BY id LIMIT 2500", b)
	require.NoError(t, err)
	var chain []string
	for rows.Next() {
		var id string
		require.NoError(t, rows.Scan(&id))
		chain = append(chain, id)
	}
	require.NoError(t, rows.Err())

	// Rantai rapat: item ke-i satu hari kerja, mulai tepat setelah item ke-(i-1).
	day := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)
	for i, id := range chain {
		for day.Weekday() == time.Saturday || day.Weekday() == time.Sunday {
			day = day.AddDate(0, 0, 1)
		}
		v, _ := json.Marshal(map[string]string{"mulai": day.Format("2006-01-02"), "selesai": day.Format("2006-01-02")})
		_, err := fx.pool.Exec(ctx, `INSERT INTO item_values (item_id, column_id, nilai_json) VALUES ($1, $2, $3)
			ON CONFLICT (item_id, column_id) DO UPDATE SET nilai_json = EXCLUDED.nilai_json`, id, col, v)
		require.NoError(t, err)
		if i > 0 {
			_, err = fx.pool.Exec(ctx, `INSERT INTO dependencies (board_id, predecessor_id, successor_id, dibuat_oleh)
				SELECT $1, $2, $3, dibuat_oleh FROM items WHERE id = $3`, b, chain[i-1], id)
			require.NoError(t, err)
		}
		day = day.AddDate(0, 0, 1)
	}

	timed := func(n int, f func() testutil.Resp) (time.Duration, time.Duration) {
		var ds []time.Duration
		for range n {
			start := time.Now()
			r := f()
			ds = append(ds, time.Since(start))
			require.Equal(t, http.StatusOK, r.Status, r.Raw[:min(len(r.Raw), 300)])
		}
		sort.Slice(ds, func(i, j int) bool { return ds[i] < ds[j] })
		return ds[len(ds)/2], ds[len(ds)-1]
	}
	med, worst := timed(7, func() testutil.Resp { return fx.manager.Do(http.MethodGet, schedulePath(b), nil) })
	t.Logf("GET /schedule (5.000 item, 2.499 dependensi): median %v, maks %v", med, worst)

	var shiftMed, shiftWorst time.Duration
	var shifted int
	for k := range 3 {
		// Perpanjang kepala rantai satu hari kerja lagi tiap putaran: semua successor bergeser.
		end := time.Date(2026, 1, 6+k, 0, 0, 0, 0, time.UTC)
		for end.Weekday() == time.Saturday || end.Weekday() == time.Sunday {
			end = end.AddDate(0, 0, 1)
		}
		start := time.Now()
		r := setTimeline(fx.manager, chain[0], col, "2026-01-05", end.Format("2006-01-02"))
		d := time.Since(start)
		require.Equal(t, http.StatusOK, r.Status, r.Raw[:min(len(r.Raw), 300)])
		shifted = len(r.Body["geser_otomatis"].([]any))
		shiftWorst = max(shiftWorst, d)
		if k == 1 {
			shiftMed = d
		}
	}
	t.Logf("PUT timeline kepala rantai: %d successor bergeser; tipikal %v, maks %v", shifted, shiftMed, shiftWorst)
	require.Equal(t, len(chain)-1, shifted)
}
