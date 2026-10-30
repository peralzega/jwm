package auth

import (
	"context"
	"errors"
	"fmt"
	"net/mail"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"jwm/internal/audit"
	"jwm/internal/db"
	"jwm/internal/db/gen"
)

// ErrAdminExists dikembalikan bila seed dijalankan saat Admin sudah ada.
var ErrAdminExists = errors.New("admin sudah ada; seed hanya boleh dijalankan sekali")

// ErrInvalidEmail dikembalikan untuk alamat email yang tidak valid.
var ErrInvalidEmail = errors.New("alamat email tidak valid")

// bootstrapLockKey adalah kunci advisory agar seed tidak berjalan bersamaan.
const bootstrapLockKey = 0x4a574d01

// ValidateEmail menormalkan dan memvalidasi alamat email polos (tanpa nama tampilan).
func ValidateEmail(email string) (string, error) {
	email = normalizeEmail(email)
	addr, err := mail.ParseAddress(email)
	if err != nil || addr.Address != email || len(email) > 254 {
		return "", ErrInvalidEmail
	}
	return email, nil
}

// BootstrapAdmin membuat Admin pertama. Menolak bila sudah ada Admin (aktif
// maupun tidak). Sandi awal wajib diganti pada login pertama.
func BootstrapAdmin(ctx context.Context, pool *pgxpool.Pool, h *Hasher, nama, email, password string) (User, error) {
	email, err := ValidateEmail(email)
	if err != nil {
		return User{}, err
	}
	if err := ValidatePassword(password, email); err != nil {
		return User{}, fmt.Errorf("BOOTSTRAP_ADMIN_PASSWORD: %w", err)
	}
	hash, err := h.Hash(password)
	if err != nil {
		return User{}, err
	}
	var out User
	err = db.InTx(ctx, pool, func(tx pgx.Tx) error {
		q := dbgen.New(tx)
		if err := q.AdvisoryXactLock(ctx, bootstrapLockKey); err != nil {
			return fmt.Errorf("kunci seed: %w", err)
		}
		n, err := q.CountAdmins(ctx)
		if err != nil {
			return fmt.Errorf("hitung admin: %w", err)
		}
		if n > 0 {
			return ErrAdminExists
		}
		u, err := q.CreateUser(ctx, dbgen.CreateUserParams{
			Nama: nama, Email: email, PasswordHash: hash, Peran: string(RoleAdmin), WajibGantiSandi: true,
		})
		if err != nil {
			return fmt.Errorf("buat admin: %w", err)
		}
		out = toUser(u)
		return audit.Record(ctx, q, audit.Entry{
			Entitas: "user", EntitasID: u.ID.String(), Aksi: "bootstrap_admin",
			Sesudah: map[string]string{"email": u.Email, "peran": u.Peran},
		})
	})
	return out, err
}
