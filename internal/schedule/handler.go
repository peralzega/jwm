package schedule

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

// Handler menyediakan endpoint dependensi.
type Handler struct {
	svc *Service
	log *slog.Logger
}

// NewHandler membuat handler dependensi.
func NewHandler(svc *Service, log *slog.Logger) *Handler {
	return &Handler{svc: svc, log: log}
}

// Routes dipasang di /api/v1.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/boards/{id}/dependencies", h.list)
	r.Post("/boards/{id}/dependencies", h.create)
	r.Delete("/dependencies/{id}", h.delete)
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
	case errors.Is(err, ErrExists):
		status, code = http.StatusConflict, "dependency_exists"
	case errors.Is(err, ErrCycle):
		status, code = http.StatusUnprocessableEntity, "dependency_cycle"
	case errors.Is(err, ErrSelfDependency), errors.Is(err, ErrInvalidType), errors.Is(err, ErrInvalidLag),
		errors.Is(err, board.ErrWrongBoard):
		status, code = http.StatusUnprocessableEntity, "validation"
	}
	if status == http.StatusInternalServerError {
		h.log.ErrorContext(r.Context(), "schedule", "err", err)
		httpx.WriteError(w, status, code, "Terjadi kesalahan internal.")
		return
	}
	httpx.WriteError(w, status, code, err.Error())
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	boardID, ok := idParam(w, r)
	if !ok {
		return
	}
	actor, _ := actorAndMeta(r)
	items, err := h.svc.List(r.Context(), actor, boardID)
	if err != nil {
		h.writeErr(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	boardID, ok := idParam(w, r)
	if !ok {
		return
	}
	var in struct {
		PredecessorID uuid.UUID `json:"predecessor_id"`
		SuccessorID   uuid.UUID `json:"successor_id"`
		Tipe          string    `json:"tipe"`
		LagHari       int32     `json:"lag_hari"`
	}
	if err := httpx.DecodeJSON(w, r, &in); err != nil || in.PredecessorID == uuid.Nil || in.SuccessorID == uuid.Nil {
		httpx.WriteError(w, http.StatusBadRequest, "bad_request", "Permintaan tidak valid.")
		return
	}
	actor, meta := actorAndMeta(r)
	d, err := h.svc.Create(r.Context(), actor, boardID, CreateInput(in), meta)
	if err != nil {
		h.writeErr(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, d)
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
