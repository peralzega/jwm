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
portfolios(id, nama, dibuat_oleh, dibuat_pada, diubah_pada)   -- nama unik
projects(id, portfolio_id, nama, pemilik_id, mulai, selesai,
         prioritas, status_manual, status_manual_alasan, status_manual_oleh,
         status_manual_pada)                    -- status_manual: hijau | kuning | merah
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
budget_entries(id, project_id, tanggal, jenis, jumlah, keterangan, dibuat_oleh,
               dibuat_pada, diubah_oleh, diubah_pada)  -- jenis: anggaran | realisasi; jumlah > 0
audit_log(id, aktor_id, entitas, entitas_id, aksi, sebelum, sesudah, waktu)
work_calendar(tanggal, hari_kerja, keterangan, diubah_oleh, diubah_pada)
                                                  -- menimpa pola Senin–Jumat
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
  (tanggal dan keterangan) pada tabel `work_calendar`, atau diimpor dari CSV
  yang diisi Admin dari data resmi. Aplikasi tidak membawa daftar libur bawaan.
- Tidak ada impor otomatis dari sumber luar, kalender per divisi, atau shift pada v1.
- Mengubah tanggal predecessor menggeser successor sesuai aturan terjadwal.
- Tes unit mencakup: jalur ganda, lag, hari libur, grafik tak terhubung.

Keputusan implementasi M4:
- **Tanggal murni.** Semua tanggal adalah `DATE` atau string `YYYY-MM-DD`,
  dihitung sebagai nomor hari tanpa jam dan zona waktu (diuji pada batas bulan,
  tahun, dan tahun kabisat).
- **Kalender kerja.** Senin–Jumat adalah hari kerja. Baris `work_calendar`
  menimpa pola ini: `hari_kerja = false` untuk libur, `true` untuk akhir pekan
  yang dijadikan hari kerja. Impor CSV (Admin): header `tanggal` (wajib),
  `keterangan`, `hari_kerja` (ya/tidak); pemisah koma atau titik koma; satu baris
  salah menolak seluruh berkas dengan nomor barisnya; opsi "ganti" menyamakan
  isi tahun-tahun di berkas. `/schedule` memberi peringatan `kalender_kosong`
  untuk setiap tahun dalam rentang jadwal yang belum punya satu baris pun.
- **Durasi dan lag** dalam hari kerja. Item yang mulai pada hari libur dianggap
  mulai pada hari kerja berikutnya. Item yang seluruhnya jatuh pada hari libur
  berdurasi nol (menjadi satu hari kerja bila digeser otomatis). Lag negatif
  (lead) diizinkan, −365 sampai 365.
- **CPM.** Tanggal mulai terjadwal adalah batasan "mulai tidak lebih awal dari".
  ES = maks(mulai terjadwal, EF predecessor + lag). Selesai proyek = EF
  terbesar di board; LF dihitung mundur darinya. Slack = LS − ES (hari kerja,
  tidak pernah negatif). Kritis bila slack = 0. Dihitung saat diminta
  (`GET /boards/{id}/schedule`), tidak disimpan.
- **Melanggar FS.** Successor boleh dijadwalkan mendahului predecessor-nya
  (mis. diseret manual). Itu tidak ditolak, tetapi ditandai `melanggar_fs` pada
  item dan `dependensi_dilanggar` pada dependensi, dan disorot di Gantt. ES
  item tersebut dihitung dari predecessor, jadi berbeda dari jadwalnya.
- **Item tanpa jadwal di tengah rantai** diperlakukan sebagai titik lewat
  berdurasi nol: rantai tidak terputus, dan batasan predecessor (ditambah lag
  kedua dependensi) diteruskan ke successor. Item ini ditandai `tanpa_jadwal`.
  Item tanpa jadwal yang tidak punya predecessor terhitung tidak memiliki ES
  (semua tanggal CPM null). Item tanpa jadwal dan tanpa dependensi tidak
  disertakan.
- **Item induk (`induk_id`).** API subitem belum ada. CPM memperlakukan setiap
  item, termasuk induk dan subitem, sebagai item biasa. Jadwal subitem tidak
  dijumlahkan ke induk, dan tidak ada dependensi implisit induk–subitem.
  **[ASUMSI]** Ditinjau ulang saat subitem dibuka di UI.
- **Pergeseran otomatis** hanya pada kolom timeline utama (kolom timeline
  pertama board), hanya maju, dan durasi hari kerja tetap. Successor bergeser
  bila mulainya lebih awal dari selesai predecessor yang berubah ditambah lag,
  berantai ke successor berikutnya. Predecessor yang tidak berubah tidak
  memicu pergeseran, sehingga pelanggaran lama di bagian lain tidak diubah diam-diam.
  Mengubah kolom timeline lain tidak menggeser apa pun, dan Gantt menampilkan
  catatannya.
- **Hak akses pergeseran.** Pelaku harus berhak mengubah setiap successor yang
  bergeser. Bila tidak, seluruh perubahan (termasuk item yang diubah) ditolak
  dengan 409 `shift_forbidden` yang menyebut item terhalang; tidak ada
  perubahan sebagian. Admin dan PMO (manager efektif) serta Project Manager
  proyek selalu boleh. Member hanya bila ia pembuat atau penanggung jawab semua
  successor yang bergeser. Viewer ditolak sejak item pertama.
- **Audit.** Setiap successor yang bergeser dicatat `geser_otomatis` dengan
  aktor = pelaku asli, `penyebab_id`/`penyebab_judul` = item yang diubah, dan
  `predecessor_id` = predecessor langsung yang memaksa pergeseran.
- **UI dependensi.** Sel kolom `dependency` di tabel menampilkan predecessor;
  Enter membuka panel tambah/hapus (dengan lag). Di Gantt, tombol D atau ikon
  di panel judul membuka panel yang sama.

### M5. Portofolio (Fase 3)
Penerimaan:
- Dasbor menampilkan semua proyek: progres, prioritas, health status.
- Progres proyek = rata-rata tertimbang progres item (bobot = durasi).
- Health status otomatis (Hijau, Kuning, Merah) dari aturan di bagian 5.
- PMO dapat menimpa dengan `status_manual` disertai alasan.
- Filter menurut prioritas, pemilik, dan status.

Keputusan implementasi M5:
- **Siapa melihat apa:** sama dengan daftar proyek. Admin dan PMO melihat semua
  proyek aktif; pengguna lain hanya proyek tempat ia menjadi anggota. Proyek
  arsip tidak ditampilkan. Halaman `/portofolio`; API `GET /portfolio` dan
  `GET /portfolio/projects/{id}/risks` (rincian item berisiko).
- **Sumber data per board:** kolom pertama tiap tipe (urutan tampilan):
  - status: Done bila label `done`;
  - progress: bila kosong, item Done 100% dan lainnya 0%;
  - person: penanggung jawab;
  - timeline utama: durasi, tenggat, dan CPM (M4).
- **Progres proyek:** rata-rata tertimbang progres item semua board; bobot =
  durasi hari kerja timeline utama (minimal 1). Item tanpa timeline berbobot 1.
- **Progres rencana:** porsi hari kerja timeline item yang sudah lewat sampai
  hari ini, ditimbang sama, hanya untuk item berjadwal. "Tertinggal dari
  rencana" bila progres aktual item yang sama lebih rendah lebih dari ambang
  (bawaan 10 poin).
- **Aturan risiko (bagian 5):** item Done tidak pernah berisiko.
  1. Tenggat (selesai timeline utama) sebelum hari ini.
  2. Sisa hari kerja setelah hari ini sampai tenggat ≤ 3 dan progres < 50%.
  3. Board punya kolom person dan kolom person pertama item kosong.
  4. Item kritis (CPM) dan predecessor langsungnya lewat tenggat, atau item
     dijadwalkan mulai sebelum predecessor selesai (`melanggar_fs`).
  5. Item kritis dan slack terhadap tanggal selesai proyek (`projects.selesai`)
     ≤ 0 hari kerja: perkiraan selesai (EF) tepat pada atau melewati tenggat
     proyek. Dilewati bila proyek tanpa tanggal selesai. Tafsiran ini dipilih
     karena dengan CPM M4 setiap item kritis selalu ber-slack 0.
  Setiap alasan memuat tanggal dan angka yang dapat ditelusuri.
- **Health:** Merah bila ada item kritis yang lewat tenggat (aturan 1) atau
  item berisiko > 25% dari semua item; Kuning bila 10–25% atau tertinggal dari
  rencana; selain itu Hijau. Proyek tanpa item Hijau.
- **Ambang** lewat env: `RISK_DUE_DAYS` (3), `RISK_DUE_PROGRESS` (50),
  `HEALTH_YELLOW_PCT` (10), `HEALTH_RED_PCT` (25), `HEALTH_BEHIND_POINTS` (10).
  **[ASUMSI]** Belum ada UI pengaturan pada v1.
- **Hari ini** = tanggal lokal server. **[ASUMSI]** Server berjalan di WIB.
- **Timpa status:** Admin dan PMO, status `hijau`/`kuning`/`merah` dengan
  alasan wajib (≤ 500 karakter); dapat dikembalikan ke otomatis. Dasbor
  menampilkan status efektif, status otomatis, siapa, dan alasannya. Tercatat
  di audit (`timpa_status`, `hapus_timpa_status`). Proyek arsip ditolak.
- **Portofolio:** tabel `portfolios` (nama unik tanpa membedakan huruf besar
  kecil). Admin dan PMO membuat, mengganti nama, dan menghapus; menghapus
  melepas proyeknya. Project Manager (manager efektif proyek) memilih
  portofolio proyeknya lewat pengaturan proyek. Filter dasbor juga menurut
  portofolio (termasuk "tanpa portofolio").
- **Perhitungan** dilakukan saat diminta, tidak disimpan: beberapa query untuk
  semua proyek terlihat, lalu CPM per board.

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

Keputusan implementasi M6:
- **Halaman `/dasbor`, API `GET /dashboard?project=`**: lingkup satu proyek
  atau semua proyek aktif yang terlihat (aturan visibilitas M5). Sumber kolom
  sama dengan M5 (kolom pertama tiap tipe per board).
  - **Sebaran status**: jumlah item per label status, digabung per judul label
    lintas board; item tanpa nilai status dihitung terpisah.
  - **Beban kerja**: item terbuka (bukan Done) per penanggung jawab. Item dengan
    beberapa penanggung jawab dihitung penuh untuk masing-masing. Disertai total
    hari kerja timeline utama, jumlah item terlambat, dan jumlah item terbuka
    tanpa penanggung jawab.
  - **Tren penyelesaian**: 12 minggu terakhir (mulai Senin, tanggal lokal
    server). Dihitung dari `audit_log`: transisi terakhir ke status `done` pada
    kolom status pertama, hanya untuk item yang saat ini Done. Item yang dibuka
    lagi tidak dihitung, dan yang selesai berulang dihitung sekali. Item yang
    disisipkan massal tanpa audit (data demo) tidak muncul di tren.
- **Keuangan**: tabel `budget_entries`, jenis `anggaran` atau `realisasi`,
  jumlah > 0 (string desimal, maksimal dua desimal, tanpa pemisah ribuan).
  Total anggaran = jumlah entri `anggaran`; kolom lama `projects.anggaran`
  dihapus. Sisa = anggaran − realisasi, boleh negatif.
  - Lihat: semua anggota proyek. Tambah, ubah, hapus: PMO, atau Project Manager
    proyek (anggota berperan manager). Admin tanpa keanggotaan manager hanya
    melihat. **[ASUMSI]** Tafsiran harfiah "hanya PMO dan Project Manager".
  - Proyek arsip hanya-baca. Semua perubahan diaudit (`budget_entry`).
  - API: `GET`/`POST /projects/{id}/budget`, `PATCH`/`DELETE /budget-entries/{id}`.
- **SSE `GET /stream?project=`**: trigger database mengirim `NOTIFY
  jwm_berubah` untuk setiap perubahan item, nilai, dependensi, board, kolom,
  anggaran, dan proyek; server meneruskannya sebagai event `berubah`
  `{project_id, jenis}` tanpa isi data, sehingga klien mengambil ulang lewat
  API biasa (hak diperiksa di sana). Event dalam 300 ms digabung.
  - Hanya proyek yang boleh dibaca. Maksimal 5 koneksi per pengguna (429).
  - Setiap 25 detik: komentar ping, pemeriksaan ulang sesi (tanpa
    memperpanjang masa idle) dan hak; sesi dicabut atau hak hilang menutup
    koneksi. Umur koneksi maksimal 5 menit; klien menyambung ulang otomatis.
  - Koneksi stream dibebaskan dari ReadTimeout/WriteTimeout server dan ditutup
    saat server shutdown.
- **Ekspor**: CSV per panel dibuat di browser (pemisah titik koma, BOM UTF-8;
  teks yang diawali `= + - @` diberi petik tunggal agar tidak dieksekusi
  sebagai rumus). PDF lewat dialog cetak browser (`window.print()` dengan
  stylesheet cetak), tanpa pustaka tambahan.

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

Keputusan implementasi M7 (dikerjakan bertahap: M7a mesin aturan dan aksi
item; M7b kirim email dan panggil webhook):
- **Aturan per board** (`automation_rules`), maksimal 50 per board, 10 kondisi,
  dan 10 aksi. Dikelola manager efektif proyek (Project Manager, PMO, Admin);
  anggota lain hanya melihat aturan dan riwayatnya. Proyek arsip hanya-baca.
  Perubahan aturan diaudit (`automation_rule`).
  API: `GET`/`POST /boards/{id}/automations`, `PATCH`/`DELETE /automations/{id}`,
  `GET /automations/{id}/runs` (100 terbaru).
- **Pemicu:** item dibuat; status berubah (opsional kolom, dari label, ke
  label; `""` = kosong); penugasan berubah (opsional kolom); tanggal tiba
  (kolom date, atau tanggal selesai timeline, sama dengan hari ini — tanggal
  lokal server; diperiksa saat start dan tiap 15 menit, sekali per aturan,
  item, dan tanggal).
- **Kondisi** (semua harus terpenuhi): nilai kolom sama dengan (status,
  prioritas, teks, angka, progres, tanggal; kosong boleh), penanggung jawab
  termasuk pengguna, item berada di grup.
- **Aksi (M7a):** pindahkan ke grup (ke akhir grup); ubah nilai kolom (status,
  prioritas, person, teks, angka, progres, tanggal). Timeline tidak dapat
  diubah otomasi karena perubahan timeline menggeser successor (M4) atas nama
  pengguna. Nilai divalidasi saat aturan disimpan dan dinormalkan ulang saat
  dijalankan (label atau keanggotaan bisa berubah).
- **Eksekusi asinkron:** trigger database mengisi `automation_events` hanya bila
  board punya aturan aktif; worker memproses antrean tiap detik
  (`FOR UPDATE SKIP LOCKED`). Aksi satu aturan berjalan dalam savepoint: satu
  aksi gagal membatalkan seluruh aksi aturan itu tanpa memengaruhi aturan lain.
  Item yang sudah dihapus atau proyek arsip: dilewati.
- **Atas nama sistem:** audit perubahan oleh aturan mencatat `aktor_id = null`
  dan `sesudah.otomasi` (id aturan, nama, pembuat).
- **Riwayat** `automation_runs`: `berhasil`, `dilewati` (dengan alasan, mis.
  kondisi tidak terpenuhi), `gagal` (pesan galat), `diblokir_loop`.
- **Pencegahan loop:** kejadian membawa rantai aturan penyebabnya (setelan
  transaksi `jwm.rantai` dibaca trigger). Satu aturan paling banyak berjalan 3
  kali per item dalam satu rantai, dan rantai paling panjang 5 langkah
  (`AUTOMATION_MAX_PER_RULE`, `AUTOMATION_MAX_DEPTH`). **[ASUMSI]** angka bawaan.
- **UI:** tab "Otomasi" di board: daftar aturan sebagai kalimat, formulir
  pemicu → kondisi → aksi, aktif/nonaktif, dan riwayat eksekusi. Pada board
  dengan aturan aktif, item yang baru diubah atau dibuat dimuat ulang sekitar
  1,5 detik kemudian agar hasil otomasi tampil.
- **M7b (menyusul):** kirim email (hanya ke pengguna terdaftar yang aktif,
  template tetap, antrean dengan percobaan ulang) dan webhook (HTTPS ke host di
  `WEBHOOK_ALLOWED_HOSTS`, tanda tangan HMAC-SHA256, rahasia per aturan, batas
  waktu 10 detik, 3 percobaan ulang).

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
- Reverse proxy tidak boleh menahan (buffer) respons `GET /api/v1/stream`
  (SSE) dan batas waktu baca proxy harus lebih dari 30 detik. Server sudah
  mengirim `X-Accel-Buffering: no` (nginx); untuk proxy lain matikan buffering
  pada jalur itu.
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
