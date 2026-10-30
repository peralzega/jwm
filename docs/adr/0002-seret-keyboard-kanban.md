# ADR 0002 — Seret keyboard di Kanban tanpa sensor keyboard dnd-kit

Status: diterima (M2c).

## Konteks

SPEC M2: kartu Kanban dapat diseret untuk mengubah status, dengan mouse dan
keyboard. Seret-lepas memakai `@dnd-kit/core` 6.3.1 (MIT). Kolom Kanban
divirtualisasi, sehingga kartu yang diseret ditampilkan lewat `DragOverlay`.
Area Kanban dapat digulir ke samping bila kolom tidak muat di layar.

Saat uji di browser (mode `normal`, area Kanban tergulir ke kanan), seret
keyboard dengan `KeyboardSensor` bawaan gagal: panah kiri tidak memindah kartu
dan kartu "mendarat" di kolom asal. `KeyboardSensor` tidak memindah koordinat
bila kolom tujuan berada di separuh kiri area gulir. Ia menggulir area itu dan
mengandalkan deteksi ulang kolom dari posisi gulir. Deteksi ulang itu tidak
terjadi saat kartu diseret lewat `DragOverlay`. Tes jsdom tidak menangkapnya
karena jsdom tidak punya tata letak dan gulir sungguhan.

## Keputusan

- Mouse dan sentuh tetap lewat dnd-kit (`MouseSensor` dengan jarak aktivasi,
  `TouchSensor` dengan jeda dan toleransi, `pointerWithin` lalu
  `closestCenter` untuk deteksi kolom).
- Keyboard memakai logika sendiri yang tidak bergantung geometri
  (`web/src/board/kanban/keyboard.ts`, fungsi murni `keyAction`):
  Spasi/Enter mengangkat, panah kiri/kanan memilih kolom tujuan (disorot dan
  digulir ke tampilan), Spasi/Enter melepas, Esc atau Tab membatalkan.
- Pengumuman untuk pembaca layar lewat live region milik Kanban, berbahasa
  Indonesia. Fokus mengikuti kartu ke kolom tujuan.
- Perubahan status lewat API yang sama dengan tabel
  (`PUT /items/{id}/values/{kolom}`), optimistic, dikembalikan bila ditolak.

## Konsekuensi

- Perilaku keyboard dapat diuji penuh di jsdom tanpa tiruan geometri.
- Kode keyboard kecil yang harus dirawat sendiri; dnd-kit tidak lagi
  menangani keyboard. Bila kelak pindah ke `@dnd-kit/react` (saat ini 0.x),
  periksa ulang apakah sensor keyboard barunya menangani kasus di atas.
- Urutan kartu dalam kolom tidak dapat diubah (tanpa `@dnd-kit/sortable`):
  urutan item di server berlaku per grup, sedangkan kolom Kanban berisi item
  dari banyak grup.
