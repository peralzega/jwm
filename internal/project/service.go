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
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"jwm/internal/audit"
	"jwm/internal/auth"
	"jwm/internal/db"
	"jwm/internal/db/gen"
	"jwm/internal/httpx"
)

// Galat layanan proyek.
var (
	// ErrNotFound juga dipakai bila pengguna tidak punya akses baca, agar
	// keberadaan proyek tidak bocor ke non-anggota.
	ErrNotFound           = errors.New("proyek tidak ditemukan")
	ErrForbidden          = errors.New("hak akses pada proyek ini tidak mencukupi")
	ErrArchived           = errors.New("proyek diarsipkan; pulihkan dahulu untuk mengubahnya")
	ErrInvalidName        = errors.New("nama proyek wajib diisi, maksimal 200 karakter")
	ErrInvalidDates       = errors.New("tanggal selesai tidak boleh sebelum tanggal mulai")
	ErrInvalidPriority    = errors.New("prioritas harus rendah, sedang, tinggi, atau kritis")
	ErrInvalidProjectRole = errors.New("peran proyek harus manager, member, atau viewer")
	ErrInvalidOwner       = errors.New("pemilik harus pengguna aktif yang berperan manager di proyek ini")
	ErrInvalidMember      = errors.New("pengguna tidak ditemukan atau tidak aktif")
	ErrRoleExceedsGlobal  = errors.New("peran proyek melebihi batas peran global pengguna")
	ErrAlreadyMember      = errors.New("pengguna sudah menjadi anggota proyek")
	ErrMemberNotFound     = errors.New("anggota proyek tidak ditemukan")
	ErrLastManager        = errors.New("proyek harus memiliki minimal satu manager")
	ErrOwnerMember        = errors.New("pemilik proyek harus tetap manager; ganti pemilik terlebih dahulu")
)

// Prioritas yang valid.
var priorities = map[string]bool{"rendah": true, "sedang": true, "tinggi": true, "kritis": true}

// Project adalah tampilan proyek untuk API.
type Project struct {
	ID             uuid.UUID  `json:"id"`
	Nama           string     `json:"nama"`
	PemilikID      uuid.UUID  `json:"pemilik_id"`
	PemilikNama    string     `json:"pemilik_nama"`
	Mulai          *Date      `json:"mulai"`
	Selesai        *Date      `json:"selesai"`
	Prioritas      string     `json:"prioritas"`
	DibuatPada     time.Time  `json:"dibuat_pada"`
	DiubahPada     time.Time  `json:"diubah_pada"`
	DiarsipkanPada *time.Time `json:"diarsipkan_pada"`
	// PeranSaya adalah hak efektif pengguna yang meminta.
	PeranSaya Role `json:"peran_saya"`
}

// snapshot adalah isi audit proyek (tanpa field turunan).
type snapshot struct {
	Nama      string     `json:"nama"`
	PemilikID uuid.UUID  `json:"pemilik_id"`
	Mulai     *Date      `json:"mulai"`
	Selesai   *Date      `json:"selesai"`
	Prioritas string     `json:"prioritas"`
	Arsip     *time.Time `json:"diarsipkan_pada,omitempty"`
}

func snapshotOf(p dbgen.Project) snapshot {
	return snapshot{
		Nama: p.Nama, PemilikID: p.PemilikID, Mulai: fromPgDate(p.Mulai), Selesai: fromPgDate(p.Selesai),
		Prioritas: p.Prioritas, Arsip: p.DiarsipkanPada,
	}
}

// Service adalah logika bisnis proyek dan keanggotaan.
type Service struct {
	pool *pgxpool.Pool
	q    *dbgen.Queries
}

// NewService membuat layanan proyek.
func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool, q: dbgen.New(pool)}
}

func memberRole(s *string) Role {
	if s == nil {
		return RoleNone
	}
	r, _ := ParseRole(*s)
	return r
}

// access membaca hak efektif pelaku pada proyek dari database (bukan dari
// Principal yang dibaca saat autentikasi), lalu memeriksa hak minimal.
// write=true menolak proyek yang diarsipkan.
func access(ctx context.Context, q *dbgen.Queries, actor auth.Principal, projectID uuid.UUID, need Role, write bool) (Role, error) {
	row, err := q.GetProjectAccess(ctx, dbgen.GetProjectAccessParams{UserID: actor.User.ID, ProjectID: projectID})
	if errors.Is(err, pgx.ErrNoRows) {
		return RoleNone, ErrNotFound
	}
	if err != nil {
		return RoleNone, fmt.Errorf("baca akses proyek: %w", err)
	}
	eff := Effective(auth.Role(row.Peran), row.Aktif, memberRole(row.PeranProyek))
	switch {
	case eff < RoleViewer:
		return eff, ErrNotFound
	case eff < need:
		return eff, ErrForbidden
	case write && row.DiarsipkanPada != nil:
		return eff, ErrArchived
	}
	return eff, nil
}

func validateName(nama string) (string, error) {
	nama = strings.TrimSpace(nama)
	if nama == "" || utf8.RuneCountInString(nama) > 200 {
		return "", ErrInvalidName
	}
	return nama, nil
}

func validateDates(mulai, selesai *Date) error {
	if mulai != nil && selesai != nil && selesai.Before(mulai.Time) {
		return ErrInvalidDates
	}
	return nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// canBeManager memeriksa bahwa pengguna aktif dan peran globalnya mengizinkan manager.
func canBeManager(u dbgen.User) bool { return u.Aktif && Cap(auth.Role(u.Peran)) == RoleManager }

// CreateInput adalah data proyek baru. PemilikID nil berarti pembuat.
type CreateInput struct {
	Nama      string
	Mulai     *Date
	Selesai   *Date
	Prioritas string
	PemilikID *uuid.UUID
}

// Create membuat proyek. Pembuat otomatis menjadi manager; pemilik (bila
// berbeda) juga ditambahkan sebagai manager.
// ASSUMPTION: pemilik harus pengguna yang boleh menjadi manager (PM, PMO, Admin).
func (s *Service) Create(ctx context.Context, actor auth.Principal, in CreateInput, meta auth.RequestMeta) (Project, error) {
	nama, err := validateName(in.Nama)
	if err != nil {
		return Project{}, err
	}
	if err := validateDates(in.Mulai, in.Selesai); err != nil {
		return Project{}, err
	}
	if in.Prioritas == "" {
		in.Prioritas = "sedang"
	}
	if !priorities[in.Prioritas] {
		return Project{}, ErrInvalidPriority
	}

	var id uuid.UUID
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		me, err := q.GetUserByID(ctx, actor.User.ID)
		if err != nil {
			return fmt.Errorf("baca pelaku: %w", err)
		}
		if !me.Aktif || !CanCreateProject(auth.Role(me.Peran)) {
			return ErrForbidden
		}
		owner := me
		if in.PemilikID != nil && *in.PemilikID != me.ID {
			owner, err = q.GetUserByID(ctx, *in.PemilikID)
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrInvalidOwner
			}
			if err != nil {
				return fmt.Errorf("baca pemilik: %w", err)
			}
		}
		if !canBeManager(owner) {
			return ErrInvalidOwner
		}

		p, err := q.CreateProject(ctx, dbgen.CreateProjectParams{
			Nama: nama, PemilikID: owner.ID, Mulai: toPgDate(in.Mulai), Selesai: toPgDate(in.Selesai),
			Prioritas: in.Prioritas, DibuatOleh: me.ID,
		})
		if err != nil {
			return fmt.Errorf("buat proyek: %w", err)
		}
		id = p.ID
		if err := audit.Record(ctx, q, audit.Entry{
			AktorID: &me.ID, Entitas: "project", EntitasID: p.ID.String(), Aksi: "buat",
			Sesudah: snapshotOf(p), IP: meta.IP,
		}); err != nil {
			return err
		}
		for _, uid := range uniqueIDs(me.ID, owner.ID) {
			if err := addMember(ctx, q, actor.User.ID, p.ID, uid, RoleManager, meta); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return Project{}, err
	}
	return s.Get(ctx, actor, id)
}

func uniqueIDs(a, b uuid.UUID) []uuid.UUID {
	if a == b {
		return []uuid.UUID{a}
	}
	return []uuid.UUID{a, b}
}

func toProject(p dbgen.Project, pemilikNama string, peran Role) Project {
	return Project{
		ID: p.ID, Nama: p.Nama, PemilikID: p.PemilikID, PemilikNama: pemilikNama,
		Mulai: fromPgDate(p.Mulai), Selesai: fromPgDate(p.Selesai), Prioritas: p.Prioritas,
		DibuatPada: p.DibuatPada, DiubahPada: p.DiubahPada, DiarsipkanPada: p.DiarsipkanPada, PeranSaya: peran,
	}
}

// Get mengembalikan proyek bila pelaku punya akses baca.
func (s *Service) Get(ctx context.Context, actor auth.Principal, id uuid.UUID) (Project, error) {
	eff, err := access(ctx, s.q, actor, id, RoleViewer, false)
	if err != nil {
		return Project{}, err
	}
	v, err := s.q.GetProjectView(ctx, id)
	if err != nil {
		return Project{}, fmt.Errorf("baca proyek: %w", err)
	}
	p := dbgen.Project{
		ID: v.ID, PortfolioID: v.PortfolioID, Nama: v.Nama, PemilikID: v.PemilikID, Mulai: v.Mulai,
		Selesai: v.Selesai, Prioritas: v.Prioritas, DibuatPada: v.DibuatPada, DiubahPada: v.DiubahPada,
		DiarsipkanPada: v.DiarsipkanPada,
	}
	return toProject(p, v.PemilikNama, eff), nil
}

// List mengembalikan proyek yang dapat dilihat pelaku. Admin dan PMO melihat semua.
func (s *Service) List(ctx context.Context, actor auth.Principal, arsip bool) ([]Project, error) {
	g := actor.User.Peran
	out := []Project{}
	if g == auth.RoleAdmin || g == auth.RolePMO {
		rows, err := s.q.ListAllProjects(ctx, dbgen.ListAllProjectsParams{UserID: actor.User.ID, Arsip: arsip})
		if err != nil {
			return nil, fmt.Errorf("daftar proyek: %w", err)
		}
		for _, r := range rows {
			p := dbgen.Project{
				ID: r.ID, Nama: r.Nama, PemilikID: r.PemilikID, Mulai: r.Mulai, Selesai: r.Selesai,
				Prioritas: r.Prioritas, DibuatPada: r.DibuatPada, DiubahPada: r.DiubahPada, DiarsipkanPada: r.DiarsipkanPada,
			}
			out = append(out, toProject(p, r.PemilikNama, Effective(g, true, memberRole(r.PeranProyek))))
		}
		return out, nil
	}
	rows, err := s.q.ListMemberProjects(ctx, dbgen.ListMemberProjectsParams{UserID: actor.User.ID, Arsip: arsip})
	if err != nil {
		return nil, fmt.Errorf("daftar proyek: %w", err)
	}
	for _, r := range rows {
		eff := Effective(g, true, memberRole(&r.PeranProyek))
		if eff < RoleViewer {
			continue
		}
		p := dbgen.Project{
			ID: r.ID, Nama: r.Nama, PemilikID: r.PemilikID, Mulai: r.Mulai, Selesai: r.Selesai,
			Prioritas: r.Prioritas, DibuatPada: r.DibuatPada, DiubahPada: r.DiubahPada, DiarsipkanPada: r.DiarsipkanPada,
		}
		out = append(out, toProject(p, r.PemilikNama, eff))
	}
	return out, nil
}

// UpdateInput berisi perubahan proyek. Field yang tidak dikirim tidak diubah;
// mulai/selesai bernilai null menghapus tanggal.
type UpdateInput struct {
	Nama      httpx.Optional[string]
	Mulai     httpx.Optional[Date]
	Selesai   httpx.Optional[Date]
	Prioritas httpx.Optional[string]
	PemilikID httpx.Optional[uuid.UUID]
}

// Update mengubah proyek. Hanya manager efektif.
func (s *Service) Update(ctx context.Context, actor auth.Principal, id uuid.UUID, in UpdateInput, meta auth.RequestMeta) (Project, error) {
	err := db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		before, err := lockProject(ctx, q, actor, id)
		if err != nil {
			return err
		}
		p := dbgen.UpdateProjectParams{
			ID: id, Nama: before.Nama, PemilikID: before.PemilikID, Mulai: before.Mulai,
			Selesai: before.Selesai, Prioritas: before.Prioritas,
		}
		if in.Nama.Set {
			if in.Nama.Null {
				return ErrInvalidName
			}
			if p.Nama, err = validateName(in.Nama.Value); err != nil {
				return err
			}
		}
		if in.Mulai.Set {
			p.Mulai = optionalDate(in.Mulai)
		}
		if in.Selesai.Set {
			p.Selesai = optionalDate(in.Selesai)
		}
		if err := validateDates(fromPgDate(p.Mulai), fromPgDate(p.Selesai)); err != nil {
			return err
		}
		if in.Prioritas.Set {
			if in.Prioritas.Null || !priorities[in.Prioritas.Value] {
				return ErrInvalidPriority
			}
			p.Prioritas = in.Prioritas.Value
		}
		if in.PemilikID.Set {
			if in.PemilikID.Null {
				return ErrInvalidOwner
			}
			if err := checkOwner(ctx, q, id, in.PemilikID.Value); err != nil {
				return err
			}
			p.PemilikID = in.PemilikID.Value
		}

		after, err := q.UpdateProject(ctx, p)
		if err != nil {
			return fmt.Errorf("ubah proyek: %w", err)
		}
		return audit.Record(ctx, q, audit.Entry{
			AktorID: &actor.User.ID, Entitas: "project", EntitasID: id.String(), Aksi: "ubah",
			Sebelum: snapshotOf(before), Sesudah: snapshotOf(after), IP: meta.IP,
		})
	})
	if err != nil {
		return Project{}, err
	}
	return s.Get(ctx, actor, id)
}

func optionalDate(o httpx.Optional[Date]) pgtype.Date {
	if o.Null {
		return toPgDate(nil)
	}
	return toPgDate(&o.Value)
}

// checkOwner memastikan calon pemilik adalah anggota manager yang aktif dan
// peran globalnya mengizinkan manager.
func checkOwner(ctx context.Context, q *dbgen.Queries, projectID, userID uuid.UUID) error {
	m, err := q.GetProjectMember(ctx, dbgen.GetProjectMemberParams{ProjectID: projectID, UserID: userID})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrInvalidOwner
	}
	if err != nil {
		return fmt.Errorf("baca anggota: %w", err)
	}
	if m.PeranProyek != RoleManager.String() {
		return ErrInvalidOwner
	}
	u, err := q.GetUserByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("baca pengguna: %w", err)
	}
	if !canBeManager(u) {
		return ErrInvalidOwner
	}
	return nil
}

// lockProject mengunci baris proyek (urutan kunci: proyek, lalu anggota) dan
// memastikan pelaku adalah manager efektif pada proyek yang tidak diarsipkan.
func lockProject(ctx context.Context, q *dbgen.Queries, actor auth.Principal, id uuid.UUID) (dbgen.Project, error) {
	p, err := q.GetProjectForUpdate(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return dbgen.Project{}, ErrNotFound
	}
	if err != nil {
		return dbgen.Project{}, fmt.Errorf("kunci proyek: %w", err)
	}
	if _, err := access(ctx, q, actor, id, RoleManager, true); err != nil {
		return dbgen.Project{}, err
	}
	return p, nil
}

// SetArchived mengarsipkan atau memulihkan proyek. Hanya manager efektif.
func (s *Service) SetArchived(ctx context.Context, actor auth.Principal, id uuid.UUID, arsip bool, meta auth.RequestMeta) (Project, error) {
	err := db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		before, err := q.GetProjectForUpdate(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("kunci proyek: %w", err)
		}
		if _, err := access(ctx, q, actor, id, RoleManager, false); err != nil {
			return err
		}
		if (before.DiarsipkanPada != nil) == arsip {
			return nil
		}
		after, err := q.SetProjectArchived(ctx, dbgen.SetProjectArchivedParams{ID: id, Arsip: arsip})
		if err != nil {
			return fmt.Errorf("arsip proyek: %w", err)
		}
		aksi := "pulihkan"
		if arsip {
			aksi = "arsipkan"
		}
		return audit.Record(ctx, q, audit.Entry{
			AktorID: &actor.User.ID, Entitas: "project", EntitasID: id.String(), Aksi: aksi,
			Sebelum: map[string]*time.Time{"diarsipkan_pada": before.DiarsipkanPada},
			Sesudah: map[string]*time.Time{"diarsipkan_pada": after.DiarsipkanPada}, IP: meta.IP,
		})
	})
	if err != nil {
		return Project{}, err
	}
	return s.Get(ctx, actor, id)
}
