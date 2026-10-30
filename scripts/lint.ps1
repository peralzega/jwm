# gofmt, go vet, golangci-lint (bila terpasang), govulncheck, lalu tsc dan eslint untuk web/.
$ErrorActionPreference = 'Stop'
Set-Location (Split-Path $PSScriptRoot -Parent)

$unformatted = gofmt -l cmd internal
if ($unformatted) {
    Write-Error "Berkas belum di-gofmt:`n$unformatted"
}

go vet ./...
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

if (Get-Command golangci-lint -ErrorAction SilentlyContinue) {
    golangci-lint run ./...
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
} else {
    Write-Warning 'golangci-lint tidak terpasang; dilewati. Lihat https://golangci-lint.run/welcome/install/'
}

go tool govulncheck ./...
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

if (Test-Path 'web/package.json') {
    Push-Location web
    try {
        npx tsc --noEmit
        if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
        npx eslint .
        if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
    } finally {
        Pop-Location
    }
}
