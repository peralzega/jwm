-- +goose Up
CREATE TABLE users (
    id                uuid PRIMARY KEY DEFAULT uuidv7(),
    nama              text        NOT NULL CHECK (length(nama) BETWEEN 1 AND 200),
    email             text        NOT NULL CHECK (length(email) BETWEEN 3 AND 254),
    password_hash     text        NOT NULL,
    peran             text        NOT NULL DEFAULT 'viewer'
                      CHECK (peran IN ('admin', 'pmo', 'project_manager', 'member', 'viewer')),
    aktif             boolean     NOT NULL DEFAULT true,
    wajib_ganti_sandi boolean     NOT NULL DEFAULT true,
    gagal_login       integer     NOT NULL DEFAULT 0 CHECK (gagal_login >= 0),
    terkunci_hingga   timestamptz,
    login_terakhir    timestamptz,
    dibuat_pada       timestamptz NOT NULL DEFAULT now(),
    diubah_pada       timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX users_email_lower_key ON users (lower(email));

CREATE TABLE sessions (
    id                 uuid PRIMARY KEY DEFAULT uuidv7(),
    user_id            uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    token_hash         bytea       NOT NULL UNIQUE,
    kedaluwarsa        timestamptz NOT NULL,
    kedaluwarsa_mutlak timestamptz NOT NULL,
    terakhir_aktif     timestamptz NOT NULL DEFAULT now(),
    ip                 inet,
    user_agent         text,
    dibuat_pada        timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX sessions_user_id_idx ON sessions (user_id);
CREATE INDEX sessions_kedaluwarsa_mutlak_idx ON sessions (kedaluwarsa_mutlak);

-- +goose Down
DROP TABLE sessions;
DROP TABLE users;
