-- name: MaxItemUrutan :one
SELECT COALESCE(max(urutan), 0)::double precision FROM items
WHERE group_id = $1 AND dihapus_pada IS NULL;

-- name: CreateItem :one
INSERT INTO items (board_id, group_id, judul, urutan, dibuat_oleh)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetItemAccess :one
-- Item aktif beserta proyeknya dan apakah @user_id ditugaskan di kolom person mana pun.
SELECT sqlc.embed(i), b.project_id,
       EXISTS (
           SELECT 1 FROM item_values v
           JOIN board_columns c ON c.id = v.column_id AND c.tipe = 'person' AND c.dihapus_pada IS NULL
           WHERE v.item_id = i.id AND v.nilai_json @> jsonb_build_array(@user_id::text)
       )::boolean AS ditugaskan
FROM items i
JOIN boards b ON b.id = i.board_id AND b.dihapus_pada IS NULL
WHERE i.id = @id AND i.dihapus_pada IS NULL;

-- name: GetItemForUpdate :one
SELECT * FROM items WHERE id = $1 AND dihapus_pada IS NULL FOR UPDATE;

-- name: ListItems :many
SELECT i.id, i.board_id, i.group_id, i.judul, i.urutan, i.dibuat_oleh, i.dibuat_pada, i.diubah_pada,
       COALESCE((
           SELECT jsonb_object_agg(v.column_id::text, v.nilai_json)
           FROM item_values v
           JOIN board_columns c ON c.id = v.column_id AND c.dihapus_pada IS NULL
           WHERE v.item_id = i.id
       ), '{}'::jsonb)::jsonb AS nilai,
       EXISTS (
           SELECT 1 FROM item_values v
           JOIN board_columns c ON c.id = v.column_id AND c.tipe = 'person' AND c.dihapus_pada IS NULL
           WHERE v.item_id = i.id AND v.nilai_json @> jsonb_build_array(@user_id::text)
       )::boolean AS ditugaskan
FROM items i
WHERE i.board_id = @board_id
  AND i.dihapus_pada IS NULL
  AND (sqlc.narg(group_id)::uuid IS NULL OR i.group_id = sqlc.narg(group_id))
  AND (sqlc.narg(setelah_id)::uuid IS NULL OR i.id > sqlc.narg(setelah_id))
  AND (sqlc.narg(id)::uuid IS NULL OR i.id = sqlc.narg(id))
ORDER BY i.id
LIMIT @batas;

-- name: UpdateItemTitle :exec
UPDATE items SET judul = $2, diubah_pada = now() WHERE id = $1;

-- name: MoveItem :exec
UPDATE items SET group_id = $2, urutan = $3, diubah_pada = now() WHERE id = $1;

-- name: TouchItem :exec
UPDATE items SET diubah_pada = now() WHERE id = $1;

-- name: SoftDeleteItem :exec
UPDATE items SET dihapus_pada = now(), diubah_pada = now() WHERE id = $1;

-- name: NextItemUrutanAfter :one
-- Urutan item berikutnya setelah @urutan di grup (mengabaikan item yang dipindah).
SELECT urutan FROM items
WHERE group_id = @group_id AND dihapus_pada IS NULL AND urutan > @urutan AND id <> @kecuali
ORDER BY urutan
LIMIT 1;

-- name: MinItemUrutan :one
SELECT urutan FROM items
WHERE group_id = @group_id AND dihapus_pada IS NULL AND id <> @kecuali
ORDER BY urutan
LIMIT 1;

-- name: RenumberGroupItems :exec
-- Menata ulang urutan satu grup dengan jarak 1024 bila celah pecahan terlalu sempit.
UPDATE items i SET urutan = r.n * 1024
FROM (
    SELECT x.id, row_number() OVER (ORDER BY x.urutan, x.id) AS n
    FROM items x WHERE x.group_id = $1 AND x.dihapus_pada IS NULL
) r
WHERE i.id = r.id;

-- name: GetItemValue :one
SELECT * FROM item_values WHERE item_id = $1 AND column_id = $2;

-- name: UpsertItemValue :exec
INSERT INTO item_values (item_id, column_id, nilai_json, diubah_oleh)
VALUES ($1, $2, $3, $4)
ON CONFLICT (item_id, column_id)
DO UPDATE SET nilai_json = EXCLUDED.nilai_json, diubah_oleh = EXCLUDED.diubah_oleh, diubah_pada = now();

-- name: DeleteItemValue :exec
DELETE FROM item_values WHERE item_id = $1 AND column_id = $2;
