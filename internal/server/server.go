// Package server merangkai router HTTP aplikasi.
package server

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"jwm/internal/auth"
	"jwm/internal/automation"
	"jwm/internal/board"
	"jwm/internal/config"
	"jwm/internal/httpx"
	"jwm/internal/project"
	"jwm/internal/realtime"
	"jwm/internal/report"
	"jwm/internal/schedule"
	"jwm/internal/user"
)

// Deps adalah dependensi router.
type Deps struct {
	Config  config.Config
	Pool    *pgxpool.Pool
	Auth    *auth.Service
	User    *user.Service
	Project *project.Service
	Board   *board.Service
	// Schedule boleh nil pada tes paket lain; rutenya hanya dipasang bila diisi.
	// Bila diisi, Schedule juga dipasang sebagai pergeseran otomatis di Board.
	Schedule *schedule.Service
	// Report (dasbor portofolio) boleh nil pada tes paket lain.
	Report *report.Service
	// Automation boleh nil pada tes paket lain.
	Automation *automation.Service
	// Realtime (hub SSE) boleh nil; rute /stream hanya dipasang bila diisi.
	Realtime *realtime.Hub
	// StreamKeepAlive dan StreamMaxAge: nol berarti bawaan paket realtime (untuk tes).
	StreamKeepAlive time.Duration
	StreamMaxAge    time.Duration
	Log             *slog.Logger
}

// streamScope: satu proyek yang boleh dibaca (404 bila tidak), atau semua
// proyek aktif yang terlihat pelaku.
func streamScope(svc *project.Service) realtime.ScopeFunc {
	return func(ctx context.Context, actor auth.Principal, pid *uuid.UUID) ([]uuid.UUID, error) {
		if pid != nil {
			p, err := svc.Get(ctx, actor, *pid)
			if err != nil {
				return nil, err
			}
			return []uuid.UUID{p.ID}, nil
		}
		list, err := svc.List(ctx, actor, false)
		if err != nil {
			return nil, err
		}
		ids := make([]uuid.UUID, len(list))
		for i, p := range list {
			ids[i] = p.ID
		}
		return ids, nil
	}
}

// NewRouter membangun handler HTTP lengkap.
func NewRouter(d Deps) http.Handler {
	if d.Schedule != nil && d.Board != nil {
		d.Board.SetCascader(d.Schedule)
	}
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
			boards := board.NewHandler(d.Board, d.Log)
			r.Route("/portfolios", project.NewHandler(d.Project, d.Log).PortfolioRoutes)
			r.Route("/budget-entries", project.NewHandler(d.Project, d.Log).BudgetEntryRoutes)
			r.Route("/projects", func(r chi.Router) {
				project.NewHandler(d.Project, d.Log).Routes(r)
				r.Route("/{id}/boards", boards.ProjectRoutes)
			})
			boards.Routes(r)
			if d.Schedule != nil {
				schedule.NewHandler(d.Schedule, d.Log).Routes(r)
			}
			if d.Report != nil {
				report.NewHandler(d.Report, d.Log).Routes(r)
			}
			if d.Automation != nil {
				automation.NewHandler(d.Automation, d.Log).Routes(r)
			}
			if d.Realtime != nil {
				r.Method(http.MethodGet, "/stream", &realtime.Handler{
					Hub: d.Realtime, Sessions: d.Auth, Scope: streamScope(d.Project), Log: d.Log,
					KeepAlive: d.StreamKeepAlive, MaxAge: d.StreamMaxAge,
					MapErr: func(w http.ResponseWriter, r *http.Request, err error) {
						if errors.Is(err, project.ErrNotFound) {
							httpx.WriteError(w, http.StatusNotFound, "not_found", err.Error())
							return
						}
						d.Log.ErrorContext(r.Context(), "stream", "err", err)
						httpx.WriteError(w, http.StatusInternalServerError, "internal", "Terjadi kesalahan internal.")
					},
				})
			}
			r.Route("/admin", func(r chi.Router) {
				r.Use(httpx.AllowCIDRs(d.Config.AdminAllowedCIDRs))
				users.AdminRoutes(r)
				if d.Schedule != nil {
					schedule.NewHandler(d.Schedule, d.Log).AdminRoutes(r)
				}
			})
		})
	})

	// Selain /api dan /healthz: frontend hasil build (bila WEB_DIR berisi index.html).
	var web http.Handler
	if webDirReady(d.Config.WebDir) {
		web = spaHandler(d.Config.WebDir)
	}
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		if web != nil && !strings.HasPrefix(r.URL.Path, "/api/") {
			web.ServeHTTP(w, r)
			return
		}
		httpx.WriteError(w, http.StatusNotFound, "not_found", "Sumber tidak ditemukan.")
	})
	return r
}
