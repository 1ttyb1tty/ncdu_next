# Tier 2 gate (Windows). Mirrors `make check`. Run from the repo root.
# Usage:
#   powershell -ExecutionPolicy Bypass -File scripts/check.ps1          # full
#   powershell -ExecutionPolicy Bypass -File scripts/check.ps1 -Fast    # tier 1
param([switch]$Fast)

$ErrorActionPreference = 'Stop'

function Fail([string]$msg) {
    Write-Host "check FAILED: $msg" -ForegroundColor Red
    exit 1
}

if (-not (Test-Path -LiteralPath 'go.mod')) {
    Fail "wrong working directory (no go.mod): $PWD"
}

$fmt = gofmt -l .
if ($fmt) {
    Fail "gofmt needed:`n$($fmt -join "`n")"
}

go vet ./...
if ($LASTEXITCODE -ne 0) { Fail 'go vet failed' }

if ($Fast) {
    # Whole packages, not a name regex, so a guarded test cannot be excluded
    # by a stale selector.
    go test ./internal/filter/ ./internal/ui/
    if ($LASTEXITCODE -ne 0) { Fail 'focused tests failed' }
    Write-Host 'check-fast OK' -ForegroundColor Green
    exit 0
}

go build ./...
if ($LASTEXITCODE -ne 0) { Fail 'go build failed' }

go test -count=1 -race ./...
if ($LASTEXITCODE -ne 0) { Fail 'go test -race failed' }

Write-Host 'check OK' -ForegroundColor Green
