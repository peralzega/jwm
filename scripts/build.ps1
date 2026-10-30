# Membangun frontend (web/dist) dan biner API (bin/jwm-api.exe).
# Server API menyajikan web/dist (atau WEB_DIR) di origin yang sama dengan /api.
$ErrorActionPreference = 'Stop'
$root = Split-Path $PSScriptRoot -Parent
Set-Location $root

Push-Location web
try {
    npm ci
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
    npm run build
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
} finally {
    Pop-Location
}

go build -o (Join-Path $root 'bin\jwm-api.exe') ./cmd/api
exit $LASTEXITCODE
