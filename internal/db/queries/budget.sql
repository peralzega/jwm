-- name: ListBudgetEntries :many
SELECT e.id, e.project_id, to_char(e.tanggal, 'YYYY-MM-DD')::text AS tanggal, e.jenis, e.jumlah::text AS jumlah,
       e.keterangan, e.dibuat_pada, e.diubah_pada, u.nama AS dibuat_oleh_nama
FROM budget_entries e
JOIN users u ON u.id = e.dibuat_oleh
WHERE e.project_id = $1
ORDER BY e.tanggal DESC, e.id DESC;

-- name: GetBudgetEntryForUpdate :one
SELECT id, project_id, to_char(tanggal, 'YYYY-MM-DD')::text AS tanggal, jenis, jumlah::text AS jumlah, keterangan
FROM budget_entries WHERE id = $1 FOR UPDATE;

-- name: CreateBudgetEntry :one
INSERT INTO budget_entries (project_id, tanggal, jenis, jumlah, keterangan, dibuat_oleh)
VALUES (@project_id, CAST(@tanggal::text AS date), @jenis, CAST(@jumlah::text AS numeric), @keterangan, @dibuat_oleh)
RETURNING id;

-- name: UpdateBudgetEntry :exec
UPDATE budget_entries
SET tanggal = CAST(@tanggal::text AS date), jenis = @jenis, jumlah = CAST(@jumlah::text AS numeric),
    keterangan = @keterangan, diubah_oleh = @diubah_oleh, diubah_pada = now()
WHERE id = @id;

-- name: DeleteBudgetEntry :exec
DELETE FROM budget_entries WHERE id = $1;

-- name: BudgetTotals :many
-- Total anggaran dan realisasi per proyek (teks desimal agar tidak kehilangan presisi).
SELECT project_id,
       COALESCE(sum(jumlah) FILTER (WHERE jenis = 'anggaran'), 0)::text AS anggaran,
       COALESCE(sum(jumlah) FILTER (WHERE jenis = 'realisasi'), 0)::text AS realisasi,
       (COALESCE(sum(jumlah) FILTER (WHERE jenis = 'anggaran'), 0)
        - COALESCE(sum(jumlah) FILTER (WHERE jenis = 'realisasi'), 0))::text AS sisa,
       count(*)::int AS jumlah_entri
FROM budget_entries
WHERE project_id = ANY (@project_ids::uuid[])
GROUP BY project_id;
