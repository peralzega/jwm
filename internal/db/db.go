// Package db menyediakan koneksi pool, transaksi, dan migrasi.
package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"jwm/internal/db/migrations"
)

// Connect membuka pool dan memastikan database dapat dijangkau.
func Connect(ctx context.Context, url string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("parse DATABASE_URL: %w", err)
	}
	return ConnectConfig(ctx, cfg)
}

// ConnectConfig membuka pool dari konfigurasi yang sudah diurai.
func ConnectConfig(ctx context.Context, cfg *pgxpool.Config) (*pgxpool.Pool, error) {
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("buka pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return pool, nil
}

// InTx menjalankan fn di dalam transaksi. Commit bila fn sukses, rollback bila gagal.
func InTx(ctx context.Context, pool *pgxpool.Pool, fn func(pgx.Tx) error) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("mulai transaksi: %w", err)
	}
	if err := fn(tx); err != nil {
		if rbErr := tx.Rollback(ctx); rbErr != nil && !errors.Is(rbErr, pgx.ErrTxClosed) {
			return errors.Join(err, fmt.Errorf("rollback: %w", rbErr))
		}
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

func newProvider(pool *pgxpool.Pool) (*goose.Provider, *sql.DB, error) {
	sqlDB := stdlib.OpenDBFromPool(pool)
	p, err := goose.NewProvider(goose.DialectPostgres, sqlDB, migrations.FS)
	if err != nil {
		_ = sqlDB.Close()
		return nil, nil, fmt.Errorf("buat provider goose: %w", err)
	}
	return p, sqlDB, nil
}

// MigrateUp menerapkan semua migrasi yang belum dijalankan.
func MigrateUp(ctx context.Context, pool *pgxpool.Pool) error {
	p, sqlDB, err := newProvider(pool)
	if err != nil {
		return err
	}
	defer sqlDB.Close()
	if _, err := p.Up(ctx); err != nil {
		return fmt.Errorf("migrasi up: %w", err)
	}
	return nil
}

// MigrateDownAll memundurkan semua migrasi. Dipakai tes untuk memverifikasi Down.
func MigrateDownAll(ctx context.Context, pool *pgxpool.Pool) error {
	p, sqlDB, err := newProvider(pool)
	if err != nil {
		return err
	}
	defer sqlDB.Close()
	if _, err := p.DownTo(ctx, 0); err != nil {
		return fmt.Errorf("migrasi down: %w", err)
	}
	return nil
}
