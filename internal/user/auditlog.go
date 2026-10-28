package user

import (
	"context"
	"encoding/json"
	"fmt"
	"net/netip"
	"time"

	"github.com/google/uuid"

	"jwm/internal/auth"
	"jwm/internal/db/gen"
)

// AuditEntry adalah satu baris audit_log untuk halaman admin.
type AuditEntry struct {
	ID        int64           `json:"id"`
	AktorID   *uuid.UUID      `json:"aktor_id"`
	Entitas   string          `json:"entitas"`
	EntitasID *string         `json:"entitas_id"`
	Aksi      string          `json:"aksi"`
	Sebelum   json.RawMessage `json:"sebelum"`
	Sesudah   json.RawMessage `json:"sesudah"`
	IP        *netip.Addr     `json:"ip"`
	Waktu     time.Time       `json:"waktu"`
}

// AuditFilter menyaring audit_log. SebelumID adalah kursor (id terakhir halaman sebelumnya).
type AuditFilter struct {
	Entitas   string
	EntitasID string
	AktorID   *uuid.UUID
	SebelumID *int64
	Batas     int
}

// Batas halaman audit_log.
const (
	DefaultAuditLimit = 50
	MaxAuditLimit     = 200
)

// AuditPage adalah satu halaman audit_log, terbaru lebih dulu.
type AuditPage struct {
	Items     []AuditEntry `json:"items"`
	NextAfter *int64       `json:"kursor_berikut"`
}

// AuditLog mengembalikan audit_log untuk Admin dengan paginasi kursor.
func (s *Service) AuditLog(ctx context.Context, actor auth.Principal, f AuditFilter) (AuditPage, error) {
	if err := requireAdmin(actor); err != nil {
		return AuditPage{}, err
	}
	limit := f.Batas
	if limit <= 0 {
		limit = DefaultAuditLimit
	}
	limit = min(limit, MaxAuditLimit)
	p := dbgen.ListAuditParams{AktorID: f.AktorID, SebelumID: f.SebelumID, Batas: int32(limit)}
	if f.Entitas != "" {
		p.Entitas = &f.Entitas
	}
	if f.EntitasID != "" {
		p.EntitasID = &f.EntitasID
	}
	rows, err := s.q.ListAudit(ctx, p)
	if err != nil {
		return AuditPage{}, fmt.Errorf("baca audit: %w", err)
	}
	page := AuditPage{Items: make([]AuditEntry, len(rows))}
	for i, r := range rows {
		page.Items[i] = AuditEntry{
			ID: r.ID, AktorID: r.AktorID, Entitas: r.Entitas, EntitasID: r.EntitasID, Aksi: r.Aksi,
			Sebelum: r.Sebelum, Sesudah: r.Sesudah, IP: r.Ip, Waktu: r.Waktu,
		}
	}
	if len(rows) == limit {
		last := rows[len(rows)-1].ID
		page.NextAfter = &last
	}
	return page, nil
}
