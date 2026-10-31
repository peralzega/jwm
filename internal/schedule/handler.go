package schedule

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

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
	r.Get("/boards/{id}/schedule", h.schedule)
	r.Get("/work-calendar", h.calendar)
}

// AdminRoutes dipasang di /api/v1/admin (dibatasi ADMIN_ALLOWED_CIDRS).
func (h *Handler) AdminRoutes(r chi.Router) {
	r.Put("/work-calendar/{tanggal}", h.setCalendarDay)
	r.Delete("/work-calendar/{tanggal}", h.deleteCalendarDay)
	r.Post("/work-calendar/import", h.importCalendar)
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
	case errors.Is(err, ErrNotFound), errors.Is(err, board.ErrNotFound), errors.Is(err, project.ErrNotFound),
		errors.Is(err, ErrCalendarNotFound):
		status, code = http.StatusNotFound, "not_found"
	case errors.Is(err, ErrNoTimeline):
		status, code = http.StatusUnprocessableEntity, "no_timeline"
	case errors.Is(err, ErrInvalidCSV):
		status, code = http.StatusUnprocessableEntity, "invalid_csv"
	case errors.Is(err, ErrNotTimeline), errors.Is(err, ErrScheduleTooWide), errors.Is(err, ErrInvalidDate),
		errors.Is(err, ErrInvalidNote), errors.Is(err, ErrInvalidYear):
		status, code = http.StatusUnprocessableEntity, "validation"
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

func (h *Handler) schedule(w http.ResponseWriter, r *http.Request) {
	boardID, ok := idParam(w, r)
	if !ok {
		return
	}
	var col *uuid.UUID
	if v := r.URL.Query().Get("timeline"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "bad_request", "Parameter timeline tidak valid.")
			return
		}
		col = &id
	}
	actor, _ := actorAndMeta(r)
	view, err := h.svc.Schedule(r.Context(), actor, boardID, col)
	if err != nil {
		h.writeErr(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, view)
}

func (h *Handler) calendar(w http.ResponseWriter, r *http.Request) {
	tahun := time.Now().Year()
	if v := r.URL.Query().Get("tahun"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "bad_request", "Parameter tahun tidak valid.")
			return
		}
		tahun = n
	}
	out, err := h.svc.CalendarYear(r.Context(), tahun)
	if err != nil {
		h.writeErr(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) setCalendarDay(w http.ResponseWriter, r *http.Request) {
	var in struct {
		HariKerja  bool   `json:"hari_kerja"`
		Keterangan string `json:"keterangan"`
	}
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "bad_request", "Permintaan tidak valid.")
		return
	}
	actor, meta := actorAndMeta(r)
	d, err := h.svc.SetCalendarDay(r.Context(), actor, CalendarDay{
		Tanggal: chi.URLParam(r, "tanggal"), HariKerja: in.HariKerja, Keterangan: in.Keterangan,
	}, meta)
	if err != nil {
		h.writeErr(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, d)
}

func (h *Handler) deleteCalendarDay(w http.ResponseWriter, r *http.Request) {
	actor, meta := actorAndMeta(r)
	if err := h.svc.DeleteCalendarDay(r.Context(), actor, chi.URLParam(r, "tanggal"), meta); err != nil {
		h.writeErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) importCalendar(w http.ResponseWriter, r *http.Request) {
	var in struct {
		CSV   string `json:"csv"`
		Ganti bool   `json:"ganti"`
	}
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "bad_request", "Permintaan tidak valid atau berkas melebihi 1 MB.")
		return
	}
	actor, meta := actorAndMeta(r)
	res, err := h.svc.ImportCalendar(r.Context(), actor, []byte(in.CSV), in.Ganti, meta)
	if err != nil {
		h.writeErr(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, res)
}
