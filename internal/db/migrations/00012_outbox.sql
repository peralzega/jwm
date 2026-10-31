-- +goose Up
-- M7b: antrean webhook dengan percobaan ulang dan rahasia HMAC per aturan.
-- Tidak ada kanal email (keputusan pengguna, revisi M7).
CREATE TABLE webhook_outbox (
    id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    rule_id      uuid        REFERENCES automation_rules (id) ON DELETE SET NULL,
    url          text        NOT NULL,
    payload      jsonb       NOT NULL,
    status       text        NOT NULL DEFAULT 'menunggu' CHECK (status IN ('menunggu', 'terkirim', 'gagal')),
    percobaan    int         NOT NULL DEFAULT 0,
    berikutnya   timestamptz NOT NULL DEFAULT now(),
    galat        text        NOT NULL DEFAULT '',
    kode_http    int,
    dibuat_pada  timestamptz NOT NULL DEFAULT now(),
    selesai_pada timestamptz
);

CREATE INDEX webhook_outbox_antrean_idx ON webhook_outbox (berikutnya) WHERE status = 'menunggu';
CREATE INDEX webhook_outbox_rule_idx ON webhook_outbox (rule_id, id DESC);

-- Rahasia HMAC dipakai bersama semua aksi webhook dalam satu aturan.
-- ASSUMPTION: disimpan tanpa enkripsi di database (akses database = akses rahasia);
-- tidak pernah dikirim balik lewat API kecuali sekali saat dibuat atau diganti.
CREATE TABLE automation_secrets (
    rule_id     uuid        PRIMARY KEY REFERENCES automation_rules (id) ON DELETE CASCADE,
    rahasia     text        NOT NULL,
    dibuat_pada timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE automation_secrets;
DROP TABLE webhook_outbox;
