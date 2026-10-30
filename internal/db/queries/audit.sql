-- name: InsertAudit :exec
INSERT INTO audit_log (aktor_id, entitas, entitas_id, aksi, sebelum, sesudah, ip)
VALUES ($1, $2, $3, $4, $5, $6, $7);

-- name: ListAuditByEntity :many
SELECT * FROM audit_log
WHERE entitas = $1 AND entitas_id = $2
ORDER BY id;

-- name: ListAudit :many
SELECT * FROM audit_log
WHERE (sqlc.narg(entitas)::text IS NULL OR entitas = sqlc.narg(entitas))
  AND (sqlc.narg(entitas_id)::text IS NULL OR entitas_id = sqlc.narg(entitas_id))
  AND (sqlc.narg(aktor_id)::uuid IS NULL OR aktor_id = sqlc.narg(aktor_id))
  AND (sqlc.narg(sebelum_id)::bigint IS NULL OR id < sqlc.narg(sebelum_id))
ORDER BY id DESC
LIMIT @batas;
