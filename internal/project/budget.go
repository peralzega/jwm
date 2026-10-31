package project

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"jwm/internal/audit"
	"jwm/internal/auth"
	"jwm/internal/db"
	"jwm/internal/db/gen"
)

// Galat anggaran.
var (
	ErrBudgetNotFound    = errors.New("entri anggaran tidak ditemukan")
	ErrInvalidBudgetDay  = errors.New("tanggal harus berformat YYYY-MM-DD yang valid")
	ErrInvalidBudgetKind = errors.New("jenis harus anggaran atau realisasi")
	ErrInvalidAmount     = errors.New("jumlah harus angka positif dengan paling banyak dua desimal")
	ErrInvalidBudgetNote = errors.New("keterangan maksimal 500 karakter")
)

// amountRe: Rupiah tanpa pemisah ribuan, paling banyak 16 digit bulat dan 2 desimal.
var amountRe = regexp.MustCompile(`^\d{1,16}(\.\d{1,2})?$`)

// BudgetEntry adalah satu entri anggaran atau realisasi. Jumlah berupa string
// desimal agar tidak kehilangan presisi.
type BudgetEntry struct {
	ID             uuid.UUID `json:"id"`
	ProjectID      uuid.UUID `json:"project_id"`
	Tanggal        string    `json:"tanggal"`
	Jenis          string    `json:"jenis"`
	Jumlah         string    `json:"jumlah"`
	Keterangan     string    `json:"keterangan"`
	DibuatOlehNama string    `json:"dibuat_oleh_nama"`
	DibuatPada     time.Time `json:"dibuat_pada"`
	DiubahPada     time.Time `json:"diubah_pada"`
}

// BudgetTotal adalah ringkasan keuangan proyek: sisa = anggaran − realisasi.
type BudgetTotal struct {
	Anggaran  string `json:"anggaran"`
	Realisasi string `json:"realisasi"`
	Sisa      string `json:"sisa"`
}

// Budget adalah respons GET /projects/{id}/budget.
type Budget struct {
	Items    []BudgetEntry `json:"items"`
	Total    BudgetTotal   `json:"total"`
	BisaUbah bool          `json:"bisa_ubah"`
}

// BudgetInput adalah isi entri baru atau perubahan.
type BudgetInput struct {
	Tanggal    string
	Jenis      string
	Jumlah     string
	Keterangan string
}

func (in BudgetInput) validate() (BudgetInput, error) {
	if _, err := time.Parse("2006-01-02", in.Tanggal); err != nil {
		return in, ErrInvalidBudgetDay
	}
	if in.Jenis != "anggaran" && in.Jenis != "realisasi" {
		return in, ErrInvalidBudgetKind
	}
	in.Jumlah = strings.TrimSpace(in.Jumlah)
	if !amountRe.MatchString(in.Jumlah) || strings.Trim(in.Jumlah, "0.") == "" {
		return in, ErrInvalidAmount
	}
	in.Keterangan = strings.TrimSpace(in.Keterangan)
	if !utf8.ValidString(in.Keterangan) || utf8.RuneCountInString(in.Keterangan) > 500 {
		return in, ErrInvalidBudgetNote
	}
	return in, nil
}

// budgetAccess memeriksa hak baca proyek, lalu (bila write) hak input keuangan:
// PMO, atau Project Manager proyek (anggota berperan manager). Admin tanpa
// keanggotaan manager tidak termasuk (SPEC M6: "hanya PMO dan Project Manager
// proyek terkait"). Proyek arsip hanya-baca.
func budgetAccess(ctx context.Context, q *dbgen.Queries, actor auth.Principal, projectID uuid.UUID, write bool) (bool, error) {
	if _, err := Access(ctx, q, actor, projectID, RoleViewer, false); err != nil {
		return false, err
	}
	row, err := q.GetProjectAccess(ctx, dbgen.GetProjectAccessParams{UserID: actor.User.ID, ProjectID: projectID})
	if err != nil {
		return false, fmt.Errorf("baca akses proyek: %w", err)
	}
	can := row.Aktif && (auth.Role(row.Peran) == auth.RolePMO || memberRole(row.PeranProyek) == RoleManager)
	if write {
		if !can {
			return false, ErrForbidden
		}
		if row.DiarsipkanPada != nil {
			return false, ErrArchived
		}
	}
	return can && row.DiarsipkanPada == nil, nil
}

// Budget mengembalikan entri dan total keuangan proyek. Semua anggota proyek.
func (s *Service) Budget(ctx context.Context, actor auth.Principal, projectID uuid.UUID) (Budget, error) {
	can, err := budgetAccess(ctx, s.q, actor, projectID, false)
	if err != nil {
		return Budget{}, err
	}
	rows, err := s.q.ListBudgetEntries(ctx, projectID)
	if err != nil {
		return Budget{}, fmt.Errorf("daftar anggaran: %w", err)
	}
	out := Budget{Items: make([]BudgetEntry, len(rows)), BisaUbah: can, Total: BudgetTotal{"0.00", "0.00", "0.00"}}
	for i, r := range rows {
		out.Items[i] = BudgetEntry{
			ID: r.ID, ProjectID: r.ProjectID, Tanggal: r.Tanggal, Jenis: r.Jenis, Jumlah: r.Jumlah,
			Keterangan: r.Keterangan, DibuatOlehNama: r.DibuatOlehNama, DibuatPada: r.DibuatPada, DiubahPada: r.DiubahPada,
		}
	}
	totals, err := s.q.BudgetTotals(ctx, []uuid.UUID{projectID})
	if err != nil {
		return Budget{}, fmt.Errorf("total anggaran: %w", err)
	}
	if len(totals) == 1 {
		out.Total = BudgetTotal{totals[0].Anggaran, totals[0].Realisasi, totals[0].Sisa}
	}
	return out, nil
}

type budgetSnapshot struct {
	Tanggal    string `json:"tanggal"`
	Jenis      string `json:"jenis"`
	Jumlah     string `json:"jumlah"`
	Keterangan string `json:"keterangan"`
}

func recordBudget(ctx context.Context, q *dbgen.Queries, actor auth.Principal, meta auth.RequestMeta, id uuid.UUID, aksi string, sebelum, sesudah any) error {
	return audit.Record(ctx, q, audit.Entry{
		AktorID: &actor.User.ID, Entitas: "budget_entry", EntitasID: id.String(), Aksi: aksi,
		Sebelum: sebelum, Sesudah: sesudah, IP: meta.IP,
	})
}

// CreateBudgetEntry menambah entri anggaran atau realisasi.
func (s *Service) CreateBudgetEntry(ctx context.Context, actor auth.Principal, projectID uuid.UUID, in BudgetInput, meta auth.RequestMeta) (uuid.UUID, error) {
	in, err := in.validate()
	if err != nil {
		return uuid.Nil, err
	}
	var id uuid.UUID
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if _, err := budgetAccess(ctx, q, actor, projectID, true); err != nil {
			return err
		}
		id, err = q.CreateBudgetEntry(ctx, dbgen.CreateBudgetEntryParams{
			ProjectID: projectID, Tanggal: in.Tanggal, Jenis: in.Jenis, Jumlah: in.Jumlah,
			Keterangan: in.Keterangan, DibuatOleh: actor.User.ID,
		})
		if err != nil {
			return fmt.Errorf("buat entri anggaran: %w", err)
		}
		return recordBudget(ctx, q, actor, meta, id, "buat", nil,
			map[string]any{"project_id": projectID, "entri": budgetSnapshot(in)})
	})
	return id, err
}

// lockBudgetEntry mengunci entri dan memeriksa hak input pada proyeknya.
// Entri proyek yang tidak dapat dilihat dilaporkan tidak ditemukan.
func lockBudgetEntry(ctx context.Context, q *dbgen.Queries, actor auth.Principal, id uuid.UUID) (dbgen.GetBudgetEntryForUpdateRow, error) {
	e, err := q.GetBudgetEntryForUpdate(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return e, ErrBudgetNotFound
	}
	if err != nil {
		return e, fmt.Errorf("kunci entri anggaran: %w", err)
	}
	if _, err := budgetAccess(ctx, q, actor, e.ProjectID, true); errors.Is(err, ErrNotFound) {
		return e, ErrBudgetNotFound
	} else if err != nil {
		return e, err
	}
	return e, nil
}

// UpdateBudgetEntry mengubah entri.
func (s *Service) UpdateBudgetEntry(ctx context.Context, actor auth.Principal, id uuid.UUID, in BudgetInput, meta auth.RequestMeta) error {
	in, err := in.validate()
	if err != nil {
		return err
	}
	return db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		before, err := lockBudgetEntry(ctx, q, actor, id)
		if err != nil {
			return err
		}
		if err := q.UpdateBudgetEntry(ctx, dbgen.UpdateBudgetEntryParams{
			ID: id, Tanggal: in.Tanggal, Jenis: in.Jenis, Jumlah: in.Jumlah, Keterangan: in.Keterangan, DiubahOleh: &actor.User.ID,
		}); err != nil {
			return fmt.Errorf("ubah entri anggaran: %w", err)
		}
		return recordBudget(ctx, q, actor, meta, id, "ubah",
			budgetSnapshot{before.Tanggal, before.Jenis, before.Jumlah, before.Keterangan}, budgetSnapshot(in))
	})
}

// DeleteBudgetEntry menghapus entri.
func (s *Service) DeleteBudgetEntry(ctx context.Context, actor auth.Principal, id uuid.UUID, meta auth.RequestMeta) error {
	return db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		before, err := lockBudgetEntry(ctx, q, actor, id)
		if err != nil {
			return err
		}
		if err := q.DeleteBudgetEntry(ctx, id); err != nil {
			return fmt.Errorf("hapus entri anggaran: %w", err)
		}
		return recordBudget(ctx, q, actor, meta, id, "hapus",
			map[string]any{"project_id": before.ProjectID, "entri": budgetSnapshot{before.Tanggal, before.Jenis, before.Jumlah, before.Keterangan}}, nil)
	})
}
