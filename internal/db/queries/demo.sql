-- Query khusus data demo (cmd/seed-demo). Tidak dipakai API.

-- name: InsertDemoItems :copyfrom
INSERT INTO items (id, board_id, group_id, judul, urutan, dibuat_oleh)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: InsertDemoItemValues :copyfrom
INSERT INTO item_values (item_id, column_id, nilai_json, diubah_oleh)
VALUES ($1, $2, $3, $4);
