-- name: CreateProject :one
INSERT INTO projects (nama, pemilik_id, mulai, selesai, prioritas, dibuat_oleh)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetProjectForUpdate :one
SELECT * FROM projects WHERE id = $1 FOR UPDATE;

-- name: GetProjectAccess :one
-- Data akses segar untuk satu pengguna pada satu proyek. peran_proyek NULL bila bukan anggota.
SELECT u.peran, u.aktif, p.diarsipkan_pada, pm.peran_proyek
FROM users u
CROSS JOIN projects p
LEFT JOIN project_members pm ON pm.project_id = p.id AND pm.user_id = u.id
WHERE u.id = @user_id AND p.id = @project_id;

-- name: GetProjectView :one
SELECT p.*, o.nama AS pemilik_nama, pf.nama AS portfolio_nama
FROM projects p
JOIN users o ON o.id = p.pemilik_id
LEFT JOIN portfolios pf ON pf.id = p.portfolio_id
WHERE p.id = $1;

-- name: ListAllProjects :many
SELECT p.*, o.nama AS pemilik_nama, pm.peran_proyek
FROM projects p
JOIN users o ON o.id = p.pemilik_id
LEFT JOIN project_members pm ON pm.project_id = p.id AND pm.user_id = @user_id
WHERE (p.diarsipkan_pada IS NULL) <> @arsip::boolean
ORDER BY lower(p.nama), p.id;

-- name: ListMemberProjects :many
SELECT p.*, o.nama AS pemilik_nama, pm.peran_proyek
FROM projects p
JOIN users o ON o.id = p.pemilik_id
JOIN project_members pm ON pm.project_id = p.id AND pm.user_id = @user_id
WHERE (p.diarsipkan_pada IS NULL) <> @arsip::boolean
ORDER BY lower(p.nama), p.id;

-- name: UpdateProject :one
UPDATE projects
SET nama = $2, pemilik_id = $3, mulai = $4, selesai = $5, prioritas = $6, portfolio_id = $7, diubah_pada = now()
WHERE id = $1
RETURNING *;

-- name: SetProjectArchived :one
UPDATE projects
SET diarsipkan_pada = CASE WHEN @arsip::boolean THEN now() ELSE NULL END, diubah_pada = now()
WHERE id = @id
RETURNING *;

-- name: AddProjectMember :one
INSERT INTO project_members (project_id, user_id, peran_proyek, ditambahkan_oleh)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetProjectMember :one
SELECT * FROM project_members WHERE project_id = $1 AND user_id = $2;

-- name: ListProjectMembers :many
SELECT pm.user_id, pm.peran_proyek, pm.ditambahkan_pada, u.nama, u.email, u.peran, u.aktif
FROM project_members pm
JOIN users u ON u.id = pm.user_id
WHERE pm.project_id = $1
ORDER BY CASE pm.peran_proyek WHEN 'manager' THEN 0 WHEN 'member' THEN 1 ELSE 2 END, lower(u.nama), u.id;

-- name: UpdateProjectMemberRole :one
UPDATE project_members SET peran_proyek = $3
WHERE project_id = $1 AND user_id = $2
RETURNING *;

-- name: DeleteProjectMember :execrows
DELETE FROM project_members WHERE project_id = $1 AND user_id = $2;

-- name: CountProjectManagers :one
SELECT count(*) FROM project_members WHERE project_id = $1 AND peran_proyek = 'manager';
