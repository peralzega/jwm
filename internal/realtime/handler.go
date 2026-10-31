package realtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"

	"jwm/internal/auth"
	"jwm/internal/httpx"
)

// ScopeFunc mengembalikan proyek yang boleh diikuti pelaku: satu proyek bila
// project tidak nil (galat bila tidak boleh dibaca), atau semua proyek aktif
// yang terlihat.
type ScopeFunc func(ctx context.Context, actor auth.Principal, project *uuid.UUID) ([]uuid.UUID, error)

// SessionChecker memeriksa ulang sesi tanpa memperpanjang masa idle.
type SessionChecker interface {
	StillValid(ctx context.Context, p auth.Principal) error
}

// Handler menyajikan GET /api/v1/stream.
type Handler struct {
	Hub      *Hub
	Sessions SessionChecker
	Scope    ScopeFunc
	// MapErr menulis galat scope (404, 403) sesuai konvensi paket asal.
	MapErr func(w http.ResponseWriter, r *http.Request, err error)
	Log    *slog.Logger

	// KeepAlive: interval komentar ping dan pemeriksaan ulang sesi serta hak.
	KeepAlive time.Duration
	// MaxAge: umur maksimal koneksi; klien EventSource menyambung ulang sendiri.
	MaxAge time.Duration
	// Debounce: event dalam jeda ini digabung menjadi satu kiriman.
	Debounce time.Duration
}

// Bawaan durasi.
const (
	DefaultKeepAlive = 25 * time.Second
	DefaultMaxAge    = 5 * time.Minute
	DefaultDebounce  = 300 * time.Millisecond
)

func (h *Handler) durations() (time.Duration, time.Duration, time.Duration) {
	k, m, d := h.KeepAlive, h.MaxAge, h.Debounce
	if k <= 0 {
		k = DefaultKeepAlive
	}
	if m <= 0 {
		m = DefaultMaxAge
	}
	if d <= 0 {
		d = DefaultDebounce
	}
	return k, m, d
}

// ServeHTTP membuka stream event "berubah" untuk ?project=<uuid> atau semua
// proyek yang terlihat pelaku.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var pid *uuid.UUID
	if v := r.URL.Query().Get("project"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "bad_request", "Parameter project tidak valid.")
			return
		}
		pid = &id
	}
	actor, _ := auth.PrincipalFrom(r.Context())
	projects, err := h.Scope(r.Context(), actor, pid)
	if err != nil {
		h.MapErr(w, r, err)
		return
	}
	sub, err := h.Hub.Subscribe(actor.User.ID, projects)
	if errors.Is(err, ErrTooManyStreams) {
		httpx.WriteError(w, http.StatusTooManyRequests, "too_many_streams", err.Error())
		return
	}
	defer sub.Close()

	// Server memakai ReadTimeout/WriteTimeout; koneksi stream dibebaskan darinya.
	rc := http.NewResponseController(w)
	for _, set := range []func(time.Time) error{rc.SetWriteDeadline, rc.SetReadDeadline} {
		if err := set(time.Time{}); err != nil && !errors.Is(err, http.ErrNotSupported) {
			h.Log.WarnContext(r.Context(), "stream: lepas tenggat", "err", err)
		}
	}
	hd := w.Header()
	hd.Set("Content-Type", "text/event-stream")
	hd.Set("Cache-Control", "no-store")
	// Nginx dan sejenisnya tidak boleh menahan (buffer) respons stream.
	hd.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	write := func(s string) bool {
		if _, err := fmt.Fprint(w, s); err != nil {
			return false
		}
		return rc.Flush() == nil
	}
	if !write("retry: 3000\n: tersambung\n\n") {
		return
	}

	keepAlive, maxAge, debounce := h.durations()
	ping := time.NewTicker(keepAlive)
	defer ping.Stop()
	deadline := time.NewTimer(maxAge)
	defer deadline.Stop()
	var flush <-chan time.Time
	pending := map[Event]bool{}
	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case <-h.Hub.Done():
			write("event: tutup\ndata: {}\n\n")
			return
		case e := <-sub.C():
			pending[e] = true
			if flush == nil {
				flush = time.After(debounce)
			}
		case <-flush:
			flush = nil
			for e := range pending {
				b, _ := json.Marshal(e)
				if !write("event: berubah\ndata: " + string(b) + "\n\n") {
					return
				}
			}
			clear(pending)
		case <-ping.C:
			// Sesi dicabut atau hak hilang: tutup koneksi; klien menyambung ulang
			// dan mendapat 401/404 dari pemeriksaan biasa.
			if err := h.Sessions.StillValid(ctx, actor); err != nil {
				return
			}
			projects, err := h.Scope(ctx, actor, pid)
			if err != nil {
				return
			}
			sub.SetProjects(projects)
			if !write(": ping\n\n") {
				return
			}
		case <-deadline.C:
			write("event: tutup\ndata: {}\n\n")
			return
		}
	}
}
