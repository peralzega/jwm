package project

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"jwm/internal/audit"
	"jwm/internal/auth"
	"jwm/internal/db"
	"jwm/internal/db/gen"
)

// Galat portofolio dan status manual.
var (
	ErrPortfolioNotFound   = errors.New("portofolio tidak ditemukan")
	ErrInvalidPortfolio    = errors.New("portofolio tidak ditemukan")
	ErrPortfolioName       = errors.New("nama portofolio wajib diisi, maksimal 200 karakter")
	ErrPortfolioExists     = errors.New("nama portofolio sudah dipakai")
	ErrInvalidHealthStatus = errors.New("status harus hijau, kuning, atau merah")
	ErrInvalidReason       = errors.New("alasan wajib diisi, maksimal 500 karakter")
)

// Status health proyek.
const (
	HealthGreen  = "hijau"
	HealthYellow = "kuning"
	HealthRed    = "merah"
)

// CanManagePortfolio: Admin dan PMO mengelola portofolio dan menimpa status
// health proyek (keputusan M5).
func CanManagePortfolio(g auth.Role) bool { return g == auth.RoleAdmin || g == auth.RolePMO }

// Portfolio adalah tampilan portofolio.
type Portfolio struct {
	ID           uuid.UUID `json:"id"`
	Nama         string    `json:"nama"`
	JumlahProyek int32     `json:"jumlah_proyek"`
	DibuatPada   time.Time `json:"dibuat_pada"`
	DiubahPada   time.Time `json:"diubah_pada"`
}

func validPortfolioName(nama string) (string, error) {
	nama = strings.TrimSpace(nama)
	if nama == "" || utf8.RuneCountInString(nama) > 200 {
		return "", ErrPortfolioName
	}
	return nama, nil
}

func isForeignKeyViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23503"
}

func requirePortfolioManager(actor auth.Principal) error {
	if !CanManagePortfolio(actor.User.Peran) {
		return ErrForbidden
	}
	return nil
}

// ListPortfolios mengembalikan semua portofolio (untuk filter dan pilihan). Semua pengguna login.
func (s *Service) ListPortfolios(ctx context.Context) ([]Portfolio, error) {
	rows, err := s.q.ListPortfolios(ctx)
	if err != nil {
		return nil, fmt.Errorf("daftar portofolio: %w", err)
	}
	out := make([]Portfolio, len(rows))
	for i, r := range rows {
		out[i] = Portfolio{ID: r.ID, Nama: r.Nama, JumlahProyek: r.JumlahProyek, DibuatPada: r.DibuatPada, DiubahPada: r.DiubahPada}
	}
	return out, nil
}

func recordPortfolio(ctx context.Context, q *dbgen.Queries, actor auth.Principal, meta auth.RequestMeta, id uuid.UUID, aksi string, sebelum, sesudah any) error {
	return audit.Record(ctx, q, audit.Entry{
		AktorID: &actor.User.ID, Entitas: "portfolio", EntitasID: id.String(), Aksi: aksi,
		Sebelum: sebelum, Sesudah: sesudah, IP: meta.IP,
	})
}

// CreatePortfolio membuat portofolio. Admin dan PMO.
func (s *Service) CreatePortfolio(ctx context.Context, actor auth.Principal, nama string, meta auth.RequestMeta) (Portfolio, error) {
	if err := requirePortfolioManager(actor); err != nil {
		return Portfolio{}, err
	}
	nama, err := validPortfolioName(nama)
	if err != nil {
		return Portfolio{}, err
	}
	var out Portfolio
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		p, err := q.CreatePortfolio(ctx, dbgen.CreatePortfolioParams{Nama: nama, DibuatOleh: actor.User.ID})
		if isUniqueViolation(err) {
			return ErrPortfolioExists
		}
		if err != nil {
			return fmt.Errorf("buat portofolio: %w", err)
		}
		out = Portfolio{ID: p.ID, Nama: p.Nama, DibuatPada: p.DibuatPada, DiubahPada: p.DiubahPada}
		return recordPortfolio(ctx, q, actor, meta, p.ID, "buat", nil, map[string]string{"nama": p.Nama})
	})
	return out, err
}

// RenamePortfolio mengganti nama portofolio. Admin dan PMO.
func (s *Service) RenamePortfolio(ctx context.Context, actor auth.Principal, id uuid.UUID, nama string, meta auth.RequestMeta) (Portfolio, error) {
	if err := requirePortfolioManager(actor); err != nil {
		return Portfolio{}, err
	}
	nama, err := validPortfolioName(nama)
	if err != nil {
		return Portfolio{}, err
	}
	var out Portfolio
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		before, err := q.GetPortfolioForUpdate(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrPortfolioNotFound
		}
		if err != nil {
			return fmt.Errorf("kunci portofolio: %w", err)
		}
		p, err := q.UpdatePortfolio(ctx, dbgen.UpdatePortfolioParams{ID: id, Nama: nama})
		if isUniqueViolation(err) {
			return ErrPortfolioExists
		}
		if err != nil {
			return fmt.Errorf("ubah portofolio: %w", err)
		}
		out = Portfolio{ID: p.ID, Nama: p.Nama, DibuatPada: p.DibuatPada, DiubahPada: p.DiubahPada}
		return recordPortfolio(ctx, q, actor, meta, id, "ubah", map[string]string{"nama": before.Nama}, map[string]string{"nama": p.Nama})
	})
	return out, err
}

// DeletePortfolio menghapus portofolio; proyeknya dilepas (tidak dihapus). Admin dan PMO.
func (s *Service) DeletePortfolio(ctx context.Context, actor auth.Principal, id uuid.UUID, meta auth.RequestMeta) error {
	if err := requirePortfolioManager(actor); err != nil {
		return err
	}
	return db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		before, err := q.GetPortfolioForUpdate(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrPortfolioNotFound
		}
		if err != nil {
			return fmt.Errorf("kunci portofolio: %w", err)
		}
		if err := q.DeletePortfolio(ctx, id); err != nil {
			return fmt.Errorf("hapus portofolio: %w", err)
		}
		return recordPortfolio(ctx, q, actor, meta, id, "hapus", map[string]string{"nama": before.Nama}, nil)
	})
}

// ManualStatus adalah status health yang ditimpa PMO atau Admin.
type ManualStatus struct {
	Status   string    `json:"status"`
	Alasan   string    `json:"alasan"`
	OlehNama string    `json:"oleh_nama"`
	Pada     time.Time `json:"pada"`
}

type manualSnapshot struct {
	Status *string `json:"status_manual"`
	Alasan *string `json:"alasan"`
}

// SetManualStatus menimpa status health proyek dengan alasan. Admin dan PMO;
// proyek arsip ditolak. Tercatat di audit.
func (s *Service) SetManualStatus(ctx context.Context, actor auth.Principal, id uuid.UUID, status, alasan string, meta auth.RequestMeta) error {
	if err := requirePortfolioManager(actor); err != nil {
		return err
	}
	if status != HealthGreen && status != HealthYellow && status != HealthRed {
		return ErrInvalidHealthStatus
	}
	alasan = strings.TrimSpace(alasan)
	if alasan == "" || utf8.RuneCountInString(alasan) > 500 {
		return ErrInvalidReason
	}
	return s.changeManualStatus(ctx, actor, id, meta, "timpa_status", func(q *dbgen.Queries) (dbgen.Project, error) {
		return q.SetProjectManualStatus(ctx, dbgen.SetProjectManualStatusParams{
			ID: id, Status: &status, Alasan: &alasan, Oleh: &actor.User.ID,
		})
	})
}

// ClearManualStatus mengembalikan status health proyek ke perhitungan otomatis.
func (s *Service) ClearManualStatus(ctx context.Context, actor auth.Principal, id uuid.UUID, meta auth.RequestMeta) error {
	if err := requirePortfolioManager(actor); err != nil {
		return err
	}
	return s.changeManualStatus(ctx, actor, id, meta, "hapus_timpa_status", func(q *dbgen.Queries) (dbgen.Project, error) {
		return q.ClearProjectManualStatus(ctx, id)
	})
}

func (s *Service) changeManualStatus(ctx context.Context, actor auth.Principal, id uuid.UUID, meta auth.RequestMeta,
	aksi string, apply func(*dbgen.Queries) (dbgen.Project, error)) error {
	return db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		before, err := q.GetProjectForUpdate(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("kunci proyek: %w", err)
		}
		// Admin dan PMO adalah manager efektif; tetap diperiksa untuk akun nonaktif dan arsip.
		if _, err := Access(ctx, q, actor, id, RoleManager, true); err != nil {
			return err
		}
		after, err := apply(q)
		if err != nil {
			return fmt.Errorf("ubah status manual: %w", err)
		}
		return audit.Record(ctx, q, audit.Entry{
			AktorID: &actor.User.ID, Entitas: "project", EntitasID: id.String(), Aksi: aksi,
			Sebelum: manualSnapshot{Status: before.StatusManual, Alasan: before.StatusManualAlasan},
			Sesudah: manualSnapshot{Status: after.StatusManual, Alasan: after.StatusManualAlasan},
			IP:      meta.IP,
		})
	})
}
