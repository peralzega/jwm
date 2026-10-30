// Package schedule mengelola dependensi antar-item. Perhitungan CPM, kalender
// kerja, dan pergeseran otomatis successor menyusul di M4.
package schedule

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"jwm/internal/audit"
	"jwm/internal/auth"
	"jwm/internal/board"
	"jwm/internal/db"
	"jwm/internal/db/gen"
)

// Tipe dependensi. Hanya FS pada v1 (SPEC M3); SS, FF, SF opsional di fase lanjut.
const TypeFS = "FS"

// Batas lag dalam hari, sama dengan CHECK di migrasi.
// ASSUMPTION: lag negatif (lead) diizinkan; maknanya pada CPM ditetapkan di M4.
const (
	MinLag = -365
	MaxLag = 365
)

// Galat layanan dependensi.
var (
	ErrNotFound       = errors.New("dependensi tidak ditemukan")
	ErrSelfDependency = errors.New("item tidak dapat bergantung pada dirinya sendiri")
	ErrCycle          = errors.New("dependensi ditolak karena membentuk siklus")
	ErrExists         = errors.New("dependensi antara kedua item ini sudah ada")
	ErrInvalidType    = errors.New("tipe dependensi belum didukung; hanya FS")
	ErrInvalidLag     = fmt.Errorf("lag_hari harus antara %d dan %d", MinLag, MaxLag)
)

// Dependency adalah tampilan satu dependensi.
type Dependency struct {
	ID            uuid.UUID `json:"id"`
	BoardID       uuid.UUID `json:"board_id"`
	PredecessorID uuid.UUID `json:"predecessor_id"`
	SuccessorID   uuid.UUID `json:"successor_id"`
	Tipe          string    `json:"tipe"`
	LagHari       int32     `json:"lag_hari"`
}

func toDependency(d dbgen.Dependency) Dependency {
	return Dependency{
		ID: d.ID, BoardID: d.BoardID, PredecessorID: d.PredecessorID, SuccessorID: d.SuccessorID,
		Tipe: d.Tipe, LagHari: d.LagHari,
	}
}

// Service adalah logika bisnis dependensi.
type Service struct {
	pool *pgxpool.Pool
	q    *dbgen.Queries
}

// NewService membuat layanan dependensi.
func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool, q: dbgen.New(pool)}
}

// CreateInput berisi dependensi baru.
type CreateInput struct {
	PredecessorID uuid.UUID
	SuccessorID   uuid.UUID
	Tipe          string
	LagHari       int32
}

// List mengembalikan dependensi aktif pada board. Hak baca board.
func (s *Service) List(ctx context.Context, actor auth.Principal, boardID uuid.UUID) ([]Dependency, error) {
	if err := board.BoardForRead(ctx, s.q, actor, boardID); err != nil {
		return nil, err
	}
	rows, err := s.q.ListDependencies(ctx, boardID)
	if err != nil {
		return nil, fmt.Errorf("daftar dependensi: %w", err)
	}
	out := make([]Dependency, len(rows))
	for i, d := range rows {
		out[i] = toDependency(d)
	}
	return out, nil
}

// lockItemsForEdit mengunci kedua item dalam urutan id (mencegah deadlock antar
// transaksi) dan memastikan pelaku berhak mengubah keduanya.
func lockItemsForEdit(ctx context.Context, q *dbgen.Queries, actor auth.Principal, a, b uuid.UUID) (map[uuid.UUID]dbgen.Item, error) {
	ids := []uuid.UUID{a, b}
	if b.String() < a.String() {
		ids = []uuid.UUID{b, a}
	}
	out := make(map[uuid.UUID]dbgen.Item, 2)
	for _, id := range ids {
		it, err := board.ItemForEdit(ctx, q, actor, id)
		if err != nil {
			return nil, err
		}
		out[id] = it
	}
	return out, nil
}

// Create menambah dependensi antara dua item aktif di board yang sama.
// Pelaku harus berhak mengubah kedua item. Siklus ditolak.
func (s *Service) Create(ctx context.Context, actor auth.Principal, boardID uuid.UUID, in CreateInput, meta auth.RequestMeta) (Dependency, error) {
	if in.Tipe == "" {
		in.Tipe = TypeFS
	}
	if in.Tipe != TypeFS {
		return Dependency{}, ErrInvalidType
	}
	if in.LagHari < MinLag || in.LagHari > MaxLag {
		return Dependency{}, ErrInvalidLag
	}
	if in.PredecessorID == in.SuccessorID {
		return Dependency{}, ErrSelfDependency
	}
	var out Dependency
	err := db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if err := board.BoardForRead(ctx, q, actor, boardID); err != nil {
			return err
		}
		if err := q.LockBoardDependencies(ctx, boardID.String()); err != nil {
			return fmt.Errorf("kunci dependensi board: %w", err)
		}
		items, err := lockItemsForEdit(ctx, q, actor, in.PredecessorID, in.SuccessorID)
		if err != nil {
			return err
		}
		pred, succ := items[in.PredecessorID], items[in.SuccessorID]
		if pred.BoardID != boardID || succ.BoardID != boardID {
			return board.ErrWrongBoard
		}
		// Siklus terbentuk bila successor sudah (langsung atau tidak) mendahului predecessor.
		cycle, err := q.DependencyPathExists(ctx, dbgen.DependencyPathExistsParams{Dari: succ.ID, Ke: pred.ID})
		if err != nil {
			return fmt.Errorf("periksa siklus: %w", err)
		}
		if cycle {
			return fmt.Errorf("%w: %q sudah mendahului %q secara langsung atau tidak langsung",
				ErrCycle, succ.Judul, pred.Judul)
		}
		d, err := q.CreateDependency(ctx, dbgen.CreateDependencyParams{
			BoardID: boardID, PredecessorID: pred.ID, SuccessorID: succ.ID,
			Tipe: in.Tipe, LagHari: in.LagHari, DibuatOleh: actor.User.ID,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrExists
		}
		if err != nil {
			return fmt.Errorf("buat dependensi: %w", err)
		}
		out = toDependency(d)
		return record(ctx, q, actor, meta, d.ID, "buat", nil, out)
	})
	return out, err
}

// Delete menghapus dependensi. Pelaku harus berhak mengubah kedua item.
func (s *Service) Delete(ctx context.Context, actor auth.Principal, id uuid.UUID, meta auth.RequestMeta) error {
	return db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		d, err := q.GetDependencyForUpdate(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("kunci dependensi: %w", err)
		}
		// Non-anggota mendapat 404, sama seperti untuk item.
		if err := board.BoardForRead(ctx, q, actor, d.BoardID); err != nil {
			return err
		}
		if _, err := lockItemsForEdit(ctx, q, actor, d.PredecessorID, d.SuccessorID); err != nil {
			return err
		}
		if err := q.DeleteDependency(ctx, id); err != nil {
			return fmt.Errorf("hapus dependensi: %w", err)
		}
		return record(ctx, q, actor, meta, id, "hapus", toDependency(d), nil)
	})
}

func record(ctx context.Context, q *dbgen.Queries, actor auth.Principal, meta auth.RequestMeta, id uuid.UUID, aksi string, sebelum, sesudah any) error {
	return audit.Record(ctx, q, audit.Entry{
		AktorID: &actor.User.ID, Entitas: "dependency", EntitasID: id.String(), Aksi: aksi,
		Sebelum: sebelum, Sesudah: sesudah, IP: meta.IP,
	})
}
