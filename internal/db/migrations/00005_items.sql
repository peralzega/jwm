-- +goose Up
CREATE TABLE items (
    id           uuid PRIMARY KEY DEFAULT uuidv7(),
    board_id     uuid        NOT NULL REFERENCES boards (id) ON DELETE CASCADE,
    group_id     uuid        NOT NULL REFERENCES groups (id),
    judul        text        NOT NULL CHECK (length(judul) BETWEEN 1 AND 500),
    urutan       double precision NOT NULL,
    induk_id     uuid REFERENCES items (id),  -- subitem; belum dipakai API di M1
    dibuat_oleh  uuid        NOT NULL REFERENCES users (id),
    dibuat_pada  timestamptz NOT NULL DEFAULT now(),
    diubah_pada  timestamptz NOT NULL DEFAULT now(),
    dihapus_pada timestamptz
);

CREATE INDEX items_group_urutan_idx ON items (group_id, urutan) WHERE dihapus_pada IS NULL;
CREATE INDEX items_board_id_idx ON items (board_id, id) WHERE dihapus_pada IS NULL;
CREATE INDEX items_induk_id_idx ON items (induk_id) WHERE induk_id IS NOT NULL;

CREATE TABLE item_values (
    item_id     uuid        NOT NULL REFERENCES items (id) ON DELETE CASCADE,
    column_id   uuid        NOT NULL REFERENCES board_columns (id) ON DELETE CASCADE,
    nilai_json  jsonb       NOT NULL,
    diubah_oleh uuid REFERENCES users (id) ON DELETE SET NULL,
    diubah_pada timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (item_id, column_id)
);

CREATE INDEX item_values_column_id_idx ON item_values (column_id);

-- +goose Down
DROP TABLE item_values;
DROP TABLE items;
