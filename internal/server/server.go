// Package server merangkai router HTTP aplikasi.
package server

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"jwm/internal/auth"
	"jwm/internal/config"
	"jwm/internal/httpx"
	"jwm/internal/project"
	"jwm/internal/user"
)

// Deps adalah dependensi router.
type Deps struct {
	Config  config.Config
	Pool    *pgxpool.Pool
	Auth    *auth.Service
	User    *user.Service
	Project *project.Service
	Log     *slog.Logger
}

// NewRouter membangun handler HTTP lengkap.
func NewRouter(d Deps) http.Handler {
	cookies := auth.CookieConfig{Secure: d.Config.CookieSecure}
	loginLimiter := httpx.NewRateLimiter(d.Config.LoginRatePerMinute)

	r := chi.NewRouter()
	r.Use(
		httpx.WithClientIP(d.Config.TrustedProxies),
		httpx.Recover(d.Log),
		httpx.Logger(d.Log),
		httpx.SecurityHeaders(d.Config.CookieSecure),
		httpx.CrossOrigin(),
	)

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := d.Pool.Ping(ctx); err != nil {
			d.Log.ErrorContext(ctx, "healthz", "err", err)
			httpx.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "db_down"})
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	r.Route("/api/v1", func(r chi.Router) {
		r.Route("/auth", auth.NewHandler(d.Auth, cookies, loginLimiter, d.Log).Routes)

		// Semua rute lain: wajib login dan sandi sudah diganti.
		r.Group(func(r chi.Router) {
			r.Use(auth.RequireAuth(d.Auth, cookies, d.Log), auth.RequirePasswordChanged)
			users := user.NewHandler(d.User, d.Log)
			r.Route("/users", users.Routes)
			r.Route("/projects", project.NewHandler(d.Project, d.Log).Routes)
			r.Route("/admin", func(r chi.Router) {
				r.Use(httpx.AllowCIDRs(d.Config.AdminAllowedCIDRs))
				users.AdminRoutes(r)
			})
		})
	})

	r.NotFound(func(w http.ResponseWriter, _ *http.Request) {
		httpx.WriteError(w, http.StatusNotFound, "not_found", "Sumber tidak ditemukan.")
	})
	return r
}
