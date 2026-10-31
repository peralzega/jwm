package realtime

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Channel adalah kanal NOTIFY yang dipakai trigger jwm_notify_change.
const Channel = "jwm_berubah"

// ParsePayload membaca payload "<project_id>:<jenis>".
func ParsePayload(s string) (Event, error) {
	id, jenis, ok := strings.Cut(s, ":")
	pid, err := uuid.Parse(id)
	if !ok || err != nil || jenis == "" {
		return Event{}, fmt.Errorf("payload notifikasi tidak valid: %q", s)
	}
	return Event{ProjectID: pid, Jenis: jenis}, nil
}

// Listen mendengarkan NOTIFY dan meneruskannya ke hub sampai ctx selesai.
// Koneksi yang putus disambung ulang dengan jeda bertahap (1 detik s.d. 30 detik).
func Listen(ctx context.Context, pool *pgxpool.Pool, hub *Hub, log *slog.Logger) {
	wait := time.Second
	for ctx.Err() == nil {
		connected, err := listenOnce(ctx, pool, hub, log)
		if connected {
			wait = time.Second
		}
		if ctx.Err() != nil {
			return
		}
		log.Warn("realtime: koneksi LISTEN terputus, menyambung ulang", "err", err, "jeda", wait)
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		wait = min(wait*2, 30*time.Second)
	}
}

// listenOnce mengembalikan connected = true bila LISTEN sempat berhasil.
func listenOnce(ctx context.Context, pool *pgxpool.Pool, hub *Hub, log *slog.Logger) (bool, error) {
	c, err := pool.Acquire(ctx)
	if err != nil {
		return false, fmt.Errorf("ambil koneksi: %w", err)
	}
	// Koneksi dengan status LISTEN tidak dikembalikan ke pool.
	conn := c.Hijack()
	defer func() { _ = conn.Close(context.Background()) }()
	// LISTEN bukan query data; tidak lewat sqlc. Nama kanal konstanta, bukan masukan.
	if _, err := conn.Exec(ctx, "LISTEN "+Channel); err != nil {
		return false, fmt.Errorf("LISTEN: %w", err)
	}
	log.Info("realtime: mendengarkan perubahan", "kanal", Channel)
	for {
		n, err := conn.WaitForNotification(ctx)
		if err != nil {
			return true, fmt.Errorf("tunggu notifikasi: %w", err)
		}
		e, err := ParsePayload(n.Payload)
		if err != nil {
			log.Warn("realtime", "err", err)
			continue
		}
		hub.Publish(e)
	}
}
