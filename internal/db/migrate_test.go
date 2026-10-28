package db_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"jwm/internal/db"
	"jwm/internal/testutil"
)

func TestMigrationsUpDownUp(t *testing.T) {
	ctx := context.Background()
	pool := testutil.NewEmptyPool(t)

	require.NoError(t, db.MigrateUp(ctx, pool))
	require.NoError(t, db.MigrateDownAll(ctx, pool))

	var n int
	require.NoError(t, pool.QueryRow(ctx,
		"SELECT count(*) FROM information_schema.tables WHERE table_schema = current_schema() AND table_name <> 'goose_db_version'").Scan(&n))
	require.Zero(t, n, "semua tabel harus terhapus setelah down")

	require.NoError(t, db.MigrateUp(ctx, pool))
}
