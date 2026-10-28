package server

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"jwm/internal/auth"
	"jwm/internal/config"
	"jwm/internal/testutil"
)

func TestRouter(t *testing.T) {
	pool := testutil.NewPool(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	hasher, err := auth.NewHasher(auth.Argon2Params{Memory: 1024, Time: 1, Threads: 1, SaltLen: 16, KeyLen: 32})
	require.NoError(t, err)
	cfg := config.Config{CookieSecure: true, LoginRatePerMinute: 10}
	svc := auth.NewService(pool, hasher, auth.Settings{
		SessionIdleTTL: time.Hour, SessionAbsoluteTTL: time.Hour, MaxFailures: 5,
		LockoutBase: time.Minute, LockoutMax: time.Hour,
	}, log)
	srv := httptest.NewServer(NewRouter(Deps{Config: cfg, Pool: pool, Auth: svc, Log: log}))
	t.Cleanup(srv.Close)

	tests := []struct {
		name   string
		method string
		path   string
		want   int
	}{
		{"health check terbuka", http.MethodGet, "/healthz", http.StatusOK},
		{"me butuh login", http.MethodGet, "/api/v1/auth/me", http.StatusUnauthorized},
		{"rute tak dikenal", http.MethodGet, "/api/v1/tidak-ada", http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest(tt.method, srv.URL+tt.path, nil)
			require.NoError(t, err)
			res, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			defer res.Body.Close()
			assert.Equal(t, tt.want, res.StatusCode)
			assert.Equal(t, "nosniff", res.Header.Get("X-Content-Type-Options"))
			assert.NotEmpty(t, res.Header.Get("Strict-Transport-Security"))
		})
	}
}
