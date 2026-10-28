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
