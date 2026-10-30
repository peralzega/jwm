// Package testutil menyediakan database PostgreSQL terisolasi untuk tes integrasi.
package testutil

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"jwm/internal/db"
)

var (
	containerOnce sync.Once
	containerURL  string
	containerErr  error
)

// baseURL memilih sumber database: TEST_DATABASE_URL bila diisi, selain itu
// container postgres:18 lewat testcontainers. Tes di-skip bila keduanya tidak ada.
func baseURL(t *testing.T) string {
	t.Helper()
	if u := os.Getenv("TEST_DATABASE_URL"); u != "" {
		return u
	}
	testcontainers.SkipIfProviderIsNotHealthy(t)
	containerOnce.Do(func() {
		ctx := context.Background()
		c, err := postgres.Run(ctx, "postgres:18-alpine",
			postgres.WithDatabase("jwm_test"),
			postgres.WithUsername("jwm"),
			postgres.WithPassword("jwm"),
			testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).WithStartupTimeout(60*time.Second)),
		)
		if err != nil {
			containerErr = err
			return
		}
		containerURL, containerErr = c.ConnectionString(ctx, "sslmode=disable")
	})
	if containerErr != nil {
		t.Fatalf("jalankan container postgres: %v", containerErr)
	}
	return containerURL
}

// NewPool membuat skema baru yang terisolasi, menjalankan semua migrasi di
// dalamnya, dan menghapus skema saat tes selesai.
func NewPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, _ := newSchemaPool(t)
	if err := db.MigrateUp(context.Background(), pool); err != nil {
		t.Fatalf("migrasi: %v", err)
	}
	return pool
}

// NewEmptyPool sama seperti NewPool tanpa menjalankan migrasi.
func NewEmptyPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, _ := newSchemaPool(t)
	return pool
}

func newSchemaPool(t *testing.T) (*pgxpool.Pool, string) {
	t.Helper()
	ctx := context.Background()
	url := baseURL(t)
	schema := "t_" + strings.ToLower(rand.Text()[:12])

	admin, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatalf("koneksi tes: %v", err)
	}
	defer admin.Close(ctx)
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+pgx.Identifier{schema}.Sanitize()); err != nil {
		t.Fatalf("buat skema: %v", err)
	}

	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := db.ConnectConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("pool tes: %v", err)
	}
	t.Cleanup(func() {
		pool.Close()
		c, err := pgx.Connect(context.Background(), url)
		if err != nil {
			t.Logf("bersihkan skema: %v", err)
			return
		}
		defer c.Close(context.Background())
		if _, err := c.Exec(context.Background(),
			fmt.Sprintf("DROP SCHEMA %s CASCADE", pgx.Identifier{schema}.Sanitize())); err != nil {
			t.Logf("hapus skema: %v", err)
		}
	})
	return pool, schema
}
