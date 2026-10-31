-- +goose Up
-- M6: entri anggaran dan realisasi per proyek, serta notifikasi perubahan
-- untuk SSE (LISTEN/NOTIFY).
CREATE TABLE budget_entries (
    id          uuid PRIMARY KEY DEFAULT uuidv7(),
    project_id  uuid           NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    tanggal     date           NOT NULL,
    jenis       text           NOT NULL CHECK (jenis IN ('anggaran', 'realisasi')),
    jumlah      numeric(18, 2) NOT NULL CHECK (jumlah > 0),
    keterangan  text           NOT NULL DEFAULT '' CHECK (char_length(keterangan) <= 500),
    dibuat_oleh uuid           NOT NULL REFERENCES users (id),
    dibuat_pada timestamptz    NOT NULL DEFAULT now(),
    diubah_oleh uuid           REFERENCES users (id),
    diubah_pada timestamptz    NOT NULL DEFAULT now()
);

CREATE INDEX budget_entries_project_id_idx ON budget_entries (project_id, tanggal);

-- Total anggaran kini dari budget_entries (keputusan M6); kolom lama tidak dipakai.
ALTER TABLE projects DROP COLUMN anggaran;

-- Setiap perubahan data proyek mengirim NOTIFY jwm_berubah dengan payload
-- "<project_id>:<jenis>". Payload sama dalam satu transaksi digabung PostgreSQL,
-- sehingga sisipan massal hanya menghasilkan satu notifikasi.
-- +goose StatementBegin
CREATE FUNCTION jwm_notify_change() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    pid   uuid;
    jenis text := TG_ARGV[0];
    r     record;
BEGIN
    IF TG_OP = 'DELETE' THEN
        r := OLD;
    ELSE
        r := NEW;
    END IF;
    IF TG_TABLE_NAME = 'item_values' THEN
        SELECT b.project_id INTO pid FROM items i JOIN boards b ON b.id = i.board_id WHERE i.id = r.item_id;
    ELSIF TG_TABLE_NAME IN ('items', 'dependencies', 'board_columns') THEN
        SELECT b.project_id INTO pid FROM boards b WHERE b.id = r.board_id;
    ELSIF TG_TABLE_NAME IN ('boards', 'budget_entries') THEN
        pid := r.project_id;
    ELSIF TG_TABLE_NAME = 'projects' THEN
        pid := r.id;
    END IF;
    IF pid IS NOT NULL THEN
        PERFORM pg_notify('jwm_berubah', pid::text || ':' || jenis);
    END IF;
    RETURN NULL;
END
$$;
-- +goose StatementEnd

CREATE TRIGGER item_values_berubah AFTER INSERT OR UPDATE OR DELETE ON item_values
    FOR EACH ROW EXECUTE FUNCTION jwm_notify_change('item');
CREATE TRIGGER items_berubah AFTER INSERT OR UPDATE OR DELETE ON items
    FOR EACH ROW EXECUTE FUNCTION jwm_notify_change('item');
CREATE TRIGGER dependencies_berubah AFTER INSERT OR UPDATE OR DELETE ON dependencies
    FOR EACH ROW EXECUTE FUNCTION jwm_notify_change('dependensi');
CREATE TRIGGER board_columns_berubah AFTER INSERT OR UPDATE OR DELETE ON board_columns
    FOR EACH ROW EXECUTE FUNCTION jwm_notify_change('board');
CREATE TRIGGER boards_berubah AFTER INSERT OR UPDATE OR DELETE ON boards
    FOR EACH ROW EXECUTE FUNCTION jwm_notify_change('board');
CREATE TRIGGER budget_entries_berubah AFTER INSERT OR UPDATE OR DELETE ON budget_entries
    FOR EACH ROW EXECUTE FUNCTION jwm_notify_change('anggaran');
CREATE TRIGGER projects_berubah AFTER UPDATE ON projects
    FOR EACH ROW EXECUTE FUNCTION jwm_notify_change('proyek');

-- +goose Down
DROP TRIGGER projects_berubah ON projects;
DROP TRIGGER budget_entries_berubah ON budget_entries;
DROP TRIGGER boards_berubah ON boards;
DROP TRIGGER board_columns_berubah ON board_columns;
DROP TRIGGER dependencies_berubah ON dependencies;
DROP TRIGGER items_berubah ON items;
DROP TRIGGER item_values_berubah ON item_values;
DROP FUNCTION jwm_notify_change();
ALTER TABLE projects ADD COLUMN anggaran numeric(18, 2) CHECK (anggaran >= 0);
DROP TABLE budget_entries;
