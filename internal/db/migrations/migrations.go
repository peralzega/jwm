// Package migrations menyimpan berkas migrasi goose sebagai embed.FS.
package migrations

import "embed"

// FS berisi semua berkas *.sql migrasi.
//
//go:embed *.sql
var FS embed.FS
