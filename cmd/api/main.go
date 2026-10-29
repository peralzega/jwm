// Command api menjalankan server HTTP JWM.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"jwm/internal/auth"
	"jwm/internal/board"
	"jwm/internal/config"
	"jwm/internal/db"
	"jwm/internal/project"
	"jwm/internal/server"
	"jwm/internal/user"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(log); err != nil {
		log.Error("server berhenti", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	hasher, err := auth.NewHasher(auth.DefaultArgon2Params)
	if err != nil {
		return err
	}
	authSvc := auth.NewService(pool, hasher, auth.Settings{
		SessionIdleTTL:     cfg.SessionIdleTTL,
		SessionAbsoluteTTL: cfg.SessionAbsoluteTTL,
		MaxFailures:        cfg.LoginMaxFailures,
		LockoutBase:        cfg.LockoutBase,
		LockoutMax:         cfg.LockoutMax,
	}, log)

	go cleanupSessions(ctx, authSvc, log)

	srv := &http.Server{
		Addr: cfg.HTTPAddr,
		Handler: server.NewRouter(server.Deps{
			Config: cfg, Pool: pool, Auth: authSvc, User: user.NewService(pool, hasher),
			Project: project.NewService(pool), Board: board.NewService(pool), Log: log,
		}),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() {
		log.Info("server berjalan", "addr", cfg.HTTPAddr, "env", cfg.Env)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

func cleanupSessions(ctx context.Context, svc *auth.Service, log *slog.Logger) {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			n, err := svc.CleanupExpiredSessions(ctx)
			if err != nil {
				log.Error("bersihkan sesi", "err", err)
				continue
			}
			log.Info("sesi kedaluwarsa dihapus", "jumlah", n)
		}
	}
}
