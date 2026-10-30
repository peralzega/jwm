package auth

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"jwm/internal/httpx"
)

// CookieConfig mengatur cookie sesi.
type CookieConfig struct {
	Secure bool
}

// Name memakai prefiks __Host- bila Secure, agar cookie terikat ke host dan Path=/.
func (c CookieConfig) Name() string {
	if c.Secure {
		return "__Host-jwm_session"
	}
	return "jwm_session"
}

// Set menulis cookie sesi.
func (c CookieConfig) Set(w http.ResponseWriter, token string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name: c.Name(), Value: token, Path: "/", Expires: expires,
		HttpOnly: true, Secure: c.Secure, SameSite: http.SameSiteLaxMode,
	})
}

// Clear menghapus cookie sesi di peramban.
func (c CookieConfig) Clear(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: c.Name(), Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: c.Secure, SameSite: http.SameSiteLaxMode,
	})
}

// Handler menyediakan endpoint /api/v1/auth.
type Handler struct {
	svc     *Service
	cookies CookieConfig
	limiter *httpx.RateLimiter
	log     *slog.Logger
}

// NewHandler membuat handler auth. limiter dipakai untuk login dan ganti sandi per IP.
func NewHandler(svc *Service, cookies CookieConfig, limiter *httpx.RateLimiter, log *slog.Logger) *Handler {
	return &Handler{svc: svc, cookies: cookies, limiter: limiter, log: log}
}

// Routes memasang rute auth pada r (dipasang di /api/v1/auth).
func (h *Handler) Routes(r chi.Router) {
	r.With(httpx.LimitByIP(h.limiter)).Post("/login", h.login)
	r.Group(func(r chi.Router) {
		r.Use(RequireAuth(h.svc, h.cookies, h.log))
		r.Get("/me", h.me)
		r.With(RequirePasswordChanged).Patch("/me/preferensi", h.updatePreferences)
		r.Post("/logout", h.logout)
		r.With(httpx.LimitByIP(h.limiter)).Post("/password", h.changePassword)
	})
}

type sessionResponse struct {
	User      User   `json:"user"`
	CSRFToken string `json:"csrf_token"`
}

func meta(r *http.Request) RequestMeta {
	return RequestMeta{IP: httpx.IPFrom(r.Context()), UserAgent: r.UserAgent()}
}

// Batas byte masukan kredensial agar argon2 tidak dipanggil untuk masukan raksasa.
const (
	maxEmailBytes    = 254
	maxPasswordBytes = MaxPasswordLen * 4
)

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "bad_request", "Permintaan tidak valid.")
		return
	}
	if in.Email == "" || in.Password == "" || len(in.Email) > maxEmailBytes || len(in.Password) > maxPasswordBytes {
		h.writeInvalidCredentials(w)
		return
	}
	res, err := h.svc.Login(r.Context(), in.Email, in.Password, meta(r))
	if errors.Is(err, ErrInvalidCredentials) {
		h.writeInvalidCredentials(w)
		return
	}
	if err != nil {
		h.internal(w, r, "login", err)
		return
	}
	h.cookies.Set(w, res.Token, res.ExpiresAt)
	httpx.WriteJSON(w, http.StatusOK, sessionResponse{User: res.User, CSRFToken: CSRFToken(res.Token)})
}

func (h *Handler) writeInvalidCredentials(w http.ResponseWriter) {
	httpx.WriteError(w, http.StatusUnauthorized, "invalid_credentials", "Email atau kata sandi salah.")
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	p, _ := PrincipalFrom(r.Context())
	httpx.WriteJSON(w, http.StatusOK, sessionResponse{User: p.User, CSRFToken: p.CSRFToken()})
}

func (h *Handler) updatePreferences(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Kepadatan *Density `json:"kepadatan"`
	}
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "bad_request", "Permintaan tidak valid.")
		return
	}
	p, _ := PrincipalFrom(r.Context())
	u, err := h.svc.UpdatePreferences(r.Context(), p, Preferences{Kepadatan: in.Kepadatan})
	if errors.Is(err, ErrInvalidDensity) {
		httpx.WriteError(w, http.StatusUnprocessableEntity, "validation", err.Error())
		return
	}
	if err != nil {
		h.internal(w, r, "ubah preferensi", err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, sessionResponse{User: u, CSRFToken: p.CSRFToken()})
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	p, _ := PrincipalFrom(r.Context())
	if err := h.svc.Logout(r.Context(), p, meta(r)); err != nil {
		h.internal(w, r, "logout", err)
		return
	}
	h.cookies.Clear(w)
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) changePassword(w http.ResponseWriter, r *http.Request) {
	var in struct {
		SandiLama string `json:"sandi_lama"`
		SandiBaru string `json:"sandi_baru"`
	}
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "bad_request", "Permintaan tidak valid.")
		return
	}
	if len(in.SandiLama) > maxPasswordBytes || len(in.SandiBaru) > maxPasswordBytes {
		httpx.WriteError(w, http.StatusUnprocessableEntity, "password_policy", ErrPasswordTooLong.Error())
		return
	}
	p, _ := PrincipalFrom(r.Context())
	res, err := h.svc.ChangePassword(r.Context(), p, in.SandiLama, in.SandiBaru, meta(r))
	switch {
	case errors.Is(err, ErrInvalidCredentials):
		httpx.WriteError(w, http.StatusForbidden, "invalid_current_password", "Kata sandi lama salah.")
		return
	case errors.Is(err, ErrSamePassword), errors.Is(err, ErrPasswordTooShort),
		errors.Is(err, ErrPasswordTooLong), errors.Is(err, ErrPasswordCommon):
		httpx.WriteError(w, http.StatusUnprocessableEntity, "password_policy", err.Error())
		return
	case err != nil:
		h.internal(w, r, "ganti sandi", err)
		return
	}
	h.cookies.Set(w, res.Token, res.ExpiresAt)
	httpx.WriteJSON(w, http.StatusOK, sessionResponse{User: res.User, CSRFToken: CSRFToken(res.Token)})
}

func (h *Handler) internal(w http.ResponseWriter, r *http.Request, op string, err error) {
	h.log.ErrorContext(r.Context(), op, "err", err)
	httpx.WriteError(w, http.StatusInternalServerError, "internal", "Terjadi kesalahan internal.")
}
