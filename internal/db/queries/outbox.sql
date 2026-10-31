-- name: EnqueueWebhook :one
INSERT INTO webhook_outbox (rule_id, url, payload)
VALUES ($1, $2, $3)
RETURNING id;

-- name: ClaimWebhooks :many
-- Ambil kiriman yang jatuh tempo dan sewa 2 menit (berikutnya digeser) agar
-- pengirim lain tidak mengambilnya selama dikirim di luar transaksi.
UPDATE webhook_outbox SET berikutnya = now() + interval '2 minutes'
WHERE id IN (
    SELECT id FROM webhook_outbox
    WHERE status = 'menunggu' AND berikutnya <= now()
    ORDER BY berikutnya, id
    LIMIT $1
    FOR UPDATE SKIP LOCKED
)
RETURNING id, rule_id, url, payload, percobaan;

-- name: FinishWebhook :exec
UPDATE webhook_outbox
SET status = @status, percobaan = percobaan + 1, galat = @galat, kode_http = @kode_http,
    berikutnya = @berikutnya, selesai_pada = CASE WHEN @status <> 'menunggu' THEN now() END
WHERE id = @id;

-- name: ListWebhookDeliveries :many
-- Kiriman webhook terbaru sebuah aturan (untuk riwayat).
SELECT id, url, status, percobaan, galat, kode_http, dibuat_pada, selesai_pada
FROM webhook_outbox
WHERE rule_id = $1
ORDER BY id DESC
LIMIT $2;

-- name: GetAutomationSecret :one
SELECT rahasia FROM automation_secrets WHERE rule_id = $1;

-- name: UpsertAutomationSecret :exec
INSERT INTO automation_secrets (rule_id, rahasia) VALUES ($1, $2)
ON CONFLICT (rule_id) DO UPDATE SET rahasia = EXCLUDED.rahasia, dibuat_pada = now();

-- name: DeleteAutomationSecret :exec
DELETE FROM automation_secrets WHERE rule_id = $1;

-- name: GetItemContext :one
-- Nama board dan proyek untuk payload webhook.
SELECT i.judul, b.id AS board_id, b.nama AS board_nama, p.id AS project_id, p.nama AS project_nama
FROM items i
JOIN boards b ON b.id = i.board_id
JOIN projects p ON p.id = b.project_id
WHERE i.id = $1;
