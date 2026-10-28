-- +goose Up
CREATE TABLE projects (
    id              uuid PRIMARY KEY DEFAULT uuidv7(),
    portfolio_id    uuid,          -- FK ke portfolios ditambahkan di M5
    nama            text        NOT NULL CHECK (length(nama) BETWEEN 1 AND 200),
    pemilik_id      uuid        NOT NULL REFERENCES users (id),
    mulai           date,
    selesai         date,
    prioritas       text        NOT NULL DEFAULT 'sedang'
                    CHECK (prioritas IN ('rendah', 'sedang', 'tinggi', 'kritis')),
    status_manual   text,          -- diisi PMO di M5
    anggaran        numeric(18, 2) CHECK (anggaran >= 0),
    dibuat_oleh     uuid        NOT NULL REFERENCES users (id),
    dibuat_pada     timestamptz NOT NULL DEFAULT now(),
    diubah_pada     timestamptz NOT NULL DEFAULT now(),
    diarsipkan_pada timestamptz,
    CHECK (selesai IS NULL OR mulai IS NULL OR selesai >= mulai)
);

CREATE INDEX projects_pemilik_id_idx ON projects (pemilik_id);

CREATE TABLE project_members (
    project_id       uuid        NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    user_id          uuid        NOT NULL REFERENCES users (id),
    peran_proyek     text        NOT NULL CHECK (peran_proyek IN ('manager', 'member', 'viewer')),
    ditambahkan_pada timestamptz NOT NULL DEFAULT now(),
    ditambahkan_oleh uuid REFERENCES users (id) ON DELETE SET NULL,
    PRIMARY KEY (project_id, user_id)
);

CREATE INDEX project_members_user_id_idx ON project_members (user_id);

-- +goose Down
DROP TABLE project_members;
DROP TABLE projects;
