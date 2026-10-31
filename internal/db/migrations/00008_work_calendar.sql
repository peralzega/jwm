-- +goose Up
-- Kalender kerja (M4). Sabtu dan Minggu libur tanpa perlu dicatat. Baris di
-- tabel ini menimpa pola mingguan: hari_kerja = false untuk libur, true untuk
-- akhir pekan yang dijadikan hari kerja (mis. pengganti cuti bersama).
-- Data resmi diisi Admin lewat formulir atau impor CSV.
CREATE TABLE work_calendar (
    tanggal     date        PRIMARY KEY,
    hari_kerja  boolean     NOT NULL DEFAULT false,
    keterangan  text        NOT NULL DEFAULT '' CHECK (char_length(keterangan) <= 200),
    diubah_oleh uuid        REFERENCES users (id),
    diubah_pada timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE work_calendar;
