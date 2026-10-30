-- +goose Up
-- Dependensi antar-item dalam satu board (M3). Perhitungan CPM dan pergeseran
-- otomatis successor menyusul di M4.
CREATE TABLE dependencies (
    id             uuid PRIMARY KEY DEFAULT uuidv7(),
    board_id       uuid        NOT NULL REFERENCES boards (id) ON DELETE CASCADE,
    predecessor_id uuid        NOT NULL REFERENCES items (id) ON DELETE CASCADE,
    successor_id   uuid        NOT NULL REFERENCES items (id) ON DELETE CASCADE,
    tipe           text        NOT NULL DEFAULT 'FS' CHECK (tipe IN ('FS')),
    lag_hari       integer     NOT NULL DEFAULT 0 CHECK (lag_hari BETWEEN -365 AND 365),
    dibuat_oleh    uuid        NOT NULL REFERENCES users (id),
    dibuat_pada    timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT dependencies_bukan_diri_sendiri CHECK (predecessor_id <> successor_id),
    CONSTRAINT dependencies_pasangan_unik UNIQUE (predecessor_id, successor_id)
);

CREATE INDEX dependencies_board_id_idx ON dependencies (board_id);
CREATE INDEX dependencies_successor_id_idx ON dependencies (successor_id);

-- +goose Down
DROP TABLE dependencies;
