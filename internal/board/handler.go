package board

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"jwm/internal/auth"
	"jwm/internal/httpx"
	"jwm/internal/project"
)

// Handler menyediakan endpoint board, kolom, grup, dan item.
type Handler struct {
	svc *Service
	log *slog.Logger
}

// NewHandler membuat handler board.
func NewHandler(svc *Service, log *slog.Logger) *Handler {
	return &Handler{svc: svc, log: log}
}

// ProjectRoutes dipasang di /api/v1/projects/{projectId}/boards.
func (h *Handler) ProjectRoutes(r chi.Router) {
	r.Get("/", h.listBoards)
	r.Post("/", h.createBoard)
}

// Routes dipasang di /api/v1.
func (h *Handler) Routes(r chi.Router) {
	r.Route("/boards/{id}", func(r chi.Router) {
		r.Get("/", h.getBoard)
		r.Patch("/", h.updateBoard)
		r.Delete("/", h.deleteBoard)
		r.Post("/columns", h.createColumn)
		r.Post("/groups", h.createGroup)
		r.Get("/items", h.listItems)
		r.Post("/items", h.createItem)
	})
	r.Patch("/columns/{id}", h.updateColumn)
	r.Delete("/columns/{id}", h.deleteColumn)
	r.Patch("/groups/{id}", h.updateGroup)
	r.Delete("/groups/{id}", h.deleteGroup)
	r.Route("/items/{id}", func(r chi.Router) {
		r.Get("/", h.getItem)
		r.Patch("/", h.updateItem)
		r.Delete("/", h.deleteItem)
		r.Post("/move", h.moveItem)
		r.Put("/values/{columnId}", h.setValue)
	})
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

func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := httpx.DecodeJSON(w, r, dst); err != nil {
		badRequest(w)
		return false
	}
	return true
}

func (h *Handler) writeErr(w http.ResponseWriter, r *http.Request, err error) {
	status, code := http.StatusInternalServerError, "internal"
	switch {
	case errors.Is(err, ErrNotFound), errors.Is(err, project.ErrNotFound):
		status, code = http.StatusNotFound, "not_found"
	case errors.Is(err, project.ErrForbidden):
		status, code = http.StatusForbidden, "forbidden"
	case errors.Is(err, project.ErrArchived):
		status, code = http.StatusConflict, "project_archived"
	case errors.Is(err, ErrGroupNotEmpty):
		status, code = http.StatusConflict, "group_not_empty"
	case errors.Is(err, ErrLastGroup):
		status, code = http.StatusConflict, "last_group"
	case errors.Is(err, ErrLabelInUse):
		status, code = http.StatusConflict, "label_in_use"
	case errors.Is(err, ErrReadOnlyColumn):
		status, code = http.StatusUnprocessableEntity, "read_only_column"
	case errors.Is(err, ErrInvalidName), errors.Is(err, ErrInvalidOrder), errors.Is(err, ErrInvalidColor),
		errors.Is(err, ErrInvalidColumnType), errors.Is(err, ErrInvalidConfig), errors.Is(err, ErrInvalidValue),
		errors.Is(err, ErrWrongBoard):
		status, code = http.StatusUnprocessableEntity, "validation"
	}
	if status == http.StatusInternalServerError {
		h.log.ErrorContext(r.Context(), "board", "err", err)
		httpx.WriteError(w, status, code, "Terjadi kesalahan internal.")
		return
	}
	httpx.WriteError(w, status, code, err.Error())
}

func (h *Handler) listBoards(w http.ResponseWriter, r *http.Request) {
	projectID, ok := uuidParam(w, r, "id")
	if !ok {
		return
	}
	actor, _ := actorAndMeta(r)
	items, err := h.svc.ListBoards(r.Context(), actor, projectID)
	if err != nil {
		h.writeErr(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) createBoard(w http.ResponseWriter, r *http.Request) {
	projectID, ok := uuidParam(w, r, "id")
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
	b, err := h.svc.CreateBoard(r.Context(), actor, projectID, in.Nama, meta)
	if err != nil {
		h.writeErr(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, b)
}

func (h *Handler) getBoard(w http.ResponseWriter, r *http.Request) {
	id, ok := uuidParam(w, r, "id")
	if !ok {
		return
	}
	actor, _ := actorAndMeta(r)
	b, err := h.svc.GetBoard(r.Context(), actor, id)
	if err != nil {
		h.writeErr(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, b)
}

func (h *Handler) updateBoard(w http.ResponseWriter, r *http.Request) {
	id, ok := uuidParam(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		Nama   httpx.Optional[string]  `json:"nama"`
		Urutan httpx.Optional[float64] `json:"urutan"`
	}
	if !decode(w, r, &in) {
		return
	}
	actor, meta := actorAndMeta(r)
	b, err := h.svc.UpdateBoard(r.Context(), actor, id, UpdateBoardInput(in), meta)
	if err != nil {
		h.writeErr(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, b)
}

func (h *Handler) deleteBoard(w http.ResponseWriter, r *http.Request) {
	id, ok := uuidParam(w, r, "id")
	if !ok {
		return
	}
	actor, meta := actorAndMeta(r)
	if err := h.svc.DeleteBoard(r.Context(), actor, id, meta); err != nil {
		h.writeErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) createColumn(w http.ResponseWriter, r *http.Request) {
	boardID, ok := uuidParam(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		Tipe        ColumnType      `json:"tipe"`
		Judul       string          `json:"judul"`
		Konfigurasi json.RawMessage `json:"konfigurasi"`
	}
	if !decode(w, r, &in) {
		return
	}
	actor, meta := actorAndMeta(r)
	c, err := h.svc.CreateColumn(r.Context(), actor, boardID, in.Tipe, in.Judul, in.Konfigurasi, meta)
	if err != nil {
		h.writeErr(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, c)
}

func (h *Handler) updateColumn(w http.ResponseWriter, r *http.Request) {
	id, ok := uuidParam(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		Judul       httpx.Optional[string]          `json:"judul"`
		Urutan      httpx.Optional[float64]         `json:"urutan"`
		Konfigurasi httpx.Optional[json.RawMessage] `json:"konfigurasi"`
	}
	if !decode(w, r, &in) {
		return
	}
	actor, meta := actorAndMeta(r)
	c, err := h.svc.UpdateColumn(r.Context(), actor, id, UpdateColumnInput(in), meta)
	if err != nil {
		h.writeErr(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, c)
}

func (h *Handler) deleteColumn(w http.ResponseWriter, r *http.Request) {
	id, ok := uuidParam(w, r, "id")
	if !ok {
		return
	}
	actor, meta := actorAndMeta(r)
	if err := h.svc.DeleteColumn(r.Context(), actor, id, meta); err != nil {
		h.writeErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) createGroup(w http.ResponseWriter, r *http.Request) {
	boardID, ok := uuidParam(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		Judul string `json:"judul"`
		Warna string `json:"warna"`
	}
	if !decode(w, r, &in) {
		return
	}
	actor, meta := actorAndMeta(r)
	g, err := h.svc.CreateGroup(r.Context(), actor, boardID, in.Judul, in.Warna, meta)
	if err != nil {
		h.writeErr(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, g)
}

func (h *Handler) updateGroup(w http.ResponseWriter, r *http.Request) {
	id, ok := uuidParam(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		Judul  httpx.Optional[string]  `json:"judul"`
		Warna  httpx.Optional[string]  `json:"warna"`
		Urutan httpx.Optional[float64] `json:"urutan"`
	}
	if !decode(w, r, &in) {
		return
	}
	actor, meta := actorAndMeta(r)
	g, err := h.svc.UpdateGroup(r.Context(), actor, id, UpdateGroupInput(in), meta)
	if err != nil {
		h.writeErr(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, g)
}

func (h *Handler) deleteGroup(w http.ResponseWriter, r *http.Request) {
	id, ok := uuidParam(w, r, "id")
	if !ok {
		return
	}
	actor, meta := actorAndMeta(r)
	if err := h.svc.DeleteGroup(r.Context(), actor, id, meta); err != nil {
		h.writeErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func optionalUUID(w http.ResponseWriter, v string) (*uuid.UUID, bool) {
	if v == "" {
		return nil, true
	}
	id, err := uuid.Parse(v)
	if err != nil {
		badRequest(w)
		return nil, false
	}
	return &id, true
}

func (h *Handler) listItems(w http.ResponseWriter, r *http.Request) {
	boardID, ok := uuidParam(w, r, "id")
	if !ok {
		return
	}
	qs := r.URL.Query()
	var f ItemFilter
	if f.GroupID, ok = optionalUUID(w, qs.Get("group_id")); !ok {
		return
	}
	if f.Setelah, ok = optionalUUID(w, qs.Get("kursor")); !ok {
		return
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
	page, err := h.svc.ListItems(r.Context(), actor, boardID, f)
	if err != nil {
		h.writeErr(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, page)
}

func (h *Handler) createItem(w http.ResponseWriter, r *http.Request) {
	boardID, ok := uuidParam(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		GroupID uuid.UUID `json:"group_id"`
		Judul   string    `json:"judul"`
	}
	if !decode(w, r, &in) {
		return
	}
	actor, meta := actorAndMeta(r)
	it, err := h.svc.CreateItem(r.Context(), actor, boardID, in.GroupID, in.Judul, meta)
	if err != nil {
		h.writeErr(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, it)
}

func (h *Handler) getItem(w http.ResponseWriter, r *http.Request) {
	id, ok := uuidParam(w, r, "id")
	if !ok {
		return
	}
	actor, _ := actorAndMeta(r)
	it, err := h.svc.GetItem(r.Context(), actor, id)
	if err != nil {
		h.writeErr(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, it)
}

func (h *Handler) updateItem(w http.ResponseWriter, r *http.Request) {
	id, ok := uuidParam(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		Judul string `json:"judul"`
	}
	if !decode(w, r, &in) {
		return
	}
	actor, meta := actorAndMeta(r)
	it, err := h.svc.UpdateItemTitle(r.Context(), actor, id, in.Judul, meta)
	if err != nil {
		h.writeErr(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, it)
}

func (h *Handler) moveItem(w http.ResponseWriter, r *http.Request) {
	id, ok := uuidParam(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		GroupID       uuid.UUID  `json:"group_id"`
		SetelahItemID *uuid.UUID `json:"setelah_item_id"`
	}
	if !decode(w, r, &in) {
		return
	}
	actor, meta := actorAndMeta(r)
	it, err := h.svc.MoveItem(r.Context(), actor, id, in.GroupID, in.SetelahItemID, meta)
	if err != nil {
		h.writeErr(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, it)
}

func (h *Handler) setValue(w http.ResponseWriter, r *http.Request) {
	id, ok := uuidParam(w, r, "id")
	if !ok {
		return
	}
	columnID, ok := uuidParam(w, r, "columnId")
	if !ok {
		return
	}
	var in struct {
		Nilai json.RawMessage `json:"nilai"`
	}
	if !decode(w, r, &in) {
		return
	}
	actor, meta := actorAndMeta(r)
	it, err := h.svc.SetValue(r.Context(), actor, id, columnID, in.Nilai, meta)
	if err != nil {
		h.writeErr(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, it)
}

func (h *Handler) deleteItem(w http.ResponseWriter, r *http.Request) {
	id, ok := uuidParam(w, r, "id")
	if !ok {
		return
	}
	actor, meta := actorAndMeta(r)
	if err := h.svc.DeleteItem(r.Context(), actor, id, meta); err != nil {
		h.writeErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
