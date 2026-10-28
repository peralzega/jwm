# Menjalankan perintah goose terhadap DATABASE_URL.
# Contoh: .\scripts\migrate.ps1            (up)
#         .\scripts\migrate.ps1 status
#         .\scripts\migrate.ps1 down
param([string]$Command = 'up')
$ErrorActionPreference = 'Stop'
. "$PSScriptRoot\env.ps1"
Set-Location (Split-Path $PSScriptRoot -Parent)

if (-not $env:DATABASE_URL) { Write-Error 'DATABASE_URL belum diisi (lihat .env.example).' }
go tool goose -dir internal/db/migrations postgres $env:DATABASE_URL $Command
exit $LASTEXITCODE
