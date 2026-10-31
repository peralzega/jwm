package report_test

import (
	"context"
	"net/http"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"jwm/internal/auth"
	"jwm/internal/demo"
	"jwm/internal/testutil"
)

// Uji beban opsional (JWM_PERF=1): 100 proyek × 500 item (50.000 item),
// 100 dependensi berantai per board. Mengukur GET /portfolio sebagai PMO.
func TestPortfolioPerformance(t *testing.T) {
	if os.Getenv("JWM_PERF") != "1" {
		t.Skip("set JWM_PERF=1 untuk uji beban")
	}
	fx := newFixture(t)
	ctx := context.Background()
	start := time.Now()
	var first string
	for i := range 100 {
		res, err := demo.Seed(ctx, fx.pool, fx.pmUser.Email, demo.Options{Items: 500, Groups: 3, Seed: uint64(i + 1)})
		require.NoError(t, err)
		if i == 0 {
			first = res.ProjectID.String()
		}
		_, err = fx.pool.Exec(ctx, `
			INSERT INTO dependencies (board_id, predecessor_id, successor_id, dibuat_oleh)
			SELECT board_id, prev, id, dibuat_oleh FROM (
				SELECT id, board_id, dibuat_oleh, lag(id) OVER (ORDER BY id) AS prev
				FROM items WHERE board_id = $1 ORDER BY id LIMIT 101
			) s WHERE prev IS NOT NULL`, res.BoardID)
		require.NoError(t, err)
	}
	t.Logf("data: 100 proyek × 500 item dibuat dalam %v", time.Since(start).Round(time.Second))

	pmo := fx.login(fx.seedUser(auth.RolePMO))
	timed := func(n int, f func() testutil.Resp) (time.Duration, time.Duration) {
		var ds []time.Duration
		for range n {
			s := time.Now()
			r := f()
			ds = append(ds, time.Since(s))
			require.Equal(t, http.StatusOK, r.Status, r.Raw[:min(len(r.Raw), 300)])
		}
		sort.Slice(ds, func(i, j int) bool { return ds[i] < ds[j] })
		return ds[len(ds)/2], ds[len(ds)-1]
	}
	var count int
	med, worst := timed(7, func() testutil.Resp {
		r := pmo.Do(http.MethodGet, "/api/v1/portfolio", nil)
		count = len(r.Body["items"].([]any))
		return r
	})
	require.Equal(t, 100, count)
	t.Logf("GET /portfolio (100 proyek, 50.000 item, 9.900 dependensi): median %v, maks %v", med, worst)
	med, worst = timed(7, func() testutil.Resp {
		return pmo.Do(http.MethodGet, "/api/v1/portfolio/projects/"+first+"/risks", nil)
	})
	t.Logf("GET /portfolio/projects/{id}/risks: median %v, maks %v", med, worst)
}
