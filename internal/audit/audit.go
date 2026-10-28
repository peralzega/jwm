// Package audit menulis catatan ke tabel audit_log.
package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"net/netip"

	"github.com/google/uuid"

	"jwm/internal/db/gen"
)

// Entry adalah satu catatan audit. Sebelum dan Sesudah diserialisasi ke JSON;
// jangan pernah mengisinya dengan kata sandi, hash, atau token.
type Entry struct {
	AktorID   *uuid.UUID
	Entitas   string
	EntitasID string
	Aksi      string
	Sebelum   any
	Sesudah   any
	IP        netip.Addr
}

// Record menyisipkan entry memakai q, biasanya terikat pada transaksi pemanggil.
func Record(ctx context.Context, q *dbgen.Queries, e Entry) error {
	sebelum, err := marshal(e.Sebelum)
	if err != nil {
		return fmt.Errorf("audit sebelum: %w", err)
	}
	sesudah, err := marshal(e.Sesudah)
	if err != nil {
		return fmt.Errorf("audit sesudah: %w", err)
	}
	p := dbgen.InsertAuditParams{
		AktorID: e.AktorID,
		Entitas: e.Entitas,
		Aksi:    e.Aksi,
		Sebelum: sebelum,
		Sesudah: sesudah,
	}
	if e.EntitasID != "" {
		p.EntitasID = &e.EntitasID
	}
	if e.IP.IsValid() {
		p.Ip = &e.IP
	}
	if err := q.InsertAudit(ctx, p); err != nil {
		return fmt.Errorf("simpan audit %s/%s: %w", e.Entitas, e.Aksi, err)
	}
	return nil
}

func marshal(v any) ([]byte, error) {
	if v == nil {
		return nil, nil
	}
	return json.Marshal(v)
}
