package report

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"jwm/internal/auth"
	"jwm/internal/httpx"
	"jwm/internal/project"
)

// Handler menyediakan endpoint dasbor portofolio.
type Handler struct {
	svc *Service
	log *slog.Logger
}

// NewHandler membuat handler dasbor.
func NewHandler(svc *Service, log *slog.Logger) *Handler {
	return &Handler{svc: svc, log: log}
}

// Routes dipasang di /api/v1.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/portfolio", h.dashboard)
	r.Get("/portfolio/projects/{id}/risks", h.risks)
}

func (h *Handler) writeErr(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, project.ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, "not_found", err.Error())
	case errors.Is(err, project.ErrForbidden):
		httpx.WriteError(w, http.StatusForbidden, "forbidden", err.Error())
	case errors.Is(err, ErrInvalidFilter):
		httpx.WriteError(w, http.StatusBadRequest, "bad_request", err.Error())
	default:
		h.log.ErrorContext(r.Context(), "portofolio", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "Terjadi kesalahan internal.")
	}
}

func list(v string) []string {
	var out []string
	for part := range strings.SplitSeq(v, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func (h *Handler) dashboard(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := Filter{Prioritas: list(q.Get("prioritas")), Status: list(q.Get("status"))}
	if v := q.Get("pemilik"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "bad_request", "Parameter pemilik tidak valid.")
			return
		}
		f.PemilikID = &id
	}
	switch v := q.Get("portofolio"); v {
	case "":
	case "tanpa":
		f.TanpaPortofolio = true
	default:
		id, err := uuid.Parse(v)
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "bad_request", "Parameter portofolio tidak valid.")
			return
		}
		f.PortfolioID = &id
	}
	actor, _ := auth.PrincipalFrom(r.Context())
	out, err := h.svc.Dashboard(r.Context(), actor, f)
	if err != nil {
		h.writeErr(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) risks(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "bad_request", "ID tidak valid.")
		return
	}
	actor, _ := auth.PrincipalFrom(r.Context())
	out, err := h.svc.Risks(r.Context(), actor, id)
	if err != nil {
		h.writeErr(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}
