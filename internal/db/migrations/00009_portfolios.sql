-- +goose Up
-- Portofolio dan status health manual (M5).
CREATE TABLE portfolios (
    id          uuid PRIMARY KEY DEFAULT uuidv7(),
    nama        text        NOT NULL CHECK (char_length(nama) BETWEEN 1 AND 200),
    dibuat_oleh uuid        NOT NULL REFERENCES users (id),
    dibuat_pada timestamptz NOT NULL DEFAULT now(),
    diubah_pada timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX portfolios_nama_unik ON portfolios (lower(nama));

-- Menghapus portofolio melepas proyeknya (tidak menghapus proyek).
ALTER TABLE projects
    ADD CONSTRAINT projects_portfolio_id_fkey FOREIGN KEY (portfolio_id) REFERENCES portfolios (id) ON DELETE SET NULL;
CREATE INDEX projects_portfolio_id_idx ON projects (portfolio_id) WHERE portfolio_id IS NOT NULL;

-- status_manual (kolom M1, belum dipakai) diisi PMO atau Admin beserta alasannya.
ALTER TABLE projects
    ADD COLUMN status_manual_alasan text CHECK (char_length(status_manual_alasan) BETWEEN 1 AND 500),
    ADD COLUMN status_manual_oleh   uuid REFERENCES users (id),
    ADD COLUMN status_manual_pada   timestamptz,
    ADD CONSTRAINT projects_status_manual_check CHECK (status_manual IN ('hijau', 'kuning', 'merah')),
    ADD CONSTRAINT projects_status_manual_lengkap CHECK (
        (status_manual IS NULL) = (status_manual_alasan IS NULL)
        AND (status_manual IS NULL) = (status_manual_oleh IS NULL)
        AND (status_manual IS NULL) = (status_manual_pada IS NULL)
    );

-- +goose Down
ALTER TABLE projects
    DROP CONSTRAINT projects_status_manual_lengkap,
    DROP CONSTRAINT projects_status_manual_check,
    DROP COLUMN status_manual_pada,
    DROP COLUMN status_manual_oleh,
    DROP COLUMN status_manual_alasan,
    DROP CONSTRAINT projects_portfolio_id_fkey;
DROP INDEX projects_portfolio_id_idx;
DROP TABLE portfolios;
