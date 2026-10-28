-- name: InsertAudit :exec
INSERT INTO audit_log (aktor_id, entitas, entitas_id, aksi, sebelum, sesudah, ip)
VALUES ($1, $2, $3, $4, $5, $6, $7);

-- name: ListAuditByEntity :many
SELECT * FROM audit_log
WHERE entitas = $1 AND entitas_id = $2
ORDER BY id;
