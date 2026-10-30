# JIEP Work Management (JWM)

Aplikasi manajemen proyek internal PT JIEP.

- Aturan kerja untuk Claude Code: `CLAUDE.md`
- Spesifikasi lengkap: `docs/SPEC.md`
- Kontrak API: `docs/openapi.yaml`

## Mulai cepat (Windows, PowerShell)

Prasyarat: Go, Node.js, dan PostgreSQL 18.

1. Salin `.env.example` menjadi `.env`, lalu isi nilainya. Untuk pengembangan
   lokal lewat `http`, isi `SESSION_COOKIE_SECURE=false`.
2. Buat database pengembangan, lalu terapkan migrasi:
   `.\scripts\migrate.ps1`
3. Buat Admin pertama (sekali saja), lalu kosongkan `BOOTSTRAP_ADMIN_PASSWORD`:
   `.\scripts\seed.ps1`
4. Jalankan API dan frontend: `.\scripts\dev.ps1`, lalu buka
   http://localhost:5173. Admin wajib mengganti sandi saat login pertama.

Untuk menjalankan satu server saja: `.\scripts\build.ps1`, lalu
`bin\jwm-api.exe` menyajikan API dan frontend di alamat `HTTP_ADDR`.
