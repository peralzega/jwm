-- name: ListPortfolios :many
SELECT p.id, p.nama, p.dibuat_pada, p.diubah_pada,
       (SELECT count(*) FROM projects pr WHERE pr.portfolio_id = p.id AND pr.diarsipkan_pada IS NULL)::int AS jumlah_proyek
FROM portfolios p
ORDER BY lower(p.nama), p.id;

-- name: GetPortfolio :one
SELECT * FROM portfolios WHERE id = $1;

-- name: GetPortfolioForUpdate :one
SELECT * FROM portfolios WHERE id = $1 FOR UPDATE;

-- name: CreatePortfolio :one
INSERT INTO portfolios (nama, dibuat_oleh) VALUES ($1, $2) RETURNING *;

-- name: UpdatePortfolio :one
UPDATE portfolios SET nama = $2, diubah_pada = now() WHERE id = $1 RETURNING *;

-- name: DeletePortfolio :exec
DELETE FROM portfolios WHERE id = $1;

-- name: SetProjectManualStatus :one
UPDATE projects
SET status_manual = @status, status_manual_alasan = @alasan, status_manual_oleh = @oleh,
    status_manual_pada = now(), diubah_pada = now()
WHERE id = @id
RETURNING *;

-- name: ClearProjectManualStatus :one
UPDATE projects
SET status_manual = NULL, status_manual_alasan = NULL, status_manual_oleh = NULL,
    status_manual_pada = NULL, diubah_pada = now()
WHERE id = $1
RETURNING *;

-- name: ListDashboardProjects :many
-- Proyek aktif untuk dasbor portofolio. @semua: Admin dan PMO melihat semua;
-- selain itu hanya proyek tempat pengguna menjadi anggota.
SELECT p.id, p.nama, p.pemilik_id, o.nama AS pemilik_nama, p.mulai, p.selesai, p.prioritas,
       p.portfolio_id, pf.nama AS portfolio_nama,
       p.status_manual, p.status_manual_alasan, p.status_manual_pada, sm.nama AS status_manual_oleh_nama,
       pm.peran_proyek
FROM projects p
JOIN users o ON o.id = p.pemilik_id
LEFT JOIN portfolios pf ON pf.id = p.portfolio_id
LEFT JOIN users sm ON sm.id = p.status_manual_oleh
LEFT JOIN project_members pm ON pm.project_id = p.id AND pm.user_id = @user_id
WHERE p.diarsipkan_pada IS NULL
  AND (@semua::boolean OR pm.user_id IS NOT NULL)
ORDER BY lower(p.nama), p.id;

-- name: ListBoardsOfProjects :many
SELECT id, project_id, nama FROM boards
WHERE project_id = ANY (@project_ids::uuid[]) AND dihapus_pada IS NULL
ORDER BY project_id, urutan, id;

-- name: ListDependenciesOfBoards :many
SELECT d.id, d.board_id, d.predecessor_id, d.successor_id, d.lag_hari FROM dependencies d
JOIN items p ON p.id = d.predecessor_id AND p.dihapus_pada IS NULL
JOIN items s ON s.id = d.successor_id AND s.dihapus_pada IS NULL
WHERE d.board_id = ANY (@board_ids::uuid[])
ORDER BY d.id;

-- name: ListColumnsOfBoards :many
SELECT id, board_id, tipe FROM board_columns
WHERE board_id = ANY (@board_ids::uuid[]) AND dihapus_pada IS NULL
  AND tipe IN ('status', 'progress', 'person', 'timeline')
ORDER BY board_id, urutan, id;

-- name: ListPortfolioItems :many
-- Satu baris per item aktif dengan nilai kolom pertama tiap tipe yang dipakai
-- aturan risiko, sudah sebagai skalar, dalam satu kali lewat item_values.
-- Larik kolom: kolom terpilih semua board per tipe; tiap item paling banyak
-- cocok satu kolom per tipe. Kolom dicocokkan lewat hash join, bukan ANY per baris.
-- Nilai kosong: status '', progres -1, jumlah_person 0, tanggal ''.
SELECT i.id, i.board_id, i.judul,
       COALESCE(max(v.nilai_json #>> '{}') FILTER (WHERE pk.kind = 'status'), '')::text AS status,
       COALESCE(max((v.nilai_json #>> '{}')::float8)
           FILTER (WHERE pk.kind = 'progress' AND jsonb_typeof(v.nilai_json) = 'number'), -1)::float8 AS progres,
       COALESCE(max(jsonb_array_length(v.nilai_json))
           FILTER (WHERE pk.kind = 'person' AND jsonb_typeof(v.nilai_json) = 'array'), 0)::int AS jumlah_person,
       COALESCE(max(v.nilai_json ->> 'mulai') FILTER (WHERE pk.kind = 'timeline'), '')::text AS mulai,
       COALESCE(max(v.nilai_json ->> 'selesai') FILTER (WHERE pk.kind = 'timeline'), '')::text AS selesai
FROM items i
LEFT JOIN (
    item_values v
    JOIN (
        SELECT unnest(@status_cols::uuid[]) AS col, 'status'::text AS kind
        UNION ALL SELECT unnest(@progress_cols::uuid[]), 'progress'
        UNION ALL SELECT unnest(@person_cols::uuid[]), 'person'
        UNION ALL SELECT unnest(@timeline_cols::uuid[]), 'timeline'
    ) AS pk ON pk.col = v.column_id
) ON v.item_id = i.id
WHERE i.board_id = ANY (@board_ids::uuid[]) AND i.dihapus_pada IS NULL
GROUP BY i.id;

-- name: SetDashboardWorkMem :exec
-- Agregasi dasbor (hingga 50 ribu item) muat di memori tanpa tumpah ke disk.
SELECT set_config('work_mem', '32MB', true);
