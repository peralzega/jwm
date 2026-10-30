package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSPAHandler(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html>app</html>"), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "assets"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "assets", "app-abc.js"), []byte("js"), 0o600))
	require.True(t, webDirReady(dir))
	assert.False(t, webDirReady(t.TempDir()))
	assert.False(t, webDirReady(""))

	srv := httptest.NewServer(spaHandler(dir))
	t.Cleanup(srv.Close)

	tests := []struct {
		name      string
		method    string
		path      string
		status    int
		body      string
		cacheCtrl string
	}{
		{"akar", http.MethodGet, "/", http.StatusOK, "<html>app</html>", "no-cache"},
		{"rute frontend", http.MethodGet, "/proyek/123", http.StatusOK, "<html>app</html>", "no-cache"},
		{"aset berhash", http.MethodGet, "/assets/app-abc.js", http.StatusOK, "js", "public, max-age=31536000, immutable"},
		{"aset hilang", http.MethodGet, "/assets/tidak-ada.js", http.StatusNotFound, "", ""},
		{"traversal ditolak", http.MethodGet, "/../../etc/passwd", http.StatusBadRequest, "", ""},
		{"POST ditolak", http.MethodPost, "/", http.StatusMethodNotAllowed, "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest(tt.method, srv.URL+tt.path, nil)
			require.NoError(t, err)
			res, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			defer res.Body.Close()
			b, err := io.ReadAll(res.Body)
			require.NoError(t, err)
			assert.Equal(t, tt.status, res.StatusCode)
			if tt.body != "" {
				assert.Equal(t, tt.body, string(b))
			}
			if tt.cacheCtrl != "" {
				assert.Equal(t, tt.cacheCtrl, res.Header.Get("Cache-Control"))
			}
		})
	}
}
