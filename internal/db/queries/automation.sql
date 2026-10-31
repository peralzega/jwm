-- name: ListAutomationRules :many
SELECT sqlc.embed(r), u.nama AS dibuat_oleh_nama
FROM automation_rules r
JOIN users u ON u.id = r.dibuat_oleh
WHERE r.board_id = $1
ORDER BY r.dibuat_pada, r.id;

-- name: CountAutomationRules :one
SELECT count(*)::int FROM automation_rules WHERE board_id = $1;

-- name: GetAutomationRule :one
SELECT * FROM automation_rules WHERE id = $1;

-- name: GetAutomationRuleForUpdate :one
SELECT * FROM automation_rules WHERE id = $1 FOR UPDATE;

-- name: CreateAutomationRule :one
INSERT INTO automation_rules (board_id, nama, pemicu_json, kondisi_json, aksi_json, aktif, dibuat_oleh)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: UpdateAutomationRule :one
UPDATE automation_rules
SET nama = $2, pemicu_json = $3, kondisi_json = $4, aksi_json = $5, aktif = $6, diubah_pada = now()
WHERE id = $1
RETURNING *;

-- name: DeleteAutomationRule :exec
DELETE FROM automation_rules WHERE id = $1;

-- name: ListActiveRulesForBoard :many
SELECT * FROM automation_rules WHERE board_id = $1 AND aktif ORDER BY dibuat_pada, id;

-- name: ClaimAutomationEvents :many
-- Kejadian yang belum diproses; SKIP LOCKED agar beberapa worker tidak bertabrakan.
SELECT * FROM automation_events
WHERE diproses_pada IS NULL
ORDER BY id
LIMIT $1
FOR UPDATE SKIP LOCKED;

-- name: MarkAutomationEventDone :exec
UPDATE automation_events SET diproses_pada = now() WHERE id = $1;

-- name: SetAutomationChain :exec
-- Rantai aturan untuk kejadian yang dibuat trigger dalam transaksi ini.
SELECT set_config('jwm.rantai', @rantai::text, true);

-- name: InsertAutomationRun :exec
INSERT INTO automation_runs (rule_id, item_id, event_id, hasil, pesan)
VALUES ($1, $2, $3, $4, $5);

-- name: ListAutomationRuns :many
SELECT r.id, r.rule_id, r.item_id, r.event_id, r.hasil, r.pesan, r.waktu, i.judul AS item_judul
FROM automation_runs r
LEFT JOIN items i ON i.id = r.item_id
WHERE r.rule_id = $1
ORDER BY r.id DESC
LIMIT $2;

-- name: GetItemForAutomation :one
-- Item aktif beserta proyek dan status arsipnya; dikunci untuk aksi.
SELECT i.id, i.board_id, i.group_id, i.judul, b.project_id, p.diarsipkan_pada
FROM items i
JOIN boards b ON b.id = i.board_id AND b.dihapus_pada IS NULL
JOIN projects p ON p.id = b.project_id
WHERE i.id = $1 AND i.dihapus_pada IS NULL
FOR UPDATE OF i;

-- name: ListItemValuesForItem :many
SELECT column_id, nilai_json FROM item_values WHERE item_id = $1;

-- name: ListDueDateRules :many
SELECT id, board_id, pemicu_json FROM automation_rules
WHERE aktif AND pemicu_json ->> 'jenis' = 'tanggal_tiba';

-- name: EnqueueDueDateEvents :execrows
-- Item yang tanggalnya (kolom date, atau selesai pada timeline) sama dengan
-- @hari_ini, sekali per aturan, item, dan tanggal.
WITH due AS (
    SELECT i.id
    FROM items i
    JOIN item_values v ON v.item_id = i.id AND v.column_id = @column_id
    WHERE i.board_id = @board_id AND i.dihapus_pada IS NULL
      AND CASE WHEN jsonb_typeof(v.nilai_json) = 'string' THEN v.nilai_json #>> '{}'
               ELSE v.nilai_json ->> 'selesai' END = @hari_ini::text
), fired AS (
    INSERT INTO automation_date_fired (rule_id, item_id, tanggal)
    SELECT @rule_id, id, CAST(@hari_ini::text AS date) FROM due
    ON CONFLICT DO NOTHING
    RETURNING item_id
)
INSERT INTO automation_events (board_id, item_id, jenis, column_id, rule_id)
SELECT @board_id, item_id, 'tanggal_tiba', @column_id, @rule_id FROM fired;
