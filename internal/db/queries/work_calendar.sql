-- name: ListWorkCalendar :many
-- Tanggal dibaca dan ditulis sebagai teks YYYY-MM-DD agar tidak melewati
-- konversi zona waktu di sisi Go.
SELECT to_char(tanggal, 'YYYY-MM-DD')::text AS tanggal, hari_kerja, keterangan
FROM work_calendar
ORDER BY tanggal;

-- name: ListWorkCalendarYear :many
SELECT to_char(tanggal, 'YYYY-MM-DD')::text AS tanggal, hari_kerja, keterangan
FROM work_calendar
WHERE extract(year FROM tanggal)::int = @tahun::int
ORDER BY tanggal;

-- name: ListWorkCalendarYears :many
SELECT DISTINCT extract(year FROM tanggal)::int AS tahun FROM work_calendar ORDER BY tahun;

-- name: GetWorkCalendarDay :one
SELECT to_char(tanggal, 'YYYY-MM-DD')::text AS tanggal, hari_kerja, keterangan
FROM work_calendar
WHERE tanggal = CAST(@tanggal::text AS date);

-- name: UpsertWorkCalendarDay :exec
INSERT INTO work_calendar (tanggal, hari_kerja, keterangan, diubah_oleh)
VALUES (CAST(@tanggal::text AS date), @hari_kerja, @keterangan, @diubah_oleh)
ON CONFLICT (tanggal)
DO UPDATE SET hari_kerja = EXCLUDED.hari_kerja, keterangan = EXCLUDED.keterangan,
              diubah_oleh = EXCLUDED.diubah_oleh, diubah_pada = now();

-- name: DeleteWorkCalendarDay :execrows
DELETE FROM work_calendar WHERE tanggal = CAST(@tanggal::text AS date);

-- name: DeleteWorkCalendarYears :execrows
DELETE FROM work_calendar WHERE extract(year FROM tanggal)::int = ANY (@tahun::int[]);

-- name: LockWorkCalendar :exec
-- Kunci transaksi agar dua impor bersamaan tidak saling menimpa sebagian.
SELECT pg_advisory_xact_lock(hashtextextended('work_calendar', 0));
