# Menjalankan API (port dari HTTP_ADDR, default 8080) dan frontend Vite (http://localhost:5173).
# Buka http://localhost:5173 di peramban. Tekan Ctrl+C untuk menghentikan keduanya.
$ErrorActionPreference = 'Stop'
. "$PSScriptRoot\env.ps1"
$root = Split-Path $PSScriptRoot -Parent
Set-Location $root

if (-not (Test-Path 'web/node_modules')) {
    Push-Location web
    try { npm ci; if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE } } finally { Pop-Location }
}

# Bangun biner dahulu agar prosesnya bisa dihentikan bersih (go run membuat proses anak).
$exe = Join-Path $root 'bin\jwm-api.exe'
go build -o $exe ./cmd/api
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

$api = Start-Process -FilePath $exe -NoNewWindow -PassThru
try {
    Push-Location web
    npm run dev
} finally {
    Pop-Location
    if (-not $api.HasExited) { Stop-Process -Id $api.Id -Force }
}
