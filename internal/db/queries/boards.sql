-- name: NextBoardUrutan :one
SELECT (COALESCE(max(urutan), 0) + 1024)::double precision FROM boards
WHERE project_id = $1 AND dihapus_pada IS NULL;

-- name: CreateBoard :one
INSERT INTO boards (project_id, nama, urutan) VALUES ($1, $2, $3) RETURNING *;

-- name: ListBoards :many
SELECT * FROM boards WHERE project_id = $1 AND dihapus_pada IS NULL ORDER BY urutan, id;

-- name: GetBoard :one
SELECT * FROM boards WHERE id = $1 AND dihapus_pada IS NULL;

-- name: GetBoardForUpdate :one
SELECT * FROM boards WHERE id = $1 AND dihapus_pada IS NULL FOR UPDATE;

-- name: UpdateBoard :one
UPDATE boards SET nama = $2, urutan = $3, diubah_pada = now() WHERE id = $1 RETURNING *;

-- name: SoftDeleteBoard :exec
UPDATE boards SET dihapus_pada = now(), diubah_pada = now() WHERE id = $1;

-- name: NextColumnUrutan :one
SELECT (COALESCE(max(urutan), 0) + 1024)::double precision FROM board_columns
WHERE board_id = $1 AND dihapus_pada IS NULL;

-- name: CreateColumn :one
INSERT INTO board_columns (board_id, tipe, judul, urutan, konfigurasi_json)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: ListColumns :many
SELECT * FROM board_columns WHERE board_id = $1 AND dihapus_pada IS NULL ORDER BY urutan, id;

-- name: GetColumnWithProject :one
SELECT sqlc.embed(c), b.project_id
FROM board_columns c
JOIN boards b ON b.id = c.board_id AND b.dihapus_pada IS NULL
WHERE c.id = $1 AND c.dihapus_pada IS NULL;

-- name: GetColumnForUpdate :one
SELECT * FROM board_columns WHERE id = $1 AND dihapus_pada IS NULL FOR UPDATE;

-- name: UpdateColumn :one
UPDATE board_columns SET judul = $2, urutan = $3, konfigurasi_json = $4 WHERE id = $1 RETURNING *;

-- name: SoftDeleteColumn :exec
UPDATE board_columns SET dihapus_pada = now() WHERE id = $1;

-- name: CountStatusValuesOutside :one
-- Jumlah nilai status pada item aktif yang labelnya tidak ada di daftar.
SELECT count(*) FROM item_values v
JOIN items i ON i.id = v.item_id AND i.dihapus_pada IS NULL
WHERE v.column_id = @column_id AND NOT ((v.nilai_json #>> '{}') = ANY (@labels::text[]));

-- name: NextGroupUrutan :one
SELECT (COALESCE(max(urutan), 0) + 1024)::double precision FROM groups
WHERE board_id = $1 AND dihapus_pada IS NULL;

-- name: CreateGroup :one
INSERT INTO groups (board_id, judul, warna, urutan) VALUES ($1, $2, $3, $4) RETURNING *;

-- name: ListGroups :many
SELECT * FROM groups WHERE board_id = $1 AND dihapus_pada IS NULL ORDER BY urutan, id;

-- name: GetGroupWithProject :one
SELECT sqlc.embed(g), b.project_id
FROM groups g
JOIN boards b ON b.id = g.board_id AND b.dihapus_pada IS NULL
WHERE g.id = $1 AND g.dihapus_pada IS NULL;

-- name: GetGroupForUpdate :one
SELECT * FROM groups WHERE id = $1 AND dihapus_pada IS NULL FOR UPDATE;

-- name: GetGroupForShare :one
SELECT * FROM groups WHERE id = $1 AND dihapus_pada IS NULL FOR SHARE;

-- name: UpdateGroup :one
UPDATE groups SET judul = $2, warna = $3, urutan = $4 WHERE id = $1 RETURNING *;

-- name: SoftDeleteGroup :exec
UPDATE groups SET dihapus_pada = now() WHERE id = $1;

-- name: CountActiveGroups :one
SELECT count(*) FROM groups WHERE board_id = $1 AND dihapus_pada IS NULL;

-- name: CountActiveItemsInGroup :one
SELECT count(*) FROM items WHERE group_id = $1 AND dihapus_pada IS NULL;

-- name: ListProjectMemberIDs :many
SELECT user_id FROM project_members WHERE project_id = $1;
