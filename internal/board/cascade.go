package board

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"jwm/internal/auth"
	"jwm/internal/db/gen"
)

// ErrShiftForbidden: pergeseran otomatis membutuhkan hak ubah atas successor
// yang tidak dimiliki pelaku. Seluruh perubahan dibatalkan.
var ErrShiftForbidden = errors.New("perubahan ditolak karena successor berikut harus ikut bergeser, tetapi Anda tidak berhak mengubahnya")

// ShiftedItem adalah item yang ikut digeser otomatis oleh perubahan timeline.
type ShiftedItem struct {
	ID            uuid.UUID `json:"id"`
	Judul         string    `json:"judul"`
	Mulai         string    `json:"mulai"`
	Selesai       string    `json:"selesai"`
	PredecessorID uuid.UUID `json:"predecessor_id"`
}

// Cascader menggeser successor saat timeline item berubah. Diisi paket
// schedule (yang bergantung pada paket ini) lewat SetCascader.
type Cascader interface {
	// LockBoard dipanggil sebelum item dikunci, agar urutan kunci sama dengan
	// pembuatan dependensi: kunci board terlebih dahulu, lalu baris item.
	LockBoard(ctx context.Context, q *dbgen.Queries, boardID uuid.UUID) error
	// AfterTimelineChange berjalan di transaksi yang sama setelah nilai
	// disimpan. Galat membatalkan seluruh transaksi, termasuk perubahan item.
	AfterTimelineChange(ctx context.Context, q *dbgen.Queries, actor auth.Principal, meta auth.RequestMeta,
		item dbgen.Item, columnID uuid.UUID) ([]ShiftedItem, error)
}

// SetCascader memasang pergeseran otomatis. Tanpa cascader (mis. tes paket
// lain) perubahan timeline tidak menggeser successor.
func (s *Service) SetCascader(c Cascader) { s.cascade = c }
