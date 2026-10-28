# Menjalankan semua tes. Tes integrasi memakai TEST_DATABASE_URL bila diisi,
# selain itu container postgres:18 lewat Docker. Tanpa keduanya, tes integrasi di-skip.
$ErrorActionPreference = 'Stop'
. "$PSScriptRoot\env.ps1"
Set-Location (Split-Path $PSScriptRoot -Parent)

if (-not $env:TEST_DATABASE_URL -and -not (Get-Command docker -ErrorAction SilentlyContinue)) {
    Write-Warning 'TEST_DATABASE_URL kosong dan Docker tidak ditemukan: tes integrasi akan di-SKIP.'
}

go test -count=1 ./...
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

if (Test-Path 'web/package.json') {
    Push-Location web
    try { npx vitest run; if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE } } finally { Pop-Location }
}
