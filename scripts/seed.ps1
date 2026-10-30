# Membuat Admin pertama dari BOOTSTRAP_ADMIN_EMAIL dan BOOTSTRAP_ADMIN_PASSWORD.
# Menolak berjalan bila Admin sudah ada. Kosongkan BOOTSTRAP_ADMIN_PASSWORD di .env setelahnya.
$ErrorActionPreference = 'Stop'
. "$PSScriptRoot\env.ps1"
Set-Location (Split-Path $PSScriptRoot -Parent)
go run ./cmd/seed
exit $LASTEXITCODE
