-- name: LockBoardDependencies :exec
-- Kunci transaksi per board agar dua penambahan bersamaan tidak membentuk siklus.
SELECT pg_advisory_xact_lock(hashtextextended('dependencies:' || @board_id::text, 0));

-- name: ListDependencies :many
-- Dependensi yang kedua itemnya masih aktif.
SELECT d.* FROM dependencies d
JOIN items p ON p.id = d.predecessor_id AND p.dihapus_pada IS NULL
JOIN items s ON s.id = d.successor_id AND s.dihapus_pada IS NULL
WHERE d.board_id = @board_id
ORDER BY d.id;

-- name: DependencyPathExists :one
-- Apakah @dari mencapai @ke lewat dependensi aktif (dari predecessor ke successor).
WITH RECURSIVE jangkau (id) AS (
    SELECT d.successor_id FROM dependencies d
    JOIN items s ON s.id = d.successor_id AND s.dihapus_pada IS NULL
    WHERE d.predecessor_id = @dari::uuid
    UNION
    SELECT d.successor_id FROM dependencies d
    JOIN jangkau j ON d.predecessor_id = j.id
    JOIN items s ON s.id = d.successor_id AND s.dihapus_pada IS NULL
)
SELECT EXISTS (SELECT 1 FROM jangkau WHERE id = @ke::uuid)::boolean;

-- name: CreateDependency :one
INSERT INTO dependencies (board_id, predecessor_id, successor_id, tipe, lag_hari, dibuat_oleh)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT ON CONSTRAINT dependencies_pasangan_unik DO NOTHING
RETURNING *;

-- name: GetDependencyForUpdate :one
SELECT * FROM dependencies WHERE id = $1 FOR UPDATE;

-- name: DeleteDependency :exec
DELETE FROM dependencies WHERE id = $1;
