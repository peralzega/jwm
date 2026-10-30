package project

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"jwm/internal/audit"
	"jwm/internal/auth"
	"jwm/internal/db"
	"jwm/internal/db/gen"
)

// Member adalah anggota proyek untuk API.
type Member struct {
	UserID          uuid.UUID `json:"user_id"`
	Nama            string    `json:"nama"`
	Email           string    `json:"email"`
	PeranGlobal     auth.Role `json:"peran_global"`
	Aktif           bool      `json:"aktif"`
	PeranProyek     Role      `json:"peran_proyek"`
	PeranEfektif    Role      `json:"peran_efektif"`
	DitambahkanPada time.Time `json:"ditambahkan_pada"`
}

// memberAudit adalah isi audit keanggotaan.
type memberAudit struct {
	UserID      uuid.UUID `json:"user_id"`
	PeranProyek string    `json:"peran_proyek"`
}

// ListMembers mengembalikan anggota proyek. Semua yang punya akses baca boleh melihat.
func (s *Service) ListMembers(ctx context.Context, actor auth.Principal, projectID uuid.UUID) ([]Member, error) {
	if _, err := Access(ctx, s.q, actor, projectID, RoleViewer, false); err != nil {
		return nil, err
	}
	rows, err := s.q.ListProjectMembers(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("daftar anggota: %w", err)
	}
	out := make([]Member, len(rows))
	for i, r := range rows {
		pr, _ := ParseRole(r.PeranProyek)
		g := auth.Role(r.Peran)
		out[i] = Member{
			UserID: r.UserID, Nama: r.Nama, Email: r.Email, PeranGlobal: g, Aktif: r.Aktif,
			PeranProyek: pr, PeranEfektif: Effective(g, r.Aktif, pr), DitambahkanPada: r.DitambahkanPada,
		}
	}
	return out, nil
}

// addMember menambahkan anggota tanpa pemeriksaan hak pelaku (dipanggil
// setelah pemeriksaan, di dalam transaksi pemanggil).
func addMember(ctx context.Context, q *dbgen.Queries, actorID, projectID, userID uuid.UUID, role Role, meta auth.RequestMeta) error {
	u, err := q.GetUserByID(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrInvalidMember
	}
	if err != nil {
		return fmt.Errorf("baca pengguna: %w", err)
	}
	if !u.Aktif {
		return ErrInvalidMember
	}
	if role > Cap(auth.Role(u.Peran)) {
		return ErrRoleExceedsGlobal
	}
	_, err = q.AddProjectMember(ctx, dbgen.AddProjectMemberParams{
		ProjectID: projectID, UserID: userID, PeranProyek: role.String(), DitambahkanOleh: &actorID,
	})
	if isUniqueViolation(err) {
		return ErrAlreadyMember
	}
	if err != nil {
		return fmt.Errorf("tambah anggota: %w", err)
	}
	return audit.Record(ctx, q, audit.Entry{
		AktorID: &actorID, Entitas: "project_member", EntitasID: projectID.String(), Aksi: "tambah",
		Sesudah: memberAudit{UserID: userID, PeranProyek: role.String()}, IP: meta.IP,
	})
}

// AddMember menambahkan anggota. Hanya manager efektif.
func (s *Service) AddMember(ctx context.Context, actor auth.Principal, projectID, userID uuid.UUID, role Role, meta auth.RequestMeta) error {
	if role < RoleViewer {
		return ErrInvalidProjectRole
	}
	return db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if _, err := lockProject(ctx, q, actor, projectID); err != nil {
			return err
		}
		return addMember(ctx, q, actor.User.ID, projectID, userID, role, meta)
	})
}

// guardManagerLoss menolak perubahan yang membuat proyek kehilangan manager
// terakhir atau membuat pemilik tidak lagi manager.
// ASSUMPTION: yang dihitung adalah baris peran_proyek = manager, bukan hak efektif.
func guardManagerLoss(ctx context.Context, q *dbgen.Queries, p dbgen.Project, m dbgen.ProjectMember) error {
	if m.PeranProyek != RoleManager.String() {
		return nil
	}
	if m.UserID == p.PemilikID {
		return ErrOwnerMember
	}
	n, err := q.CountProjectManagers(ctx, p.ID)
	if err != nil {
		return fmt.Errorf("hitung manager: %w", err)
	}
	if n <= 1 {
		return ErrLastManager
	}
	return nil
}

func getMember(ctx context.Context, q *dbgen.Queries, projectID, userID uuid.UUID) (dbgen.ProjectMember, error) {
	m, err := q.GetProjectMember(ctx, dbgen.GetProjectMemberParams{ProjectID: projectID, UserID: userID})
	if errors.Is(err, pgx.ErrNoRows) {
		return m, ErrMemberNotFound
	}
	if err != nil {
		return m, fmt.Errorf("baca anggota: %w", err)
	}
	return m, nil
}

// UpdateMember mengubah peran proyek seorang anggota. Hanya manager efektif.
func (s *Service) UpdateMember(ctx context.Context, actor auth.Principal, projectID, userID uuid.UUID, role Role, meta auth.RequestMeta) error {
	if role < RoleViewer {
		return ErrInvalidProjectRole
	}
	return db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		p, err := lockProject(ctx, q, actor, projectID)
		if err != nil {
			return err
		}
		m, err := getMember(ctx, q, projectID, userID)
		if err != nil {
			return err
		}
		if m.PeranProyek == role.String() {
			return nil
		}
		if role != RoleManager {
			if err := guardManagerLoss(ctx, q, p, m); err != nil {
				return err
			}
		}
		u, err := q.GetUserByID(ctx, userID)
		if err != nil {
			return fmt.Errorf("baca pengguna: %w", err)
		}
		if role > Cap(auth.Role(u.Peran)) {
			return ErrRoleExceedsGlobal
		}
		if _, err := q.UpdateProjectMemberRole(ctx, dbgen.UpdateProjectMemberRoleParams{
			ProjectID: projectID, UserID: userID, PeranProyek: role.String(),
		}); err != nil {
			return fmt.Errorf("ubah peran anggota: %w", err)
		}
		return audit.Record(ctx, q, audit.Entry{
			AktorID: &actor.User.ID, Entitas: "project_member", EntitasID: projectID.String(), Aksi: "ubah_peran",
			Sebelum: memberAudit{UserID: userID, PeranProyek: m.PeranProyek},
			Sesudah: memberAudit{UserID: userID, PeranProyek: role.String()}, IP: meta.IP,
		})
	})
}

// RemoveMember mengeluarkan anggota. Hanya manager efektif.
func (s *Service) RemoveMember(ctx context.Context, actor auth.Principal, projectID, userID uuid.UUID, meta auth.RequestMeta) error {
	return db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		p, err := lockProject(ctx, q, actor, projectID)
		if err != nil {
			return err
		}
		m, err := getMember(ctx, q, projectID, userID)
		if err != nil {
			return err
		}
		if err := guardManagerLoss(ctx, q, p, m); err != nil {
			return err
		}
		if _, err := q.DeleteProjectMember(ctx, dbgen.DeleteProjectMemberParams{ProjectID: projectID, UserID: userID}); err != nil {
			return fmt.Errorf("hapus anggota: %w", err)
		}
		return audit.Record(ctx, q, audit.Entry{
			AktorID: &actor.User.ID, Entitas: "project_member", EntitasID: projectID.String(), Aksi: "hapus",
			Sebelum: memberAudit{UserID: userID, PeranProyek: m.PeranProyek}, IP: meta.IP,
		})
	})
}
