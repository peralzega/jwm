package automation

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"jwm/internal/auth"
	"jwm/internal/board"
	"jwm/internal/httpx"
	"jwm/internal/project"
)

// Handler menyediakan endpoint aturan otomasi.
type Handler struct {
	svc *Service
	log *slog.Logger
}

// NewHandler membuat handler aturan otomasi.
func NewHandler(svc *Service, log *slog.Logger) *Handler {
	return &Handler{svc: svc, log: log}
}

// Routes dipasang di /api/v1.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/boards/{id}/automations", h.list)
	r.Post("/boards/{id}/automations", h.create)
	r.Patch("/automations/{id}", h.update)
	r.Delete("/automations/{id}", h.delete)
	r.Get("/automations/{id}/runs", h.runs)
	r.Get("/automations/{id}/webhooks", h.deliveries)
	r.Post("/automations/{id}/webhook-secret", h.rotateSecret)
}

func actorAndMeta(r *http.Request) (auth.Principal, auth.RequestMeta) {
	p, _ := auth.PrincipalFrom(r.Context())
	return p, auth.RequestMeta{IP: httpx.IPFrom(r.Context()), UserAgent: r.UserAgent()}
}

func idParam(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "bad_request", "ID tidak valid.")
		return uuid.Nil, false
	}
	return id, true
}

func (h *Handler) writeErr(w http.ResponseWriter, r *http.Request, err error) {
	status, code := http.StatusInternalServerError, "internal"
	switch {
	case errors.Is(err, ErrNotFound), errors.Is(err, board.ErrNotFound), errors.Is(err, project.ErrNotFound):
		status, code = http.StatusNotFound, "not_found"
	case errors.Is(err, project.ErrForbidden):
		status, code = http.StatusForbidden, "forbidden"
	case errors.Is(err, project.ErrArchived):
		status, code = http.StatusConflict, "project_archived"
	case errors.Is(err, ErrTooManyRules):
		status, code = http.StatusConflict, "too_many_rules"
	case errors.Is(err, ErrInvalidRule):
		status, code = http.StatusUnprocessableEntity, "validation"
	}
	if status == http.StatusInternalServerError {
		h.log.ErrorContext(r.Context(), "otomasi", "err", err)
		httpx.WriteError(w, status, code, "Terjadi kesalahan internal.")
		return
	}
	httpx.WriteError(w, status, code, err.Error())
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	actor, _ := actorAndMeta(r)
	out, err := h.svc.List(r.Context(), actor, id)
	if err != nil {
		h.writeErr(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	var in RuleInput
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "bad_request", "Permintaan tidak valid.")
		return
	}
	actor, meta := actorAndMeta(r)
	out, err := h.svc.Create(r.Context(), actor, id, in, meta)
	if err != nil {
		h.writeErr(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, out)
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	var in RuleInput
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "bad_request", "Permintaan tidak valid.")
		return
	}
	actor, meta := actorAndMeta(r)
	out, err := h.svc.Update(r.Context(), actor, id, in, meta)
	if err != nil {
		h.writeErr(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	actor, meta := actorAndMeta(r)
	if err := h.svc.Delete(r.Context(), actor, id, meta); err != nil {
		h.writeErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) runs(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	actor, _ := actorAndMeta(r)
	out, err := h.svc.Runs(r.Context(), actor, id)
	if err != nil {
		h.writeErr(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": out})
}

func (h *Handler) deliveries(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	actor, _ := actorAndMeta(r)
	out, err := h.svc.Deliveries(r.Context(), actor, id)
	if err != nil {
		h.writeErr(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": out})
}

func (h *Handler) rotateSecret(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	actor, meta := actorAndMeta(r)
	secret, err := h.svc.RotateSecret(r.Context(), actor, id, meta)
	if err != nil {
		h.writeErr(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"webhook_rahasia": secret})
}
