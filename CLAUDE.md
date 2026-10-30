# CLAUDE.md — JIEP Work Management (nama kerja: JWM)

Aplikasi manajemen proyek internal PT JIEP. Fungsi setara monday.com untuk
lingkup PMO & delivery. Tampilan padat (compact). Bukan klon visual.

Spesifikasi lengkap: `docs/SPEC.md`. Baca sebelum mengerjakan modul apa pun.

## Aturan kerja

- Kerjakan SATU modul per sesi. Ikuti urutan fase di `docs/SPEC.md`.
- Mulai dengan rencana singkat. Tunggu persetujuan sebelum menulis kode besar.
- Setiap perubahan wajib disertai tes. Jalankan tes sebelum menyatakan selesai.
- Jangan menambah dependensi baru tanpa menyebut alasan dan lisensinya.
- Jangan menyalin aset, logo, nama, atau desain persis milik monday.com.
- Jika spesifikasi ambigu, tanyakan. Jangan menebak.
- Tandai asumsi yang belum diverifikasi dengan komentar `// ASSUMPTION:`.

## Stack (asumsi awal, dapat direvisi)

- Backend: Go 1.23+, Chi (router), pgx, sqlc, goose (migrasi).
- Database: PostgreSQL 18 (versi mayor dev dan produksi harus sama).
- Realtime: Server-Sent Events (SSE). Pindah ke WebSocket hanya bila perlu.
- Frontend: React + TypeScript + Vite, Tailwind, TanStack Table,
  TanStack Virtual, dnd-kit (drag-and-drop).
- Gantt: frappe-gantt (MIT). Pustaka ini tidak menghitung jalur kritis;
  hitung di backend dan kirim penandanya lewat API.
  Dilarang memakai pustaka berlisensi GPL atau komersial tanpa persetujuan.
  Verifikasi lisensi pada repositori resmi saat instalasi.
- Pengujian: `go test`, testify, testcontainers-go; Vitest + Testing Library.

## Struktur repositori

```
/cmd/api            entrypoint server
/internal/auth      autentikasi, sesi, RBAC
/internal/project   proyek, portofolio
/internal/board     board, grup, item, kolom
/internal/schedule  dependensi, CPM, kalender kerja
/internal/automation mesin aturan, penjadwal
/internal/report    status reporter, risk analyzer
/internal/db        query sqlc, migrasi
/web                frontend React
/docs               SPEC.md, ADR
```

## Konvensi Go

- Layer: handler → service → repository. Handler tanpa logika bisnis.
- Error dibungkus `fmt.Errorf("...: %w", err)`. Jangan menelan error.
- `context.Context` argumen pertama di semua fungsi I/O.
- Logging terstruktur dengan `log/slog`.
- Semua query lewat sqlc. Dilarang konkatenasi string SQL.
- Tes tabel (table-driven). Algoritma CPM wajib berisi tes kasus tepi.
- Format: `gofmt` dan `golangci-lint` harus bersih.

## Konvensi frontend

- TypeScript `strict`. Dilarang `any` tanpa komentar alasan.
- Komponen kecil, satu tanggung jawab. State server lewat TanStack Query.
- Kepadatan diatur token desain (`compact` default, `normal` opsional).
- Tinggi baris grid default 32 px. Font dasar 13 px. Gunakan token, bukan angka mentah.
  Semua ukuran (jarak, ikon, avatar, panel, garis, tata letak) ditulis lewat token
  di `web/src/index.css`, mis. `gap-(--space-sm)`, `px-d`, `size-(--avatar-size)`,
  `ring-(length:--focus-ring-w)`. Kelas berangka tetap (`gap-2`, `text-lg`,
  `-ml-px`, `text-[0.7rem]`) ditolak oleh `web/src/tokens.test.ts`. Token baru
  didefinisikan untuk `compact` dan, bila berbeda, `normal`.
- Grid wajib memakai virtualisasi baris.
- Aksesibilitas dasar: fokus keyboard dan label ARIA pada kontrol interaktif.

## Keamanan

- Autentikasi lokal: akun dan kredensial di PostgreSQL. Tanpa SSO dan tanpa
  "Sign in with Google". Rincian di `docs/SPEC.md` bagian 2A.
- Hash kata sandi dengan argon2id (`golang.org/x/crypto/argon2`). Dilarang
  menyimpan kata sandi sebagai teks biasa, atau memakai MD5/SHA sederhana.
- Sesi di sisi server (tabel `sessions`). Token acak 256-bit; simpan hanya hash-nya.
  Cookie `HttpOnly`, `Secure`, `SameSite=Lax`. Terapkan perlindungan CSRF.
- Wajib ada pembatasan laju dan penguncian sementara pada endpoint login dan reset.
- Pesan galat login tidak membedakan "email tidak ada" dan "sandi salah".
- Jangan mencatat kata sandi atau token di log.
- Tidak ada pendaftaran mandiri. Hanya Admin membuat akun. Peran default Viewer.
- Hanya Admin mengubah peran. Jangan pernah menurunkan atau menonaktifkan Admin aktif terakhir.
- Menonaktifkan akun atau mengganti sandi harus mencabut semua sesi aktif.
- Email notifikasi lewat SMTP Google Workspace. Itu satu-satunya peran Google.
- Aplikasi terbuka dari internet (lihat `docs/SPEC.md` bagian 7A). Anggap
  semua input tidak tepercaya.
- Pasang header keamanan (CSP, frame-ancestors, nosniff, Referrer-Policy)
  di middleware. Cookie hanya lewat HTTPS.
- Respons login dan reset sandi seragam. Jangan bocorkan keberadaan akun.
- Endpoint admin dapat dibatasi per jaringan lewat konfigurasi
  (`ADMIN_ALLOWED_CIDRS`). Default: tidak dibatasi, tetapi harus mudah diaktifkan.
- Jangan membuka port Go atau PostgreSQL ke luar container. Hanya proxy yang publik.
- CI wajib menjalankan `govulncheck`. Jangan merge bila ada temuan tinggi.
- Otorisasi diperiksa di service layer, bukan hanya di UI.
- Semua endpoint butuh autentikasi, kecuali login dan health check.
- Validasi input di batas sistem. Parameter selalu berikat (bound).
- Rahasia hanya lewat variabel lingkungan. Jangan commit `.env`.
- Catat audit untuk perubahan status, penugasan, dan hak akses.
- Hasil kerja Claude Code di area auth dan RBAC wajib di-review manusia.

## Lingkungan pengembangan (Windows)

- Pengembang memakai Windows 10/11 native dengan PowerShell. Jangan membuat
  skrip yang hanya jalan di bash. Skrip pembantu ditulis sebagai `.ps1` di `scripts/`.
- Gunakan `filepath.Join`, bukan penggabungan string untuk jalur berkas.
- Line ending repositori adalah LF (lihat `.gitattributes`).
- `make` tidak tersedia bawaan di Windows. Perintah dijalankan lewat skrip
  `scripts/*.ps1`. Skrip memuat `.env` otomatis (`scripts/env.ps1`).
- sqlc, goose, dan govulncheck dipasang sebagai direktif `tool` di `go.mod`
  dan dipanggil dengan `go tool <nama>`. Tidak perlu instalasi global.
- Pesan commit selalu lewat berkas: `git commit -F <berkas>`. Windows PowerShell
  5.1 memecah argumen program native yang berisi tanda kutip ganda, sehingga
  `git commit -m "..."` dengan pesan multibaris bisa gagal atau terpotong.

## Perintah umum

```
.\scripts\dev.ps1               # api (:8080) + Vite (http://localhost:5173, proxy /api)
.\scripts\build.ps1             # web/dist + bin/jwm-api.exe (api menyajikan web/dist)
.\scripts\test.ps1              # go test ./... dan vitest
.\scripts\lint.ps1              # gofmt, go vet, golangci-lint, govulncheck, tsc, eslint
.\scripts\migrate.ps1 [up|down|status]   # migrasi goose
.\scripts\sqlc.ps1              # hasilkan kode query
.\scripts\seed.ps1              # buat Admin pertama
.\scripts\seed-demo.ps1 -Items 5000   # proyek demo untuk uji beban (ditolak di production)
```

`web/go.mod` sengaja ada agar `go ... ./...` tidak memindai `web/node_modules`.

Tes integrasi butuh `TEST_DATABASE_URL` (PostgreSQL 18 lokal) atau Docker.
Tanpa keduanya tes integrasi di-skip; jangan menyatakan selesai dalam kondisi itu.

## Definition of Done

1. Fitur sesuai kriteria penerimaan di SPEC.
2. Tes lulus. Lint bersih.
3. Migrasi dapat dimundurkan (down) tanpa error.
4. Dokumentasi singkat diperbarui bila kontrak API berubah.
