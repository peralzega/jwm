package server

import (
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// spaHandler menyajikan hasil build frontend dari dir. Berkas yang ada
// disajikan apa adanya; jalur lain dijawab index.html agar router frontend
// yang menangani. Aset Vite di /assets/ berhash, jadi boleh di-cache lama.
func spaHandler(dir string) http.Handler {
	files := http.FileServer(http.Dir(dir))
	index := filepath.Join(dir, "index.html")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}
		clean := path.Clean("/" + r.URL.Path)
		if clean != "/" {
			full := filepath.Join(dir, filepath.FromSlash(strings.TrimPrefix(clean, "/")))
			if info, err := os.Stat(full); err == nil && !info.IsDir() {
				if strings.HasPrefix(clean, "/assets/") {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				}
				files.ServeHTTP(w, r)
				return
			}
			if strings.HasPrefix(clean, "/assets/") {
				http.NotFound(w, r)
				return
			}
		}
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFile(w, r, index)
	})
}

// webDirReady bernilai true bila dir berisi index.html hasil build.
func webDirReady(dir string) bool {
	if dir == "" {
		return false
	}
	info, err := os.Stat(filepath.Join(dir, "index.html"))
	return err == nil && !info.IsDir()
}
