-- +goose Up
-- Kepadatan tampilan per pengguna (SPEC M2): compact (default) atau normal.
ALTER TABLE users
    ADD COLUMN kepadatan text NOT NULL DEFAULT 'compact'
    CHECK (kepadatan IN ('compact', 'normal'));

-- +goose Down
ALTER TABLE users DROP COLUMN kepadatan;
