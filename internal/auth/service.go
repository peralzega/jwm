// Package auth menangani autentikasi lokal, sesi, dan peran global.
package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"jwm/internal/audit"
	"jwm/internal/db"
	"jwm/internal/db/gen"
)

// Galat layanan auth.
var (
	// ErrInvalidCredentials dipakai seragam untuk email tak dikenal, sandi salah,
	// akun nonaktif, dan akun terkunci agar keberadaan akun tidak bocor.
	ErrInvalidCredentials = errors.New("email atau kata sandi salah")
	ErrUnauthenticated    = errors.New("sesi tidak valid")
	ErrSamePassword       = errors.New("kata sandi baru harus berbeda dari yang lama")
)

// Settings adalah parameter perilaku layanan auth.
type Settings struct {
	SessionIdleTTL     time.Duration
	SessionAbsoluteTTL time.Duration
	MaxFailures        int
	LockoutBase        time.Duration
	LockoutMax         time.Duration
}

// RequestMeta adalah informasi permintaan untuk sesi dan audit.
type RequestMeta struct {
	IP        netip.Addr
	UserAgent string
}

// User adalah tampilan pengguna yang aman dikirim ke luar (tanpa hash).
type User struct {
	ID              uuid.UUID `json:"id"`
	Nama            string    `json:"nama"`
	Email           string    `json:"email"`
	Peran           Role      `json:"peran"`
	WajibGantiSandi bool      `json:"wajib_ganti_sandi"`
	Kepadatan       Density   `json:"kepadatan"`
}

func toUser(u dbgen.User) User {
	return User{
		ID: u.ID, Nama: u.Nama, Email: u.Email, Peran: Role(u.Peran), WajibGantiSandi: u.WajibGantiSandi,
		Kepadatan: Density(u.Kepadatan),
	}
}

// Principal adalah pengguna terautentikasi beserta sesinya.
type Principal struct {
	User      User
	SessionID uuid.UUID
	token     string
}

// CSRFToken mengembalikan token CSRF untuk sesi ini.
func (p Principal) CSRFToken() string { return CSRFToken(p.token) }

// SessionResult dikembalikan saat sesi baru dibuat.
type SessionResult struct {
	Token     string
	ExpiresAt time.Time
	User      User
}

// Service adalah logika bisnis autentikasi.
type Service struct {
	pool   *pgxpool.Pool
	q      *dbgen.Queries
	hasher *Hasher
	s      Settings
	log    *slog.Logger
	now    func() time.Time
}

// NewService membuat layanan auth.
func NewService(pool *pgxpool.Pool, hasher *Hasher, s Settings, log *slog.Logger) *Service {
	return &Service{pool: pool, q: dbgen.New(pool), hasher: hasher, s: s, log: log, now: time.Now}
}

func normalizeEmail(email string) string { return strings.ToLower(strings.TrimSpace(email)) }

// Login memverifikasi kredensial dan membuat sesi baru.
func (s *Service) Login(ctx context.Context, email, password string, meta RequestMeta) (SessionResult, error) {
	email = normalizeEmail(email)
	u, err := s.q.GetUserByEmail(ctx, email)
	if errors.Is(err, pgx.ErrNoRows) {
		s.hasher.VerifyDummy(password)
		s.auditLoginFailure(ctx, nil, email, "akun_tidak_ada", meta)
		return SessionResult{}, ErrInvalidCredentials
	}
	if err != nil {
		return SessionResult{}, fmt.Errorf("cari pengguna: %w", err)
	}

	if !u.Aktif {
		s.hasher.VerifyDummy(password)
		s.auditLoginFailure(ctx, &u.ID, email, "nonaktif", meta)
		return SessionResult{}, ErrInvalidCredentials
	}
	if u.TerkunciHingga != nil && u.TerkunciHingga.After(s.now()) {
		// Sandi benar pun ditolak selama terkunci; waktu respons disamakan.
		s.hasher.VerifyDummy(password)
		s.auditLoginFailure(ctx, &u.ID, email, "terkunci", meta)
		return SessionResult{}, ErrInvalidCredentials
	}

	ok, err := s.hasher.Verify(password, u.PasswordHash)
	if err != nil {
		return SessionResult{}, fmt.Errorf("verifikasi sandi: %w", err)
	}
	if !ok {
		if err := s.recordFailure(ctx, u.ID); err != nil {
			return SessionResult{}, err
		}
		s.auditLoginFailure(ctx, &u.ID, email, "sandi_salah", meta)
		return SessionResult{}, ErrInvalidCredentials
	}

	var res SessionResult
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if err := q.RecordLoginSuccess(ctx, u.ID); err != nil {
			return fmt.Errorf("catat login berhasil: %w", err)
		}
		res, err = s.createSession(ctx, q, u, meta)
		if err != nil {
			return err
		}
		return audit.Record(ctx, q, audit.Entry{
			AktorID: &u.ID, Entitas: "auth", EntitasID: u.ID.String(), Aksi: "login_berhasil", IP: meta.IP,
		})
	})
	return res, err
}

// LockoutDuration menghitung durasi kunci progresif untuk jumlah gagal tertentu.
// Nol bila ambang belum tercapai. Tiap kelipatan ambang menggandakan durasi.
func LockoutDuration(failures int, s Settings) time.Duration {
	if failures < s.MaxFailures {
		return 0
	}
	d := s.LockoutBase
	for i := 1; i < failures/s.MaxFailures && d < s.LockoutMax; i++ {
		d *= 2
	}
	return min(d, s.LockoutMax)
}

func (s *Service) recordFailure(ctx context.Context, userID uuid.UUID) error {
	return db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		n, err := q.IncrementLoginFailures(ctx, userID)
		if err != nil {
			return fmt.Errorf("catat login gagal: %w", err)
		}
		// Kunci hanya tepat pada kelipatan ambang; percobaan di antaranya tidak
		// memperpanjang kunci (penundaan progresif, SPEC 7A).
		if int(n)%s.s.MaxFailures != 0 {
			return nil
		}
		until := s.now().Add(LockoutDuration(int(n), s.s))
		if err := q.LockUserUntil(ctx, dbgen.LockUserUntilParams{ID: userID, TerkunciHingga: &until}); err != nil {
			return fmt.Errorf("kunci akun: %w", err)
		}
		return nil
	})
}

func (s *Service) auditLoginFailure(ctx context.Context, userID *uuid.UUID, email, alasan string, meta RequestMeta) {
	s.log.WarnContext(ctx, "login gagal", "alasan", alasan, "ip", meta.IP.String())
	err := audit.Record(ctx, s.q, audit.Entry{
		AktorID: userID, Entitas: "auth", Aksi: "login_gagal", IP: meta.IP,
		Sesudah: map[string]string{"email": email, "alasan": alasan},
	})
	if err != nil {
		s.log.ErrorContext(ctx, "audit login gagal", "err", err)
	}
}

func (s *Service) createSession(ctx context.Context, q *dbgen.Queries, u dbgen.User, meta RequestMeta) (SessionResult, error) {
	token, err := newToken()
	if err != nil {
		return SessionResult{}, err
	}
	now := s.now()
	absolute := now.Add(s.s.SessionAbsoluteTTL)
	idle := now.Add(s.s.SessionIdleTTL)
	if idle.After(absolute) {
		idle = absolute
	}
	p := dbgen.CreateSessionParams{
		UserID:            u.ID,
		TokenHash:         hashToken(token),
		Kedaluwarsa:       idle,
		KedaluwarsaMutlak: absolute,
	}
	if meta.IP.IsValid() {
		p.Ip = &meta.IP
	}
	if meta.UserAgent != "" {
		ua := meta.UserAgent
		if len(ua) > 512 {
			ua = ua[:512]
		}
		p.UserAgent = &ua
	}
	if _, err := q.CreateSession(ctx, p); err != nil {
		return SessionResult{}, fmt.Errorf("simpan sesi: %w", err)
	}
	return SessionResult{Token: token, ExpiresAt: absolute, User: toUser(u)}, nil
}

// touchInterval membatasi penulisan terakhir_aktif agar tidak tiap permintaan.
const touchInterval = time.Minute

// Authenticate memvalidasi token sesi dan memperpanjang masa idle.
func (s *Service) Authenticate(ctx context.Context, token string) (Principal, error) {
	if token == "" {
		return Principal{}, ErrUnauthenticated
	}
	row, err := s.q.GetSessionWithUser(ctx, hashToken(token))
	if errors.Is(err, pgx.ErrNoRows) {
		return Principal{}, ErrUnauthenticated
	}
	if err != nil {
		return Principal{}, fmt.Errorf("baca sesi: %w", err)
	}
	now := s.now()
	if now.Sub(row.Session.TerakhirAktif) > touchInterval {
		err := s.q.TouchSession(ctx, dbgen.TouchSessionParams{ID: row.Session.ID, Kedaluwarsa: now.Add(s.s.SessionIdleTTL)})
		if err != nil {
			return Principal{}, fmt.Errorf("perbarui sesi: %w", err)
		}
	}
	return Principal{User: toUser(row.User), SessionID: row.Session.ID, token: token}, nil
}

// Logout menghapus sesi saat ini.
func (s *Service) Logout(ctx context.Context, p Principal, meta RequestMeta) error {
	return db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if err := q.DeleteSession(ctx, p.SessionID); err != nil {
			return fmt.Errorf("hapus sesi: %w", err)
		}
		return audit.Record(ctx, q, audit.Entry{
			AktorID: &p.User.ID, Entitas: "auth", EntitasID: p.User.ID.String(), Aksi: "logout", IP: meta.IP,
		})
	})
}

// ChangePassword mengganti sandi pengguna sendiri, mencabut semua sesi,
// lalu membuat sesi baru untuk perangkat saat ini.
func (s *Service) ChangePassword(ctx context.Context, p Principal, current, next string, meta RequestMeta) (SessionResult, error) {
	u, err := s.q.GetUserByID(ctx, p.User.ID)
	if err != nil {
		return SessionResult{}, fmt.Errorf("baca pengguna: %w", err)
	}
	ok, err := s.hasher.Verify(current, u.PasswordHash)
	if err != nil {
		return SessionResult{}, fmt.Errorf("verifikasi sandi: %w", err)
	}
	if !ok {
		return SessionResult{}, ErrInvalidCredentials
	}
	if current == next {
		return SessionResult{}, ErrSamePassword
	}
	if err := ValidatePassword(next, u.Email); err != nil {
		return SessionResult{}, err
	}
	hash, err := s.hasher.Hash(next)
	if err != nil {
		return SessionResult{}, err
	}

	var res SessionResult
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if err := q.UpdatePassword(ctx, dbgen.UpdatePasswordParams{ID: u.ID, PasswordHash: hash, WajibGantiSandi: false}); err != nil {
			return fmt.Errorf("simpan sandi: %w", err)
		}
		revoked, err := q.DeleteUserSessions(ctx, u.ID)
		if err != nil {
			return fmt.Errorf("cabut sesi: %w", err)
		}
		u.WajibGantiSandi = false
		res, err = s.createSession(ctx, q, u, meta)
		if err != nil {
			return err
		}
		return audit.Record(ctx, q, audit.Entry{
			AktorID: &u.ID, Entitas: "user", EntitasID: u.ID.String(), Aksi: "ganti_sandi", IP: meta.IP,
			Sesudah: map[string]int64{"sesi_dicabut": revoked},
		})
	})
	return res, err
}

// RevokeAllSessions mencabut semua sesi pengguna. Dipakai saat akun
// dinonaktifkan atau sandinya direset Admin (M1b). Harus dipanggil di dalam
// transaksi yang sama dengan perubahan tersebut.
func RevokeAllSessions(ctx context.Context, q *dbgen.Queries, userID uuid.UUID) (int64, error) {
	n, err := q.DeleteUserSessions(ctx, userID)
	if err != nil {
		return 0, fmt.Errorf("cabut sesi: %w", err)
	}
	return n, nil
}

// CleanupExpiredSessions menghapus sesi kedaluwarsa.
func (s *Service) CleanupExpiredSessions(ctx context.Context) (int64, error) {
	n, err := s.q.DeleteExpiredSessions(ctx)
	if err != nil {
		return 0, fmt.Errorf("hapus sesi kedaluwarsa: %w", err)
	}
	return n, nil
}
