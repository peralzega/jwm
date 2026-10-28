// Package user menangani pengelolaan akun oleh Admin dan pencarian pengguna.
package user

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"jwm/internal/audit"
	"jwm/internal/auth"
	"jwm/internal/db"
	"jwm/internal/db/gen"
)

// Galat layanan pengguna.
var (
	ErrForbidden    = errors.New("hanya Admin yang dapat melakukan tindakan ini")
	ErrNotFound     = errors.New("pengguna tidak ditemukan")
	ErrEmailTaken   = errors.New("email sudah dipakai akun lain")
	ErrLastAdmin    = errors.New("sistem harus menyisakan minimal satu Admin aktif")
	ErrSelfAction   = errors.New("tindakan ini tidak dapat dilakukan pada akun sendiri")
	ErrInvalidRole  = errors.New("peran tidak dikenal")
	ErrInvalidName  = errors.New("nama wajib diisi, maksimal 200 karakter")
	ErrInvalidEmail = auth.ErrInvalidEmail
)

// AdminUser adalah tampilan pengguna untuk halaman admin (tanpa hash sandi).
type AdminUser struct {
	ID              uuid.UUID  `json:"id"`
	Nama            string     `json:"nama"`
	Email           string     `json:"email"`
	Peran           auth.Role  `json:"peran"`
	Aktif           bool       `json:"aktif"`
	WajibGantiSandi bool       `json:"wajib_ganti_sandi"`
	TerkunciHingga  *time.Time `json:"terkunci_hingga"`
	LoginTerakhir   *time.Time `json:"login_terakhir"`
	DibuatPada      time.Time  `json:"dibuat_pada"`
}

func toAdminUser(u dbgen.User) AdminUser {
	return AdminUser{
		ID: u.ID, Nama: u.Nama, Email: u.Email, Peran: auth.Role(u.Peran), Aktif: u.Aktif,
		WajibGantiSandi: u.WajibGantiSandi, TerkunciHingga: u.TerkunciHingga,
		LoginTerakhir: u.LoginTerakhir, DibuatPada: u.DibuatPada,
	}
}

// profile adalah bagian data yang dicatat di audit (tanpa data rahasia).
type profile struct {
	Nama  string `json:"nama"`
	Email string `json:"email"`
	Peran string `json:"peran"`
}

func profileOf(u dbgen.User) profile { return profile{Nama: u.Nama, Email: u.Email, Peran: u.Peran} }

// Summary adalah data minimal untuk pemilih pengguna (mis. anggota proyek).
type Summary struct {
	ID    uuid.UUID `json:"id"`
	Nama  string    `json:"nama"`
	Email string    `json:"email"`
}

// Service adalah logika bisnis pengelolaan pengguna.
type Service struct {
	pool   *pgxpool.Pool
	q      *dbgen.Queries
	hasher *auth.Hasher
}

// NewService membuat layanan pengguna.
func NewService(pool *pgxpool.Pool, hasher *auth.Hasher) *Service {
	return &Service{pool: pool, q: dbgen.New(pool), hasher: hasher}
}

func requireAdmin(actor auth.Principal) error {
	if !actor.User.Peran.IsAdmin() {
		return ErrForbidden
	}
	return nil
}

func validateName(nama string) (string, error) {
	nama = strings.TrimSpace(nama)
	if nama == "" || utf8.RuneCountInString(nama) > 200 {
		return "", ErrInvalidName
	}
	return nama, nil
}

// escapeLike meloloskan karakter wildcard LIKE agar dicari secara harfiah.
func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// temporaryPassword membuat sandi sementara acak (26 karakter base32, ~130 bit).
func temporaryPassword() string { return rand.Text() }

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func notFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

// ListFilter menyaring daftar pengguna.
type ListFilter struct {
	Q     string
	Peran *auth.Role
	Aktif *bool
}

// List mengembalikan daftar pengguna untuk Admin.
func (s *Service) List(ctx context.Context, actor auth.Principal, f ListFilter) ([]AdminUser, error) {
	if err := requireAdmin(actor); err != nil {
		return nil, err
	}
	p := dbgen.ListUsersParams{Aktif: f.Aktif}
	if q := strings.TrimSpace(f.Q); q != "" {
		q = escapeLike(q)
		p.Q = &q
	}
	if f.Peran != nil {
		if !f.Peran.Valid() {
			return nil, ErrInvalidRole
		}
		r := string(*f.Peran)
		p.Peran = &r
	}
	rows, err := s.q.ListUsers(ctx, p)
	if err != nil {
		return nil, fmt.Errorf("daftar pengguna: %w", err)
	}
	out := make([]AdminUser, len(rows))
	for i, u := range rows {
		out[i] = toAdminUser(u)
	}
	return out, nil
}

// Get mengembalikan satu pengguna untuk Admin.
func (s *Service) Get(ctx context.Context, actor auth.Principal, id uuid.UUID) (AdminUser, error) {
	if err := requireAdmin(actor); err != nil {
		return AdminUser{}, err
	}
	u, err := s.q.GetUserByID(ctx, id)
	if err != nil {
		return AdminUser{}, fmt.Errorf("baca pengguna: %w", notFound(err))
	}
	return toAdminUser(u), nil
}

// Search mencari pengguna aktif untuk pemilih. Terbuka bagi semua pengguna login.
func (s *Service) Search(ctx context.Context, q string) ([]Summary, error) {
	q = strings.TrimSpace(q)
	if utf8.RuneCountInString(q) < 2 {
		return []Summary{}, nil
	}
	rows, err := s.q.SearchActiveUsers(ctx, escapeLike(q))
	if err != nil {
		return nil, fmt.Errorf("cari pengguna: %w", err)
	}
	out := make([]Summary, len(rows))
	for i, r := range rows {
		out[i] = Summary{ID: r.ID, Nama: r.Nama, Email: r.Email}
	}
	return out, nil
}

// CreateInput adalah data akun baru. Peran kosong berarti Viewer.
type CreateInput struct {
	Nama  string
	Email string
	Peran auth.Role
}

// Created berisi akun baru dan sandi sementaranya. Sandi hanya dikembalikan sekali.
type Created struct {
	User           AdminUser
	SandiSementara string
}

// Create membuat akun baru. Pengguna wajib mengganti sandi pada login pertama.
func (s *Service) Create(ctx context.Context, actor auth.Principal, in CreateInput, meta auth.RequestMeta) (Created, error) {
	if err := requireAdmin(actor); err != nil {
		return Created{}, err
	}
	nama, err := validateName(in.Nama)
	if err != nil {
		return Created{}, err
	}
	email, err := auth.ValidateEmail(in.Email)
	if err != nil {
		return Created{}, err
	}
	peran := in.Peran
	if peran == "" {
		peran = auth.RoleViewer
	}
	if !peran.Valid() {
		return Created{}, ErrInvalidRole
	}
	temp := temporaryPassword()
	hash, err := s.hasher.Hash(temp)
	if err != nil {
		return Created{}, err
	}

	var out Created
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if _, err := lockAdmins(ctx, q, actor); err != nil {
			return err
		}
		u, err := q.CreateUser(ctx, dbgen.CreateUserParams{
			Nama: nama, Email: email, PasswordHash: hash, Peran: string(peran), WajibGantiSandi: true,
		})
		if isUniqueViolation(err) {
			return ErrEmailTaken
		}
		if err != nil {
			return fmt.Errorf("buat pengguna: %w", err)
		}
		out = Created{User: toAdminUser(u), SandiSementara: temp}
		return audit.Record(ctx, q, audit.Entry{
			AktorID: &actor.User.ID, Entitas: "user", EntitasID: u.ID.String(), Aksi: "buat",
			Sesudah: profileOf(u), IP: meta.IP,
		})
	})
	return out, err
}

// UpdateInput berisi perubahan profil. Field nil tidak diubah.
type UpdateInput struct {
	Nama  *string
	Email *string
	Peran *auth.Role
}

// Update mengubah nama, email, atau peran. Peran Admin aktif terakhir tidak dapat diturunkan.
func (s *Service) Update(ctx context.Context, actor auth.Principal, id uuid.UUID, in UpdateInput, meta auth.RequestMeta) (AdminUser, error) {
	if err := requireAdmin(actor); err != nil {
		return AdminUser{}, err
	}
	var out AdminUser
	err := db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		admins, err := lockAdmins(ctx, q, actor)
		if err != nil {
			return err
		}
		before, err := q.GetUserForUpdate(ctx, id)
		if err != nil {
			return fmt.Errorf("baca pengguna: %w", notFound(err))
		}

		next := profileOf(before)
		if in.Nama != nil {
			if next.Nama, err = validateName(*in.Nama); err != nil {
				return err
			}
		}
		if in.Email != nil {
			if next.Email, err = auth.ValidateEmail(*in.Email); err != nil {
				return err
			}
		}
		if in.Peran != nil {
			if !in.Peran.Valid() {
				return ErrInvalidRole
			}
			next.Peran = string(*in.Peran)
		}
		if next.Peran != string(auth.RoleAdmin) && before.Peran == string(auth.RoleAdmin) && before.Aktif {
			if err := ensureOtherAdmin(admins, id); err != nil {
				return err
			}
		}

		after, err := q.UpdateUserProfile(ctx, dbgen.UpdateUserProfileParams{
			ID: id, Nama: next.Nama, Email: next.Email, Peran: next.Peran,
		})
		if isUniqueViolation(err) {
			return ErrEmailTaken
		}
		if err != nil {
			return fmt.Errorf("ubah pengguna: %w", err)
		}
		out = toAdminUser(after)

		if before.Peran != after.Peran {
			if err := audit.Record(ctx, q, audit.Entry{
				AktorID: &actor.User.ID, Entitas: "user", EntitasID: id.String(), Aksi: "ubah_peran",
				Sebelum: map[string]string{"peran": before.Peran},
				Sesudah: map[string]string{"peran": after.Peran}, IP: meta.IP,
			}); err != nil {
				return err
			}
		}
		if before.Nama != after.Nama || before.Email != after.Email {
			return audit.Record(ctx, q, audit.Entry{
				AktorID: &actor.User.ID, Entitas: "user", EntitasID: id.String(), Aksi: "ubah_profil",
				Sebelum: map[string]string{"nama": before.Nama, "email": before.Email},
				Sesudah: map[string]string{"nama": after.Nama, "email": after.Email}, IP: meta.IP,
			})
		}
		return nil
	})
	return out, err
}

// lockAdmins mengunci baris Admin aktif dan memastikan pelaku masih salah satunya.
// Principal dibaca saat autentikasi; pemeriksaan ulang ini menutup jendela ketika
// pelaku diturunkan atau dinonaktifkan oleh Admin lain di tengah permintaan.
func lockAdmins(ctx context.Context, q *dbgen.Queries, actor auth.Principal) ([]uuid.UUID, error) {
	admins, err := q.LockActiveAdmins(ctx)
	if err != nil {
		return nil, fmt.Errorf("kunci admin: %w", err)
	}
	if !slices.Contains(admins, actor.User.ID) {
		return nil, ErrForbidden
	}
	return admins, nil
}

// ensureOtherAdmin memastikan ada Admin aktif lain selain target.
func ensureOtherAdmin(activeAdmins []uuid.UUID, target uuid.UUID) error {
	if slices.ContainsFunc(activeAdmins, func(id uuid.UUID) bool { return id != target }) {
		return nil
	}
	return ErrLastAdmin
}

// SetActive mengaktifkan atau menonaktifkan akun. Menonaktifkan mencabut semua sesi.
// ASSUMPTION: Admin tidak boleh menonaktifkan akunnya sendiri, untuk mencegah terkunci tanpa sengaja.
func (s *Service) SetActive(ctx context.Context, actor auth.Principal, id uuid.UUID, aktif bool, meta auth.RequestMeta) (AdminUser, error) {
	if err := requireAdmin(actor); err != nil {
		return AdminUser{}, err
	}
	if !aktif && id == actor.User.ID {
		return AdminUser{}, ErrSelfAction
	}
	var out AdminUser
	err := db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		admins, err := lockAdmins(ctx, q, actor)
		if err != nil {
			return err
		}
		before, err := q.GetUserForUpdate(ctx, id)
		if err != nil {
			return fmt.Errorf("baca pengguna: %w", notFound(err))
		}
		if !aktif && before.Aktif && before.Peran == string(auth.RoleAdmin) {
			if err := ensureOtherAdmin(admins, id); err != nil {
				return err
			}
		}
		after, err := q.SetUserActive(ctx, dbgen.SetUserActiveParams{ID: id, Aktif: aktif})
		if err != nil {
			return fmt.Errorf("ubah status aktif: %w", err)
		}
		out = toAdminUser(after)

		entry := audit.Entry{
			AktorID: &actor.User.ID, Entitas: "user", EntitasID: id.String(), Aksi: "aktifkan",
			Sebelum: map[string]bool{"aktif": before.Aktif}, IP: meta.IP,
		}
		if aktif {
			entry.Sesudah = map[string]any{"aktif": true}
		} else {
			revoked, err := auth.RevokeAllSessions(ctx, q, id)
			if err != nil {
				return err
			}
			entry.Aksi = "nonaktifkan"
			entry.Sesudah = map[string]any{"aktif": false, "sesi_dicabut": revoked}
		}
		return audit.Record(ctx, q, entry)
	})
	return out, err
}

// ResetPassword membuat sandi sementara baru, membuka kunci, dan mencabut semua sesi.
// Admin mengganti sandinya sendiri lewat /auth/password, bukan lewat endpoint ini.
func (s *Service) ResetPassword(ctx context.Context, actor auth.Principal, id uuid.UUID, meta auth.RequestMeta) (Created, error) {
	if err := requireAdmin(actor); err != nil {
		return Created{}, err
	}
	if id == actor.User.ID {
		return Created{}, ErrSelfAction
	}
	temp := temporaryPassword()
	hash, err := s.hasher.Hash(temp)
	if err != nil {
		return Created{}, err
	}
	var out Created
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if _, err := lockAdmins(ctx, q, actor); err != nil {
			return err
		}
		u, err := q.AdminResetPassword(ctx, dbgen.AdminResetPasswordParams{ID: id, PasswordHash: hash})
		if err != nil {
			return fmt.Errorf("reset sandi: %w", notFound(err))
		}
		revoked, err := auth.RevokeAllSessions(ctx, q, id)
		if err != nil {
			return err
		}
		out = Created{User: toAdminUser(u), SandiSementara: temp}
		return audit.Record(ctx, q, audit.Entry{
			AktorID: &actor.User.ID, Entitas: "user", EntitasID: id.String(), Aksi: "reset_sandi",
			Sesudah: map[string]int64{"sesi_dicabut": revoked}, IP: meta.IP,
		})
	})
	return out, err
}

// Unlock membuka kunci login sementara dan mereset hitungan gagal.
func (s *Service) Unlock(ctx context.Context, actor auth.Principal, id uuid.UUID, meta auth.RequestMeta) (AdminUser, error) {
	if err := requireAdmin(actor); err != nil {
		return AdminUser{}, err
	}
	var out AdminUser
	err := db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if _, err := lockAdmins(ctx, q, actor); err != nil {
			return err
		}
		u, err := q.UnlockUser(ctx, id)
		if err != nil {
			return fmt.Errorf("buka kunci: %w", notFound(err))
		}
		out = toAdminUser(u)
		return audit.Record(ctx, q, audit.Entry{
			AktorID: &actor.User.ID, Entitas: "user", EntitasID: id.String(), Aksi: "buka_kunci", IP: meta.IP,
		})
	})
	return out, err
}
