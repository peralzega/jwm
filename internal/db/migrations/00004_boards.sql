-- +goose Up
CREATE TABLE boards (
    id          uuid PRIMARY KEY DEFAULT uuidv7(),
    project_id  uuid        NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    nama        text        NOT NULL CHECK (length(nama) BETWEEN 1 AND 200),
    urutan      double precision NOT NULL,
    dibuat_pada timestamptz NOT NULL DEFAULT now(),
    diubah_pada timestamptz NOT NULL DEFAULT now(),
    dihapus_pada timestamptz
);

CREATE INDEX boards_project_id_idx ON boards (project_id) WHERE dihapus_pada IS NULL;

CREATE TABLE board_columns (
    id               uuid PRIMARY KEY DEFAULT uuidv7(),
    board_id         uuid        NOT NULL REFERENCES boards (id) ON DELETE CASCADE,
    tipe             text        NOT NULL CHECK (tipe IN (
                         'text', 'status', 'person', 'date', 'timeline',
                         'number', 'progress', 'priority', 'dependency')),
    judul            text        NOT NULL CHECK (length(judul) BETWEEN 1 AND 100),
    urutan           double precision NOT NULL,
    konfigurasi_json jsonb       NOT NULL DEFAULT '{}'::jsonb,
    dibuat_pada      timestamptz NOT NULL DEFAULT now(),
    dihapus_pada     timestamptz
);

CREATE INDEX board_columns_board_id_idx ON board_columns (board_id) WHERE dihapus_pada IS NULL;

CREATE TABLE groups (
    id           uuid PRIMARY KEY DEFAULT uuidv7(),
    board_id     uuid        NOT NULL REFERENCES boards (id) ON DELETE CASCADE,
    judul        text        NOT NULL CHECK (length(judul) BETWEEN 1 AND 100),
    warna        text        NOT NULL CHECK (warna ~ '^[a-z_]{1,20}$'),
    urutan       double precision NOT NULL,
    dibuat_pada  timestamptz NOT NULL DEFAULT now(),
    dihapus_pada timestamptz
);

CREATE INDEX groups_board_id_idx ON groups (board_id) WHERE dihapus_pada IS NULL;

-- +goose Down
DROP TABLE groups;
DROP TABLE board_columns;
DROP TABLE boards;
