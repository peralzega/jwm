package httpx

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"runtime/debug"
	"slices"
	"strings"
	"time"
)

type ctxKey int

const clientIPKey ctxKey = iota

// SecurityHeaders memasang header keamanan pada setiap respons.
func SecurityHeaders(hsts bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			// ASSUMPTION: frontend disajikan dari origin yang sama tanpa skrip inline.
			h.Set("Content-Security-Policy",
				"default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; "+
					"connect-src 'self'; object-src 'none'; base-uri 'none'; form-action 'self'; "+
					"frame-ancestors 'none'")
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("X-Frame-Options", "DENY")
			h.Set("Referrer-Policy", "no-referrer")
			h.Set("Cross-Origin-Opener-Policy", "same-origin")
			if hsts {
				h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
			}
			if strings.HasPrefix(r.URL.Path, "/api/") {
				h.Set("Cache-Control", "no-store")
			}
			next.ServeHTTP(w, r)
		})
	}
}

// ClientIP menentukan IP klien. X-Forwarded-For hanya dipercaya bila
// koneksi langsung berasal dari proxy tepercaya. Rantai dibaca dari kanan;
// alamat pertama yang bukan proxy tepercaya dianggap klien.
func ClientIP(r *http.Request, trusted []netip.Prefix) netip.Addr {
	remote := parseAddr(r.RemoteAddr)
	if !remote.IsValid() || !inPrefixes(remote, trusted) {
		return remote
	}
	var hops []string
	for _, v := range r.Header.Values("X-Forwarded-For") {
		hops = append(hops, strings.Split(v, ",")...)
	}
	for _, hop := range slices.Backward(hops) {
		a, err := netip.ParseAddr(strings.TrimSpace(hop))
		if err != nil {
			return remote
		}
		a = a.Unmap()
		if !inPrefixes(a, trusted) {
			return a
		}
	}
	return remote
}

func parseAddr(hostport string) netip.Addr {
	host, _, err := net.SplitHostPort(hostport)
	if err != nil {
		host = hostport
	}
	a, err := netip.ParseAddr(host)
	if err != nil {
		return netip.Addr{}
	}
	return a.Unmap()
}

func inPrefixes(a netip.Addr, ps []netip.Prefix) bool {
	for _, p := range ps {
		if p.Contains(a) {
			return true
		}
	}
	return false
}

// WithClientIP menyimpan IP klien di context permintaan.
func WithClientIP(trusted []netip.Prefix) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), clientIPKey, ClientIP(r, trusted))
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// IPFrom mengambil IP klien dari context. Nilai nol bila tidak ada.
func IPFrom(ctx context.Context) netip.Addr {
	a, _ := ctx.Value(clientIPKey).(netip.Addr)
	return a
}

// AllowCIDRs membatasi akses ke jaringan tertentu. Daftar kosong berarti tidak dibatasi.
func AllowCIDRs(allowed []netip.Prefix) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if len(allowed) == 0 {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if ip := IPFrom(r.Context()); !ip.IsValid() || !inPrefixes(ip, allowed) {
				WriteError(w, http.StatusForbidden, "forbidden_network", "Akses tidak diizinkan dari jaringan ini.")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// CrossOrigin menolak permintaan pengubah data lintas origin (CSRF lapis pertama).
func CrossOrigin() func(http.Handler) http.Handler {
	cop := http.NewCrossOriginProtection()
	cop.SetDenyHandler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		WriteError(w, http.StatusForbidden, "csrf", "Permintaan lintas origin ditolak.")
	}))
	return cop.Handler
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }

// Logger mencatat setiap permintaan. Query string tidak dicatat agar token tidak bocor.
func Logger(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rec, r)
			log.LogAttrs(r.Context(), slog.LevelInfo, "http",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", rec.status),
				slog.Duration("durasi", time.Since(start)),
				slog.String("ip", IPFrom(r.Context()).String()),
			)
		})
	}
}

// Recover mengubah panic menjadi respons 500 dan mencatatnya.
func Recover(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if v := recover(); v != nil {
					if v == http.ErrAbortHandler {
						panic(v)
					}
					log.Error("panic", "nilai", v, "stack", string(debug.Stack()))
					WriteError(w, http.StatusInternalServerError, "internal", "Terjadi kesalahan internal.")
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}
