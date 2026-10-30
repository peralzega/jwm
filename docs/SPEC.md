# SPEC — JIEP Work Management (JWM)

Status: draf v0.1. Letakkan di `docs/SPEC.md`.
Tanda **[ASUMSI]** berarti belum diverifikasi terhadap kebutuhan JIEP.

## 1. Tujuan dan batasan

Tujuan: satu aplikasi internal untuk memantau tugas, jadwal, dependensi,
dan kesehatan portofolio proyek.

Di luar cakupan (v1):
- Aplikasi mobile native.
- Marketplace integrasi.
- Multi-tenant lintas perusahaan.
- SSO dan "Sign in with Google". Akun dikelola lokal (bagian 2A).
- Modul keuangan penuh dan integrasi ERP. Panel keuangan hanya memakai input manual.

## 2. Peran dan hak akses (RBAC)

| Peran | Hak utama |
|---|---|
| Admin | Kelola pengguna, peran, dan konfigurasi. |
| PMO | Kelola portofolio, semua proyek, dan laporan. |
| Project Manager | Kelola proyek yang ditugaskan. |
| Member | Ubah tugas yang ditugaskan dan beri komentar. |
| Viewer | Hanya baca. |

Pengelolaan peran (keputusan final):
- Peran dikelola di dalam aplikasi, bukan dari Google Groups.
- Admin menentukan peran saat membuat akun. Bila tidak dipilih, peran default Viewer.
- Hanya Admin yang dapat mengubah peran atau menonaktifkan pengguna.
- Admin pertama dibuat sekali lewat perintah seed (`cmd/seed`) dengan
  `BOOTSTRAP_ADMIN_EMAIL` dan `BOOTSTRAP_ADMIN_PASSWORD`. Kata sandi awal
  wajib diganti saat login pertama. Seed menolak berjalan bila sudah ada Admin.
- Sistem selalu menyisakan minimal satu Admin aktif. Penurunan peran Admin terakhir ditolak.
- Setiap perubahan peran masuk `audit_log`.

Keanggotaan dan akses proyek **[ASUMSI]** (usulan, perlu persetujuan):
- Admin dan PMO melihat serta mengelola semua proyek tanpa perlu menjadi anggota.
- Peran lain hanya mengakses proyek tempat mereka terdaftar di `project_members`.
- `peran_proyek` menentukan hak di dalam proyek:
  - `manager`: ubah proyek, kelola anggota, kelola board dan kolom, ubah semua item.
  - `member`: buat dan ubah item, komentar.
  - `viewer`: hanya baca.
- Peran global membatasi peran proyek. Pengguna berperan global Viewer
  tidak boleh menjadi `manager` atau `member`. Pengguna berperan global Member
  tidak boleh menjadi `manager`. Hanya Project Manager, PMO, dan Admin
  yang dapat menjadi `manager`.
- Hak efektif = irisan peran global dan peran proyek (yang paling terbatas berlaku).
- Pembuat proyek otomatis menjadi `manager` proyek tersebut.
- Proyek tidak boleh kehilangan `manager` terakhir.
- Penambahan, perubahan, dan penghapusan anggota masuk `audit_log`.

## 2A. Autentikasi (keputusan: akun lokal)

Keputusan: kredensial disimpan di PostgreSQL. Tanpa SSO dan tanpa
"Sign in with Google". Google Workspace hanya dipakai sebagai kanal email
notifikasi (SMTP).

- Admin membuat akun. Tidak ada pendaftaran mandiri.
- Login memakai email dan kata sandi.
- Hash kata sandi: argon2id. **[ASUMSI]** parameter disetel sesuai server.
- Sesi disimpan di sisi server (tabel `sessions`). Cookie `HttpOnly`,
  `Secure`, `SameSite=Lax`. Token acak 256-bit; yang disimpan hanya hash-nya.
- Perlindungan CSRF untuk semua metode pengubah data.
- Pembatasan laju login per IP dan per akun. Kunci sementara setelah
  N kali gagal berturut-turut.
- Kebijakan sandi: minimal 12 karakter, tolak sandi umum. **[ASUMSI]**
- Ganti sandi wajib pada login pertama.
- Reset sandi: token sekali pakai, masa berlaku singkat, dikirim lewat SMTP
  Google Workspace. **[ASUMSI]** Metode SMTP (relay atau app password)
  perlu dikonfirmasi dengan admin Workspace. Alternatif: reset manual oleh Admin.
- Menonaktifkan akun atau mengganti sandi mencabut semua sesi aktif.
- Audit: login berhasil dan gagal, reset sandi, perubahan peran.
- Keputusan final: tidak ada autentikasi dua faktor (TOTP) untuk semua peran,
  termasuk Admin dan PMO. Kontrol kompensasi ada di bagian 7A.
- Risiko: tanpa SSO, akun pegawai yang keluar harus dinonaktifkan manual.
  Tetapkan prosedur offboarding bersama HR.

## 3. Model data inti

```
users(id, nama, email, password_hash, peran, aktif, wajib_ganti_sandi,
      gagal_login, terkunci_hingga, login_terakhir, dibuat_pada)
sessions(id, user_id, token_hash, kedaluwarsa, ip, user_agent, dibuat_pada)
password_resets(id, user_id, token_hash, kedaluwarsa, dipakai_pada)
portfolios(id, nama)
projects(id, portfolio_id, nama, pemilik_id, mulai, selesai,
         prioritas, status_manual, anggaran)
project_members(project_id, user_id, peran_proyek, ditambahkan_pada,
                ditambahkan_oleh)                 -- PK (project_id, user_id)
                                                  -- peran_proyek: manager | member | viewer
boards(id, project_id, nama)
board_columns(id, board_id, tipe, judul, urutan, konfigurasi_json)
groups(id, board_id, judul, warna, urutan)
items(id, group_id, judul, urutan, induk_id, dibuat_oleh, dibuat_pada)
item_values(item_id, column_id, nilai_json)      -- nilai per kolom
dependencies(id, board_id, predecessor_id, successor_id, tipe, lag_hari,
             dibuat_oleh, dibuat_pada)            -- UNIQUE (predecessor_id, successor_id)
comments(id, item_id, penulis_id, isi, dibuat_pada)
automation_rules(id, board_id, pemicu_json, kondisi_json, aksi_json, aktif)
automation_runs(id, rule_id, item_id, hasil, waktu)
reports(id, project_id, dibuat_pada, isi_json)
budget_entries(id, project_id, tanggal, jenis, jumlah, keterangan,
               dibuat_oleh)                       -- jenis: anggaran | realisasi
audit_log(id, aktor_id, entitas, entitas_id, aksi, sebelum, sesudah, waktu)
work_calendar(tanggal, hari_kerja, keterangan)    -- libur nasional
```

Tipe kolom v1: `text`, `status`, `person`, `date`, `timeline`, `number`,
`progress`, `priority`, `dependency`.

Keputusan: nilai kolom disimpan sebagai JSONB per sel. **[ASUMSI]** Cukup
untuk skala internal. Tinjau ulang bila item melebihi 100 ribu.

## 4. Modul dan kriteria penerimaan

### M1. Fondasi (Fase 1)
Autentikasi lokal (bagian 2A), RBAC, proyek, board, grup, item, kolom bertipe.

Penerimaan:
- Login dengan email dan kata sandi. Hash argon2id, bukan teks biasa (diuji).
- Tidak ada pendaftaran mandiri. Hanya Admin yang membuat akun.
- Kata sandi sementara wajib diganti pada login pertama (diuji).
- Pembatasan laju dan penguncian sementara aktif setelah N kali gagal (diuji).
- Menonaktifkan akun atau mengganti sandi mencabut semua sesi aktif (diuji).
- Admin dapat mengubah peran, mereset sandi, dan menonaktifkan pengguna
  lewat halaman admin.
- Logout berjalan. Sesi kedaluwarsa sesuai konfigurasi.
- Member tanpa hak tidak dapat mengubah proyek lain (diuji).
- Item dapat dibuat, diubah, dipindah grup, dan dihapus lunak.
- Setiap perubahan tercatat di `audit_log`.

### M2. Tampilan Tabel dan Kanban (Fase 1)
Penerimaan:
- Tabel padat dengan edit langsung di sel dan header menempel.
- Virtualisasi: 5.000 baris tetap lancar saat gulir. **[ASUMSI]** target 60 fps.
- Kanban dikelompokkan menurut kolom status. Seret kartu mengubah status.
- Kolom status default: Ready to start, In progress, Waiting, Done.
- Token kepadatan `compact` dan `normal` dapat dipilih per pengguna.

### M3. Timeline dan Gantt (Fase 2)
Penerimaan:
- Item berkolom timeline tampil sebagai batang di Gantt.
- Ubah tanggal dengan menyeret batang, tersimpan ke database. Keyboard setara:
  panah menggeser, Shift+panah mengubah tanggal selesai, Enter membuka editor tanggal.
- Garis hari ini dan zoom (hari, minggu, bulan).
- Panah dependensi tergambar antar batang.
- Tabel `dependencies` dan API-nya (daftar, tambah, hapus; tipe FS):
  - dependensi melingkar ditolak dengan pesan jelas yang menyebut kedua item;
  - dependensi antar-board dan ke diri sendiri ditolak;
  - pelaku harus berhak mengubah kedua item;
  - pasangan (predecessor, successor) unik;
  - perubahan tercatat di `audit_log`.
- Di M3, menyeret batang dapat melanggar aturan FS. Panah yang dilanggar
  ditandai putus-putus merah. Pergeseran otomatis successor baru di M4.
- Gantt dibangun sendiri (SVG dan React, `web/src/board/gantt`), bukan pustaka
  pihak ketiga. frappe-gantt 1.2.2, SVAR React Gantt 2.8.2, dan DHTMLX Gantt
  Community 10.0.3 sudah diperiksa dan tidak lolos semua kriteria:
  - nama tugas lewat `innerHTML` (XSS);
  - atribut `style` inline yang melanggar CSP;
  - keyboard dan ARIA;
  - ukuran lewat token;
  - virtualisasi baris.
- Gantt tidak menghitung jalur kritis. Perhitungan CPM tetap di backend (M4).
  Frontend hanya menyorot batang yang ditandai kritis oleh API.

### M4. Dependensi dan Critical Path (Fase 2)
Penerimaan:
- Mendukung FS (utama). SS, FF, SF opsional pada fase lanjut.
- Memakai tabel dan API dependensi dari M3, termasuk penolakan siklusnya.
- Perhitungan CPM: ES, EF, LS, LF, slack, dan penanda jalur kritis.
- Durasi dihitung dalam hari kerja. Sabtu dan Minggu otomatis libur.
  **[ASUMSI]** Pola ini sesuai jam kerja JIEP.
- Hari libur lain diinput manual oleh Admin lewat halaman sederhana
  (tanggal dan keterangan) pada tabel `work_calendar`.
- Tidak ada impor libur nasional, kalender per divisi, atau shift pada v1.
- Mengubah tanggal predecessor menggeser successor sesuai aturan terjadwal.
- Tes unit mencakup: jalur ganda, lag, hari libur, grafik tak terhubung.

### M5. Portofolio (Fase 3)
Penerimaan:
- Dasbor menampilkan semua proyek: progres, prioritas, health status.
- Progres proyek = rata-rata tertimbang progres item (bobot = durasi).
- Health status otomatis (Hijau, Kuning, Merah) dari aturan di bagian 5.
- PMO dapat menimpa dengan `status_manual` disertai alasan.
- Filter menurut prioritas, pemilik, dan status.

### M6. Dashboard dan Laporan (Fase 3)
Penerimaan:
- Panel: sebaran status, beban kerja per orang, tren penyelesaian.
- Panel keuangan (prioritas rendah, dikerjakan terakhir di Fase 3):
  anggaran, realisasi, sisa per proyek.
- Data keuangan diinput manual lewat formulir `budget_entries`.
  Hanya PMO dan Project Manager proyek terkait yang boleh menginput.
- Sisa anggaran = total anggaran − total realisasi. Perubahan tercatat di `audit_log`.
- Tidak ada impor CSV atau integrasi ERP pada v1.
- Data diperbarui lewat SSE tanpa memuat ulang halaman.
- Ekspor panel ke PDF atau CSV.

### M7. Otomasi no-code (Fase 4)
Bentuk aturan: pemicu → kondisi → aksi.

Pemicu v1: status berubah, tanggal tiba, item dibuat, penugasan berubah.
Aksi v1: pindah grup, ubah kolom, kirim email, panggil webhook.
Kanal notifikasi v1 hanya email. Tidak ada Telegram atau Google Chat.

Penerimaan:
- Aturan dibuat lewat formulir, bukan kode.
- Contoh: saat status = Done, pindahkan item ke grup "Selesai".
- Setiap eksekusi tercatat di `automation_runs`.
- Pencegahan loop: satu aturan tidak memicu dirinya sendiri lebih dari N kali.
- Webhook memakai tanda tangan HMAC untuk integrasi n8n.

### M8. Status Reporter dan Risk Analyzer (Fase 4)
Penerimaan:
- Laporan status terjadwal (mingguan, harian) per proyek.
- Isi laporan: progres, tugas selesai, tugas berisiko, keputusan tertunda.
- Pengiriman hanya lewat email.
- Pengiriman memakai antrean sederhana di database dengan percobaan ulang
  bila gagal. Kegagalan tercatat dan terlihat oleh Admin.
- Pengiriman email lewat SMTP relay Google Workspace atau Gmail API.
  **[ASUMSI]** Pilihan final dan batas kuota perlu diverifikasi dengan
  administrator Workspace.
- Setiap butir risiko menyertakan alasan yang dapat ditelusuri.

## 5. Aturan risiko dan health status

Item dinyatakan **berisiko** bila salah satu terpenuhi:
1. Lewat tenggat dan status bukan Done.
2. Tenggat ≤ 3 hari kerja dan progres < 50%. **[ASUMSI]** ambang dapat diatur.
3. Tidak memiliki penanggung jawab.
4. Predecessor terlambat dan item berada di jalur kritis.
5. Slack ≤ 0 pada jalur kritis.

Health status proyek (**[ASUMSI]**, ambang dapat diatur):
- Merah: ada item kritis terlambat, atau > 25% item berisiko.
- Kuning: 10–25% item berisiko, atau progres tertinggal dari rencana.
- Hijau: di luar kondisi di atas.

## 6. API

- REST, JSON, versi di jalur: `/api/v1/...`.
- Dokumentasi OpenAPI dihasilkan dari kode atau ditulis di `docs/openapi.yaml`.
- Pagination berbasis kursor untuk daftar item.
- Pembaruan realtime: `GET /api/v1/stream` (SSE) per proyek.

## 7. Non-fungsional

- Skala pengguna: pilot 5–6 orang (tim IT JIEP), target rollout 20–50 orang
  lintas divisi.
- Desain skala: satu instance aplikasi dan satu PostgreSQL sudah cukup.
  Horizontal scaling, antrean pesan, dan cache terdistribusi tidak diperlukan pada v1.
  **[ASUMSI]** Beban 50 pengguna dengan SSE tetap ringan untuk satu server.
- Cakupan data: di bawah 100 proyek aktif dan 50 ribu item. **[ASUMSI]**
  Tinjau ulang bila melampaui angka ini.
- Rollout bertahap: pilot IT dahulu, lalu divisi lain setelah masukan pilot
  diterapkan. Peran dan kolom disiapkan agar mudah disesuaikan per divisi.
- Waktu respons p95 < 300 ms untuk operasi tabel. **[ASUMSI]**
- Pencadangan database harian dan uji pemulihan berkala.
- Log terstruktur. Metrik dasar (latensi, error rate).
- Semua perubahan skema lewat migrasi goose yang dapat dimundurkan.
- Penerapan: Docker Compose di server internal JIEP. **[ASUMSI]**

## 7A. Paparan internet

Keputusan: aplikasi dapat diakses dari internet publik. Autentikasi hanya
kata sandi (tanpa TOTP, tanpa SSO). Kombinasi ini menaikkan risiko, jadi
kontrol berikut wajib.

- HTTPS wajib (TLS 1.2 ke atas) dengan HSTS. Sertifikat otomatis lewat
  reverse proxy (Caddy, Traefik, atau nginx). **[ASUMSI]**
- Hanya reverse proxy yang terbuka ke internet. Port aplikasi Go dan
  PostgreSQL tidak boleh terjangkau dari luar.
- Header keamanan: CSP, `X-Content-Type-Options`, `frame-ancestors`,
  `Referrer-Policy`.
- Pembatasan laju di reverse proxy (per IP), selain pembatasan di aplikasi.
- Pertimbangkan WAF atau CDN di depan aplikasi. **[ASUMSI]** Belum dievaluasi.
- Penguncian akun memakai penundaan progresif agar tidak mudah dipakai
  untuk mengunci akun orang lain.
- Reset sandi dan login memberi respons seragam (cegah penebakan akun).
- Halaman dan API admin dibatasi ke jaringan tertentu (IP kantor atau VPN).
  **[ASUMSI]** Kontrol kompensasi atas ketiadaan TOTP.
- Log autentikasi (login gagal, reset, perubahan peran) dikirim ke SIEM
  bila tersedia. Beri peringatan pada lonjakan login gagal.
- CI menjalankan `govulncheck` dan `npm audit`. Perbarui dependensi berkala.
- Sebelum rollout di luar tim IT: tinjauan terhadap OWASP ASVS (level 1–2)
  atau uji penetrasi. **[ASUMSI]**
- Cadangan database disimpan di luar server aplikasi.

## 8. Peta jalan

| Fase | Isi | Hasil |
|---|---|---|
| 1 | M1, M2 | Task tracking, tabel, Kanban |
| 2 | M3, M4 | Gantt, dependensi, critical path |
| 3 | M5, M6 | Portofolio, dashboard |
| 4 | M7, M8 | Otomasi, laporan, risk analyzer |

Estimasi total: sekitar 3–5 bulan untuk satu orang dengan Claude Code.
**[ASUMSI]** Belum divalidasi.

## 9. Pertanyaan terbuka

1. (Terjawab) Data keuangan diinput manual. Tidak ada impor pada v1.
2. (Terjawab) Tanpa SSO dan tanpa "Sign in with Google". Akun lokal di
   PostgreSQL. Peran dikelola di dalam aplikasi. Google Workspace hanya untuk email.
3. (Terjawab) Pilot 5–6 orang tim IT; rollout 20–50 orang. Jumlah proyek aktif belum diketahui.
4. (Terjawab) Kanal notifikasi hanya email.
5. (Terjawab, direvisi di M3) Gantt dibangun sendiri. frappe-gantt, SVAR React
   Gantt, dan DHTMLX Gantt Community diperiksa. Ketiganya berlisensi MIT, tetapi
   tidak lolos syarat keamanan (XSS, CSP), aksesibilitas, atau token ukuran.
   Rincian di M3.
6. (Terjawab) Hari libur diinput manual oleh Admin. Tanpa fitur lanjutan.

## 10. Prompt pembuka untuk Claude Code

```
Baca CLAUDE.md dan docs/SPEC.md. Kerjakan Fase 1, modul M1 saja.
Buat rencana dulu: struktur direktori, skema migrasi, daftar endpoint.
Tunggu persetujuan saya sebelum menulis kode.
```
