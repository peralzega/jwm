# ADR 0001 — Sesi, CSRF, dan penguncian login

Status: diusulkan (M1a). Wajib di-review manusia (area auth).

## Konteks

SPEC 2A dan 7A: akun lokal, tanpa SSO/TOTP, aplikasi terbuka ke internet.
Satu instance aplikasi dan satu PostgreSQL (SPEC 7).

## Keputusan

**Sesi.** Token acak 256-bit (base64url) di cookie `__Host-jwm_session`
(`HttpOnly`, `Secure`, `SameSite=Lax`, `Path=/`). Database hanya menyimpan
SHA-256 token. Ada dua batas waktu: idle (`SESSION_IDLE_TTL`, diperpanjang
paling sering sekali per menit) dan mutlak (`SESSION_ABSOLUTE_TTL`). Sesi
milik akun nonaktif langsung tidak berlaku karena query sesi memeriksa
`users.aktif`. Selain itu, sesinya juga dihapus.

**CSRF, dua lapis.**
1. `http.CrossOriginProtection` (stdlib Go) menolak permintaan pengubah data
   lintas origin berdasarkan `Sec-Fetch-Site`/`Origin`. Lapis ini juga
   melindungi `/auth/login`.
2. Token sinkron di header `X-CSRF-Token` untuk semua metode selain
   GET/HEAD/OPTIONS pada rute terautentikasi. Token = SHA-256 dari
   `"jwm-csrf-v1:" + token sesi`. Token tidak disimpan di database. Klien
   mendapatkannya dari respons `login`, `password`, atau `GET /auth/me`.
   Penyerang tidak dapat menghitungnya tanpa cookie sesi yang `HttpOnly`.

**Penguncian.** Setiap kegagalan menambah `gagal_login`. Pada setiap
kelipatan `LOGIN_MAX_FAILURES`, akun dikunci selama
`LOGIN_LOCKOUT_BASE × 2^(k-1)`, dengan batas atas `LOGIN_LOCKOUT_MAX`.
Selama terkunci, percobaan ditolak tanpa menambah hitungan, sehingga orang
lain tidak bisa memperpanjang kunci tanpa batas (SPEC 7A). Login berhasil
mereset hitungan. Selain itu ada token bucket per IP di memori untuk
`/auth/login` dan `/auth/password`.

**Respons seragam.** Email tidak dikenal, sandi salah, akun nonaktif, dan
akun terkunci memberi respons 401 yang identik. Waktu respons disamakan
dengan verifikasi argon2id terhadap hash palsu.

**IP klien.** `X-Forwarded-For` hanya dipercaya bila koneksi langsung datang
dari `TRUSTED_PROXIES`. Rantainya dibaca dari kanan.

## Konsekuensi

- Limiter per IP hilang saat restart dan tidak dibagi antar-instance. Ini
  dapat diterima untuk v1; reverse proxy tetap membatasi per IP.
- Frontend wajib menyimpan token CSRF di memori dan memanggil `/auth/me`
  setelah halaman dimuat ulang.
