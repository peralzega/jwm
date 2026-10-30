-- name: GetUserByEmail :one
SELECT * FROM users WHERE lower(email) = lower(@email::text);

-- name: GetUserByID :one
SELECT * FROM users WHERE id = $1;

-- name: CreateUser :one
INSERT INTO users (nama, email, password_hash, peran, wajib_ganti_sandi)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: CountActiveAdmins :one
SELECT count(*) FROM users WHERE peran = 'admin' AND aktif;

-- name: CountAdmins :one
SELECT count(*) FROM users WHERE peran = 'admin';

-- name: IncrementLoginFailures :one
UPDATE users
SET gagal_login = gagal_login + 1, diubah_pada = now()
WHERE id = $1
RETURNING gagal_login;

-- name: LockUserUntil :exec
UPDATE users SET terkunci_hingga = $2, diubah_pada = now() WHERE id = $1;

-- name: RecordLoginSuccess :exec
UPDATE users
SET gagal_login = 0, terkunci_hingga = NULL, login_terakhir = now(), diubah_pada = now()
WHERE id = $1;

-- name: UpdatePassword :exec
UPDATE users
SET password_hash = $2, wajib_ganti_sandi = $3, diubah_pada = now()
WHERE id = $1;

-- name: AdvisoryXactLock :exec
SELECT pg_advisory_xact_lock($1);

-- name: ListUsers :many
SELECT * FROM users
WHERE (sqlc.narg(q)::text IS NULL
       OR nama ILIKE '%' || sqlc.narg(q) || '%'
       OR email ILIKE '%' || sqlc.narg(q) || '%')
  AND (sqlc.narg(peran)::text IS NULL OR peran = sqlc.narg(peran))
  AND (sqlc.narg(aktif)::boolean IS NULL OR aktif = sqlc.narg(aktif))
ORDER BY lower(nama), id
LIMIT 500;

-- name: SearchActiveUsers :many
SELECT id, nama, email FROM users
WHERE aktif
  AND (nama ILIKE '%' || @q::text || '%' OR email ILIKE '%' || @q::text || '%')
ORDER BY lower(nama), id
LIMIT 20;

-- name: GetUserForUpdate :one
SELECT * FROM users WHERE id = $1 FOR UPDATE;

-- name: LockActiveAdmins :many
-- Mengunci baris semua Admin aktif (urut id) untuk pemeriksaan "Admin aktif terakhir".
SELECT id FROM users WHERE peran = 'admin' AND aktif ORDER BY id FOR UPDATE;

-- name: UpdateUserProfile :one
UPDATE users SET nama = $2, email = $3, peran = $4, diubah_pada = now()
WHERE id = $1
RETURNING *;

-- name: SetUserActive :one
UPDATE users SET aktif = $2, diubah_pada = now()
WHERE id = $1
RETURNING *;

-- name: AdminResetPassword :one
UPDATE users
SET password_hash = $2, wajib_ganti_sandi = true, gagal_login = 0, terkunci_hingga = NULL, diubah_pada = now()
WHERE id = $1
RETURNING *;

-- name: UpdateUserDensity :one
UPDATE users SET kepadatan = $2, diubah_pada = now()
WHERE id = $1
RETURNING *;

-- name: UnlockUser :one
UPDATE users SET gagal_login = 0, terkunci_hingga = NULL, diubah_pada = now()
WHERE id = $1
RETURNING *;
