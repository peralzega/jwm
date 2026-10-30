# Membuat proyek demo berisi banyak item untuk uji beban tabel dan Kanban.
# Hanya untuk pengembangan; ditolak bila APP_ENV=production.
# Contoh: .\scripts\seed-demo.ps1 -Items 5000 -Email admin@jiep.co.id
param(
    [int]$Items = 5000,
    [int]$Groups = 5,
    [string]$Email = '',
    [string]$Nama = ''
)
$ErrorActionPreference = 'Stop'
. "$PSScriptRoot\env.ps1"
Set-Location (Split-Path $PSScriptRoot -Parent)

$demoArgs = @('-items', $Items, '-groups', $Groups)
if ($Email) { $demoArgs += @('-email', $Email) }
if ($Nama) { $demoArgs += @('-nama', $Nama) }
go run ./cmd/seed-demo @demoArgs
exit $LASTEXITCODE
