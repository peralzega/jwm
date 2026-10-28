package user

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

// Handler menyediakan endpoint /api/v1/admin/users, /admin/audit-log, dan /users.
type Handler struct {
	svc *Service
	log *slog.Logger
}

// NewHandler membuat handler pengguna.
func NewHandler(svc *Service, log *slog.Logger) *Handler {
	return &Handler{svc: svc, log: log}
}

// AdminRoutes dipasang di /api/v1/admin.
func (h *Handler) AdminRoutes(r chi.Router) {
	r.Get("/users", h.list)
	r.Post("/users", h.create)
	r.Get("/users/{id}", h.get)
	r.Patch("/users/{id}", h.update)
	r.Post("/users/{id}/deactivate", h.setActive(false))
	r.Post("/users/{id}/activate", h.setActive(true))
	r.Post("/users/{id}/reset-password", h.resetPassword)
	r.Post("/users/{id}/unlock", h.unlock)
	r.Get("/audit-log", h.auditLog)
}

// Routes dipasang di /api/v1/users (pencarian untuk semua pengguna login).
func (h *Handler) Routes(r chi.Router) {
	r.Get("/", h.search)
}

func actorAndMeta(r *http.Request) (auth.Principal, auth.RequestMeta) {
	p, _ := auth.PrincipalFrom(r.Context())
	return p, auth.RequestMeta{IP: httpx.IPFrom(r.Context()), UserAgent: r.UserAgent()}
}

func pathID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "bad_request", "ID pengguna tidak valid.")
		return uuid.Nil, false
	}
	return id, true
}

func (h *Handler) writeErr(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrForbidden):
		httpx.WriteError(w, http.StatusForbidden, "forbidden", err.Error())
	case errors.Is(err, ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, "not_found", err.Error())
	case errors.Is(err, ErrEmailTaken):
		httpx.WriteError(w, http.StatusConflict, "email_taken", err.Error())
	case errors.Is(err, ErrLastAdmin):
		httpx.WriteError(w, http.StatusConflict, "last_admin", err.Error())
	case errors.Is(err, ErrSelfAction):
		httpx.WriteError(w, http.StatusConflict, "self_action", err.Error())
	case errors.Is(err, ErrInvalidRole), errors.Is(err, ErrInvalidName), errors.Is(err, ErrInvalidEmail):
		httpx.WriteError(w, http.StatusUnprocessableEntity, "validation", err.Error())
	default:
		h.log.ErrorContext(r.Context(), "pengguna", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "Terjadi kesalahan internal.")
	}
}

func badRequest(w http.ResponseWriter) {
	httpx.WriteError(w, http.StatusBadRequest, "bad_request", "Permintaan tidak valid.")
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	actor, _ := actorAndMeta(r)
	qs := r.URL.Query()
	f := ListFilter{Q: qs.Get("q")}
	if v := qs.Get("peran"); v != "" {
		role := auth.Role(v)
		f.Peran = &role
	}
	if v := qs.Get("aktif"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			badRequest(w)
			return
		}
		f.Aktif = &b
	}
	users, err := h.svc.List(r.Context(), actor, f)
	if err != nil {
		h.writeErr(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": users})
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	actor, _ := actorAndMeta(r)
	u, err := h.svc.Get(r.Context(), actor, id)
	if err != nil {
		h.writeErr(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, u)
}

type createdResponse struct {
	User           AdminUser `json:"user"`
	SandiSementara string    `json:"sandi_sementara"`
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Nama  string    `json:"nama"`
		Email string    `json:"email"`
		Peran auth.Role `json:"peran"`
	}
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		badRequest(w)
		return
	}
	actor, meta := actorAndMeta(r)
	c, err := h.svc.Create(r.Context(), actor, CreateInput(in), meta)
	if err != nil {
		h.writeErr(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, createdResponse{User: c.User, SandiSementara: c.SandiSementara})
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in struct {
		Nama  *string    `json:"nama"`
		Email *string    `json:"email"`
		Peran *auth.Role `json:"peran"`
	}
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		badRequest(w)
		return
	}
	actor, meta := actorAndMeta(r)
	u, err := h.svc.Update(r.Context(), actor, id, UpdateInput(in), meta)
	if err != nil {
		h.writeErr(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, u)
}

func (h *Handler) setActive(aktif bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		actor, meta := actorAndMeta(r)
		u, err := h.svc.SetActive(r.Context(), actor, id, aktif, meta)
		if err != nil {
			h.writeErr(w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, u)
	}
}

func (h *Handler) resetPassword(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	actor, meta := actorAndMeta(r)
	c, err := h.svc.ResetPassword(r.Context(), actor, id, meta)
	if err != nil {
		h.writeErr(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, createdResponse{User: c.User, SandiSementara: c.SandiSementara})
}

func (h *Handler) unlock(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	actor, meta := actorAndMeta(r)
	u, err := h.svc.Unlock(r.Context(), actor, id, meta)
	if err != nil {
		h.writeErr(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, u)
}

func (h *Handler) auditLog(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	f := AuditFilter{Entitas: qs.Get("entitas"), EntitasID: qs.Get("entitas_id")}
	if v := qs.Get("aktor_id"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			badRequest(w)
			return
		}
		f.AktorID = &id
	}
	if v := qs.Get("kursor"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n <= 0 {
			badRequest(w)
			return
		}
		f.SebelumID = &n
	}
	if v := qs.Get("batas"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			badRequest(w)
			return
		}
		f.Batas = n
	}
	actor, _ := actorAndMeta(r)
	page, err := h.svc.AuditLog(r.Context(), actor, f)
	if err != nil {
		h.writeErr(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, page)
}

func (h *Handler) search(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if len(q) > 200 {
		badRequest(w)
		return
	}
	users, err := h.svc.Search(r.Context(), q)
	if err != nil {
		h.writeErr(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": users})
}
