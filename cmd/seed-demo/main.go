// Command seed-demo membuat proyek contoh berisi banyak item untuk uji beban
// tampilan tabel dan Kanban. Hanya untuk pengembangan: menolak berjalan bila
// APP_ENV=production.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"jwm/internal/db"
	"jwm/internal/demo"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := run(context.Background(), log, os.Args[1:]); err != nil {
		log.Error("seed-demo gagal", "err", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, log *slog.Logger, args []string) error {
	fs := flag.NewFlagSet("seed-demo", flag.ContinueOnError)
	email := fs.String("email", os.Getenv("BOOTSTRAP_ADMIN_EMAIL"), "email pengguna pemilik proyek demo (Admin, PMO, atau PM)")
	items := fs.Int("items", 5000, "jumlah item")
	groups := fs.Int("groups", 5, "jumlah grup")
	nama := fs.String("nama", "", "nama proyek (default: \"Demo <n> item\")")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if strings.EqualFold(os.Getenv("APP_ENV"), "production") {
		return errors.New("seed-demo tidak boleh dijalankan di production")
	}
	url := os.Getenv("DATABASE_URL")
	if url == "" || strings.TrimSpace(*email) == "" {
		return errors.New("DATABASE_URL dan -email (atau BOOTSTRAP_ADMIN_EMAIL) wajib diisi")
	}

	pool, err := db.Connect(ctx, url)
	if err != nil {
		return err
	}
	defer pool.Close()

	res, err := demo.Seed(ctx, pool, strings.TrimSpace(*email), demo.Options{NamaProyek: *nama, Items: *items, Groups: *groups})
	if err != nil {
		return fmt.Errorf("buat data demo: %w", err)
	}
	log.Info("proyek demo dibuat", "project_id", res.ProjectID, "board_id", res.BoardID, "items", res.Items)
	return nil
}
