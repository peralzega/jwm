-- +goose Up
-- M7: otomasi no-code. Aturan per board (pemicu -> kondisi -> aksi), antrean
-- kejadian yang diisi trigger, riwayat eksekusi, dan penanda "tanggal tiba".
CREATE TABLE automation_rules (
    id           uuid PRIMARY KEY DEFAULT uuidv7(),
    board_id     uuid        NOT NULL REFERENCES boards (id) ON DELETE CASCADE,
    nama         text        NOT NULL CHECK (char_length(nama) BETWEEN 1 AND 200),
    pemicu_json  jsonb       NOT NULL,
    kondisi_json jsonb       NOT NULL DEFAULT '[]',
    aksi_json    jsonb       NOT NULL,
    aktif        boolean     NOT NULL DEFAULT true,
    dibuat_oleh  uuid        NOT NULL REFERENCES users (id),
    dibuat_pada  timestamptz NOT NULL DEFAULT now(),
    diubah_pada  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX automation_rules_board_id_idx ON automation_rules (board_id) WHERE aktif;

-- rantai: id aturan yang (berurutan) menyebabkan kejadian ini; kosong bila
-- dari pengguna. Dipakai pencegahan loop.
CREATE TABLE automation_events (
    id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    board_id      uuid        NOT NULL,
    item_id       uuid        NOT NULL,
    jenis         text        NOT NULL
                  CHECK (jenis IN ('item_dibuat', 'status_berubah', 'penugasan_berubah', 'tanggal_tiba')),
    column_id     uuid,
    -- rule_id: hanya aturan ini (kejadian "tanggal tiba" dibuat per aturan).
    rule_id       uuid,
    sebelum       jsonb,
    sesudah       jsonb,
    rantai        jsonb       NOT NULL DEFAULT '[]',
    dibuat_pada   timestamptz NOT NULL DEFAULT now(),
    diproses_pada timestamptz
);

CREATE INDEX automation_events_antrean_idx ON automation_events (id) WHERE diproses_pada IS NULL;

CREATE TABLE automation_runs (
    id       bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    rule_id  uuid        NOT NULL REFERENCES automation_rules (id) ON DELETE CASCADE,
    item_id  uuid        REFERENCES items (id) ON DELETE SET NULL,
    event_id bigint,
    hasil    text        NOT NULL CHECK (hasil IN ('berhasil', 'dilewati', 'gagal', 'diblokir_loop')),
    pesan    text        NOT NULL DEFAULT '',
    waktu    timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX automation_runs_rule_idx ON automation_runs (rule_id, id DESC);

-- Satu kejadian "tanggal tiba" per aturan, item, dan tanggal.
CREATE TABLE automation_date_fired (
    rule_id uuid NOT NULL REFERENCES automation_rules (id) ON DELETE CASCADE,
    item_id uuid NOT NULL REFERENCES items (id) ON DELETE CASCADE,
    tanggal date NOT NULL,
    PRIMARY KEY (rule_id, item_id, tanggal)
);

-- Kejadian hanya dicatat bila board punya aturan aktif. Rantai dibaca dari
-- setelan transaksi jwm.rantai yang diisi worker saat menjalankan aksi.
-- +goose StatementBegin
CREATE FUNCTION jwm_automation_event() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    bid    uuid;
    tipe   text;
    rantai jsonb := COALESCE(NULLIF(current_setting('jwm.rantai', true), ''), '[]')::jsonb;
BEGIN
    IF TG_TABLE_NAME = 'items' THEN
        IF NOT EXISTS (SELECT 1 FROM automation_rules WHERE board_id = NEW.board_id AND aktif) THEN
            RETURN NULL;
        END IF;
        INSERT INTO automation_events (board_id, item_id, jenis, rantai)
        VALUES (NEW.board_id, NEW.id, 'item_dibuat', rantai);
        RETURN NULL;
    END IF;

    -- item_values: status atau person yang berubah.
    IF TG_OP = 'UPDATE' AND OLD.nilai_json IS NOT DISTINCT FROM NEW.nilai_json THEN
        RETURN NULL;
    END IF;
    SELECT c.board_id, c.tipe INTO bid, tipe
    FROM board_columns c
    WHERE c.id = CASE WHEN TG_OP = 'DELETE' THEN OLD.column_id ELSE NEW.column_id END;
    IF tipe IS NULL OR tipe NOT IN ('status', 'person') THEN
        RETURN NULL;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM automation_rules WHERE board_id = bid AND aktif) THEN
        RETURN NULL;
    END IF;
    INSERT INTO automation_events (board_id, item_id, jenis, column_id, sebelum, sesudah, rantai)
    VALUES (
        bid,
        CASE WHEN TG_OP = 'DELETE' THEN OLD.item_id ELSE NEW.item_id END,
        CASE WHEN tipe = 'status' THEN 'status_berubah' ELSE 'penugasan_berubah' END,
        CASE WHEN TG_OP = 'DELETE' THEN OLD.column_id ELSE NEW.column_id END,
        CASE WHEN TG_OP = 'INSERT' THEN NULL ELSE OLD.nilai_json END,
        CASE WHEN TG_OP = 'DELETE' THEN NULL ELSE NEW.nilai_json END,
        rantai
    );
    RETURN NULL;
END
$$;
-- +goose StatementEnd

CREATE TRIGGER items_otomasi AFTER INSERT ON items
    FOR EACH ROW EXECUTE FUNCTION jwm_automation_event();
CREATE TRIGGER item_values_otomasi AFTER INSERT OR UPDATE OR DELETE ON item_values
    FOR EACH ROW EXECUTE FUNCTION jwm_automation_event();

-- +goose Down
DROP TRIGGER item_values_otomasi ON item_values;
DROP TRIGGER items_otomasi ON items;
DROP FUNCTION jwm_automation_event();
DROP TABLE automation_date_fired;
DROP TABLE automation_runs;
DROP TABLE automation_events;
DROP TABLE automation_rules;
