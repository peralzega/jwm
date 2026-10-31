-- name: CreateSession :one
INSERT INTO sessions (user_id, token_hash, kedaluwarsa, kedaluwarsa_mutlak, ip, user_agent)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetSessionWithUser :one
SELECT sqlc.embed(s), sqlc.embed(u)
FROM sessions s
JOIN users u ON u.id = s.user_id
WHERE s.token_hash = $1
  AND s.kedaluwarsa > now()
  AND s.kedaluwarsa_mutlak > now()
  AND u.aktif;

-- name: TouchSession :exec
UPDATE sessions
SET terakhir_aktif = now(), kedaluwarsa = least(@kedaluwarsa::timestamptz, kedaluwarsa_mutlak)
WHERE id = @id;

-- name: DeleteSession :exec
DELETE FROM sessions WHERE id = $1;

-- name: DeleteUserSessions :execrows
DELETE FROM sessions WHERE user_id = $1;

-- name: DeleteExpiredSessions :execrows
DELETE FROM sessions WHERE kedaluwarsa <= now() OR kedaluwarsa_mutlak <= now();

-- name: SessionStillValid :one
-- Sesi masih berlaku dan pengguna aktif, tanpa memperpanjang masa idle
-- (dipakai koneksi SSE yang berumur panjang).
SELECT EXISTS (
    SELECT 1 FROM sessions s JOIN users u ON u.id = s.user_id
    WHERE s.id = $1 AND s.kedaluwarsa > now() AND s.kedaluwarsa_mutlak > now() AND u.aktif
)::boolean;
