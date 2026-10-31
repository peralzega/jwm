package project

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"jwm/internal/auth"
	"jwm/internal/httpx"
)

// Handler menyediakan endpoint /api/v1/projects.
type Handler struct {
	svc *Service
	log *slog.Logger
}

// NewHandler membuat handler proyek.
func NewHandler(svc *Service, log *slog.Logger) *Handler {
	return &Handler{svc: svc, log: log}
}

// Routes dipasang di /api/v1/projects.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/", h.list)
	r.Post("/", h.create)
	r.Route("/{id}", func(r chi.Router) {
		r.Get("/", h.get)
		r.Patch("/", h.update)
		r.Post("/archive", h.setArchived(true))
		r.Post("/unarchive", h.setArchived(false))
		r.Get("/members", h.listMembers)
		r.Post("/members", h.addMember)
		r.Patch("/members/{userId}", h.updateMember)
		r.Delete("/members/{userId}", h.removeMember)
		r.Put("/status-manual", h.setManualStatus)
		r.Delete("/status-manual", h.clearManualStatus)
	})
}

// PortfolioRoutes dipasang di /api/v1/portfolios.
func (h *Handler) PortfolioRoutes(r chi.Router) {
	r.Get("/", h.listPortfolios)
	r.Post("/", h.createPortfolio)
	r.Patch("/{id}", h.renamePortfolio)
	r.Delete("/{id}", h.deletePortfolio)
}

func (h *Handler) listPortfolios(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.ListPortfolios(r.Context())
	if err != nil {
		h.writeErr(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": out})
}

func (h *Handler) createPortfolio(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Nama string `json:"nama"`
	}
	if !decode(w, r, &in) {
		return
	}
	actor, meta := actorAndMeta(r)
	p, err := h.svc.CreatePortfolio(r.Context(), actor, in.Nama, meta)
	if err != nil {
		h.writeErr(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, p)
}

func (h *Handler) renamePortfolio(w http.ResponseWriter, r *http.Request) {
	id, ok := uuidParam(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		Nama string `json:"nama"`
	}
	if !decode(w, r, &in) {
		return
	}
	actor, meta := actorAndMeta(r)
	p, err := h.svc.RenamePortfolio(r.Context(), actor, id, in.Nama, meta)
	if err != nil {
		h.writeErr(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, p)
}

func (h *Handler) deletePortfolio(w http.ResponseWriter, r *http.Request) {
	id, ok := uuidParam(w, r, "id")
	if !ok {
		return
	}
	actor, meta := actorAndMeta(r)
	if err := h.svc.DeletePortfolio(r.Context(), actor, id, meta); err != nil {
		h.writeErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) setManualStatus(w http.ResponseWriter, r *http.Request) {
	id, ok := uuidParam(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		Status string `json:"status"`
		Alasan string `json:"alasan"`
	}
	if !decode(w, r, &in) {
		return
	}
	actor, meta := actorAndMeta(r)
	if err := h.svc.SetManualStatus(r.Context(), actor, id, in.Status, in.Alasan, meta); err != nil {
		h.writeErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) clearManualStatus(w http.ResponseWriter, r *http.Request) {
	id, ok := uuidParam(w, r, "id")
	if !ok {
		return
	}
	actor, meta := actorAndMeta(r)
	if err := h.svc.ClearManualStatus(r.Context(), actor, id, meta); err != nil {
		h.writeErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func actorAndMeta(r *http.Request) (auth.Principal, auth.RequestMeta) {
	p, _ := auth.PrincipalFrom(r.Context())
	return p, auth.RequestMeta{IP: httpx.IPFrom(r.Context()), UserAgent: r.UserAgent()}
}

func uuidParam(w http.ResponseWriter, r *http.Request, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, name))
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "bad_request", "ID tidak valid.")
		return uuid.Nil, false
	}
	return id, true
}

func badRequest(w http.ResponseWriter) {
	httpx.WriteError(w, http.StatusBadRequest, "bad_request", "Permintaan tidak valid.")
}

func (h *Handler) writeErr(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, "not_found", err.Error())
	case errors.Is(err, ErrPortfolioNotFound):
		httpx.WriteError(w, http.StatusNotFound, "not_found", err.Error())
	case errors.Is(err, ErrPortfolioExists):
		httpx.WriteError(w, http.StatusConflict, "portfolio_exists", err.Error())
	case errors.Is(err, ErrPortfolioName), errors.Is(err, ErrInvalidPortfolio), errors.Is(err, ErrInvalidHealthStatus),
		errors.Is(err, ErrInvalidReason):
		httpx.WriteError(w, http.StatusUnprocessableEntity, "validation", err.Error())
	case errors.Is(err, ErrMemberNotFound):
		httpx.WriteError(w, http.StatusNotFound, "member_not_found", err.Error())
	case errors.Is(err, ErrForbidden):
		httpx.WriteError(w, http.StatusForbidden, "forbidden", err.Error())
	case errors.Is(err, ErrArchived):
		httpx.WriteError(w, http.StatusConflict, "project_archived", err.Error())
	case errors.Is(err, ErrAlreadyMember):
		httpx.WriteError(w, http.StatusConflict, "already_member", err.Error())
	case errors.Is(err, ErrLastManager):
		httpx.WriteError(w, http.StatusConflict, "last_manager", err.Error())
	case errors.Is(err, ErrOwnerMember):
		httpx.WriteError(w, http.StatusConflict, "owner_member", err.Error())
	case errors.Is(err, ErrRoleExceedsGlobal):
		httpx.WriteError(w, http.StatusUnprocessableEntity, "role_exceeds_global", err.Error())
	case errors.Is(err, ErrInvalidName), errors.Is(err, ErrInvalidDates), errors.Is(err, ErrInvalidPriority),
		errors.Is(err, ErrInvalidProjectRole), errors.Is(err, ErrInvalidOwner), errors.Is(err, ErrInvalidMember):
		httpx.WriteError(w, http.StatusUnprocessableEntity, "validation", err.Error())
	default:
		h.log.ErrorContext(r.Context(), "proyek", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "Terjadi kesalahan internal.")
	}
}

// decode membaca JSON; peran proyek yang tidak dikenal dilaporkan sebagai 422.
func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := httpx.DecodeJSON(w, r, dst); err != nil {
		if errors.Is(err, ErrInvalidProjectRole) {
			httpx.WriteError(w, http.StatusUnprocessableEntity, "validation", ErrInvalidProjectRole.Error())
			return false
		}
		badRequest(w)
		return false
	}
	return true
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	arsip := false
	if v := r.URL.Query().Get("arsip"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			badRequest(w)
			return
		}
		arsip = b
	}
	actor, _ := actorAndMeta(r)
	items, err := h.svc.List(r.Context(), actor, arsip)
	if err != nil {
		h.writeErr(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Nama      string     `json:"nama"`
		Mulai     *Date      `json:"mulai"`
		Selesai   *Date      `json:"selesai"`
		Prioritas string     `json:"prioritas"`
		PemilikID *uuid.UUID `json:"pemilik_id"`
	}
	if !decode(w, r, &in) {
		return
	}
	actor, meta := actorAndMeta(r)
	p, err := h.svc.Create(r.Context(), actor, CreateInput(in), meta)
	if err != nil {
		h.writeErr(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, p)
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	id, ok := uuidParam(w, r, "id")
	if !ok {
		return
	}
	actor, _ := actorAndMeta(r)
	p, err := h.svc.Get(r.Context(), actor, id)
	if err != nil {
		h.writeErr(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, p)
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	id, ok := uuidParam(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		Nama      httpx.Optional[string]    `json:"nama"`
		Mulai     httpx.Optional[Date]      `json:"mulai"`
		Selesai   httpx.Optional[Date]      `json:"selesai"`
		Prioritas httpx.Optional[string]    `json:"prioritas"`
		PemilikID httpx.Optional[uuid.UUID] `json:"pemilik_id"`
		// Project Manager proyek boleh memilih portofolio (keputusan M5).
		PortfolioID httpx.Optional[uuid.UUID] `json:"portfolio_id"`
	}
	if !decode(w, r, &in) {
		return
	}
	actor, meta := actorAndMeta(r)
	p, err := h.svc.Update(r.Context(), actor, id, UpdateInput(in), meta)
	if err != nil {
		h.writeErr(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, p)
}

func (h *Handler) setArchived(arsip bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := uuidParam(w, r, "id")
		if !ok {
			return
		}
		actor, meta := actorAndMeta(r)
		p, err := h.svc.SetArchived(r.Context(), actor, id, arsip, meta)
		if err != nil {
			h.writeErr(w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, p)
	}
}

func (h *Handler) listMembers(w http.ResponseWriter, r *http.Request) {
	id, ok := uuidParam(w, r, "id")
	if !ok {
		return
	}
	actor, _ := actorAndMeta(r)
	items, err := h.svc.ListMembers(r.Context(), actor, id)
	if err != nil {
		h.writeErr(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

// writeMembers menjawab dengan daftar anggota terbaru setelah perubahan.
func (h *Handler) writeMembers(w http.ResponseWriter, r *http.Request, status int, projectID uuid.UUID) {
	actor, _ := actorAndMeta(r)
	items, err := h.svc.ListMembers(r.Context(), actor, projectID)
	if err != nil {
		h.writeErr(w, r, err)
		return
	}
	httpx.WriteJSON(w, status, map[string]any{"items": items})
}

func (h *Handler) addMember(w http.ResponseWriter, r *http.Request) {
	id, ok := uuidParam(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		UserID      uuid.UUID `json:"user_id"`
		PeranProyek Role      `json:"peran_proyek"`
	}
	if !decode(w, r, &in) {
		return
	}
	actor, meta := actorAndMeta(r)
	if err := h.svc.AddMember(r.Context(), actor, id, in.UserID, in.PeranProyek, meta); err != nil {
		h.writeErr(w, r, err)
		return
	}
	h.writeMembers(w, r, http.StatusCreated, id)
}

func (h *Handler) updateMember(w http.ResponseWriter, r *http.Request) {
	id, ok := uuidParam(w, r, "id")
	if !ok {
		return
	}
	userID, ok := uuidParam(w, r, "userId")
	if !ok {
		return
	}
	var in struct {
		PeranProyek Role `json:"peran_proyek"`
	}
	if !decode(w, r, &in) {
		return
	}
	actor, meta := actorAndMeta(r)
	if err := h.svc.UpdateMember(r.Context(), actor, id, userID, in.PeranProyek, meta); err != nil {
		h.writeErr(w, r, err)
		return
	}
	h.writeMembers(w, r, http.StatusOK, id)
}

func (h *Handler) removeMember(w http.ResponseWriter, r *http.Request) {
	id, ok := uuidParam(w, r, "id")
	if !ok {
		return
	}
	userID, ok := uuidParam(w, r, "userId")
	if !ok {
		return
	}
	actor, meta := actorAndMeta(r)
	if err := h.svc.RemoveMember(r.Context(), actor, id, userID, meta); err != nil {
		h.writeErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
