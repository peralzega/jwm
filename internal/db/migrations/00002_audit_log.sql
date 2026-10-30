-- +goose Up
CREATE TABLE audit_log (
    id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    aktor_id   uuid REFERENCES users (id) ON DELETE SET NULL,
    entitas    text        NOT NULL,
    entitas_id text,
    aksi       text        NOT NULL,
    sebelum    jsonb,
    sesudah    jsonb,
    ip         inet,
    waktu      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX audit_log_entitas_idx ON audit_log (entitas, entitas_id);
CREATE INDEX audit_log_waktu_idx ON audit_log (waktu);

-- +goose Down
DROP TABLE audit_log;
