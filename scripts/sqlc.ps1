# Menghasilkan ulang kode query sqlc ke internal/db/gen.
$ErrorActionPreference = 'Stop'
Set-Location (Split-Path $PSScriptRoot -Parent)
go tool sqlc generate
exit $LASTEXITCODE
