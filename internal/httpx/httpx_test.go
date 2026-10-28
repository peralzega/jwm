package httpx

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var okHandler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })

func TestSecurityHeaders(t *testing.T) {
	rec := httptest.NewRecorder()
	SecurityHeaders(true)(okHandler).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/x", nil))
	h := rec.Header()
	assert.Contains(t, h.Get("Content-Security-Policy"), "frame-ancestors 'none'")
	assert.Equal(t, "nosniff", h.Get("X-Content-Type-Options"))
	assert.Equal(t, "no-referrer", h.Get("Referrer-Policy"))
	assert.NotEmpty(t, h.Get("Strict-Transport-Security"))
	assert.Equal(t, "no-store", h.Get("Cache-Control"))
}

func TestClientIP(t *testing.T) {
	trusted := []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}
	tests := []struct {
		name   string
		remote string
		xff    string
		want   string
	}{
		{"tanpa proxy", "203.0.113.5:1234", "", "203.0.113.5"},
		{"XFF dari sumber tak tepercaya diabaikan", "203.0.113.5:1234", "1.2.3.4", "203.0.113.5"},
		{"XFF dari proxy tepercaya", "10.0.0.2:1234", "198.51.100.7", "198.51.100.7"},
		{"spoof di kiri diabaikan", "10.0.0.2:1234", "6.6.6.6, 198.51.100.7", "198.51.100.7"},
		{"lewati proxy berantai", "10.0.0.2:1234", "198.51.100.7, 10.0.0.9", "198.51.100.7"},
		{"XFF rusak kembali ke remote", "10.0.0.2:1234", "bukan-ip", "10.0.0.2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.RemoteAddr = tt.remote
			if tt.xff != "" {
				r.Header.Set("X-Forwarded-For", tt.xff)
			}
			assert.Equal(t, tt.want, ClientIP(r, trusted).String())
		})
	}
}

func TestAllowCIDRs(t *testing.T) {
	allowed := []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}
	tests := []struct {
		name    string
		allowed []netip.Prefix
		remote  string
		want    int
	}{
		{"kosong berarti terbuka", nil, "203.0.113.5:1", http.StatusOK},
		{"di dalam jaringan", allowed, "10.1.1.1:1", http.StatusOK},
		{"di luar jaringan", allowed, "203.0.113.5:1", http.StatusForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := WithClientIP(nil)(AllowCIDRs(tt.allowed)(okHandler))
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.RemoteAddr = tt.remote
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, r)
			assert.Equal(t, tt.want, rec.Code)
		})
	}
}

func TestCrossOriginRejectsForeignPost(t *testing.T) {
	h := CrossOrigin()(okHandler)
	r := httptest.NewRequest(http.MethodPost, "http://jwm.local/api/v1/auth/login", nil)
	r.Header.Set("Sec-Fetch-Site", "cross-site")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	assert.Equal(t, http.StatusForbidden, rec.Code)

	r = httptest.NewRequest(http.MethodPost, "http://jwm.local/api/v1/auth/login", nil)
	r.Header.Set("Sec-Fetch-Site", "same-origin")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestRateLimiter(t *testing.T) {
	l := NewRateLimiter(3)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	l.now = func() time.Time { return now }

	for i := range 3 {
		ok, _ := l.Allow("a")
		require.True(t, ok, "percobaan %d", i+1)
	}
	ok, wait := l.Allow("a")
	assert.False(t, ok)
	assert.Positive(t, wait)

	ok, _ = l.Allow("b")
	assert.True(t, ok, "kunci lain tidak terpengaruh")

	now = now.Add(20 * time.Second) // 3/menit = 1 token per 20 detik
	ok, _ = l.Allow("a")
	assert.True(t, ok)
}

func TestDecodeJSON(t *testing.T) {
	type body struct {
		A string `json:"a"`
	}
	tests := []struct {
		name    string
		in      string
		wantErr bool
	}{
		{"valid", `{"a":"x"}`, false},
		{"field asing", `{"a":"x","b":1}`, true},
		{"dua nilai", `{"a":"x"}{"a":"y"}`, true},
		{"rusak", `{`, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var b body
			r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tt.in))
			err := DecodeJSON(httptest.NewRecorder(), r, &b)
			assert.Equal(t, tt.wantErr, err != nil)
		})
	}
}
