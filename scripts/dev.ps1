# Menjalankan server API untuk pengembangan. Frontend (web/) menyusul di M1e.
$ErrorActionPreference = 'Stop'
. "$PSScriptRoot\env.ps1"
Set-Location (Split-Path $PSScriptRoot -Parent)
go run ./cmd/api
