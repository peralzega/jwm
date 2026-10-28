package auth

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"jwm/internal/httpx"
)

type ctxKey int

const principalKey ctxKey = iota

// CSRFHeader adalah header tempat klien mengirim token CSRF.
const CSRFHeader = "X-CSRF-Token"

// PrincipalFrom mengambil pengguna terautentikasi dari context.
func PrincipalFrom(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(principalKey).(Principal)
	return p, ok
}

// WithPrincipal menaruh p di context. Dipakai juga oleh tes paket lain.
func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, principalKey, p)
}

func isSafeMethod(m string) bool {
	return m == http.MethodGet || m == http.MethodHead || m == http.MethodOptions
}

// RequireAuth mewajibkan sesi valid. Untuk metode pengubah data, token CSRF
// di header harus cocok dengan sesi.
func RequireAuth(svc *Service, cookies CookieConfig, log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c, err := r.Cookie(cookies.Name())
			if err != nil {
				httpx.WriteError(w, http.StatusUnauthorized, "unauthenticated", "Silakan masuk terlebih dahulu.")
				return
			}
			p, err := svc.Authenticate(r.Context(), c.Value)
			if errors.Is(err, ErrUnauthenticated) {
				cookies.Clear(w)
				httpx.WriteError(w, http.StatusUnauthorized, "unauthenticated", "Sesi berakhir. Silakan masuk kembali.")
				return
			}
			if err != nil {
				log.ErrorContext(r.Context(), "autentikasi", "err", err)
				httpx.WriteError(w, http.StatusInternalServerError, "internal", "Terjadi kesalahan internal.")
				return
			}
			if !isSafeMethod(r.Method) && !validCSRF(c.Value, r.Header.Get(CSRFHeader)) {
				httpx.WriteError(w, http.StatusForbidden, "csrf", "Token CSRF tidak valid.")
				return
			}
			next.ServeHTTP(w, r.WithContext(WithPrincipal(r.Context(), p)))
		})
	}
}

// RequirePasswordChanged menolak akses bila pengguna masih wajib ganti sandi.
// Dipasang pada semua rute kecuali /auth/me, /auth/password, dan /auth/logout.
func RequirePasswordChanged(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := PrincipalFrom(r.Context())
		if !ok {
			httpx.WriteError(w, http.StatusUnauthorized, "unauthenticated", "Silakan masuk terlebih dahulu.")
			return
		}
		if p.User.WajibGantiSandi {
			httpx.WriteError(w, http.StatusForbidden, "password_change_required", "Ganti kata sandi terlebih dahulu.")
			return
		}
		next.ServeHTTP(w, r)
	})
}
