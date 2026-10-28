// Command seed membuat Admin pertama dari BOOTSTRAP_ADMIN_EMAIL dan
// BOOTSTRAP_ADMIN_PASSWORD. Menolak berjalan bila Admin sudah ada.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"jwm/internal/auth"
	"jwm/internal/db"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := run(context.Background(), log); err != nil {
		log.Error("seed gagal", "err", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, log *slog.Logger) error {
	url := os.Getenv("DATABASE_URL")
	email := strings.TrimSpace(os.Getenv("BOOTSTRAP_ADMIN_EMAIL"))
	password := os.Getenv("BOOTSTRAP_ADMIN_PASSWORD")
	nama := strings.TrimSpace(os.Getenv("BOOTSTRAP_ADMIN_NAME"))
	if nama == "" {
		nama = "Administrator"
	}
	if url == "" || email == "" || password == "" {
		return errors.New("DATABASE_URL, BOOTSTRAP_ADMIN_EMAIL, dan BOOTSTRAP_ADMIN_PASSWORD wajib diisi")
	}

	pool, err := db.Connect(ctx, url)
	if err != nil {
		return err
	}
	defer pool.Close()

	hasher, err := auth.NewHasher(auth.DefaultArgon2Params)
	if err != nil {
		return err
	}
	u, err := auth.BootstrapAdmin(ctx, pool, hasher, nama, email, password)
	if err != nil {
		return fmt.Errorf("bootstrap admin: %w", err)
	}
	log.Info("admin pertama dibuat; sandi wajib diganti saat login pertama", "id", u.ID, "email", u.Email)
	return nil
}
