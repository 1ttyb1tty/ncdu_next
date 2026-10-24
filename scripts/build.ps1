# Build the runnable binary into bin/ and report the revision it embeds, so a
# stale artifact is detectable without relying on file times (see AGENTS.md,
# "Build artifact"). Run from the repo root.
#
# Usage:
#   powershell -ExecutionPolicy Bypass -File scripts/build.ps1
#   powershell -ExecutionPolicy Bypass -File scripts/build.ps1 -Out bin\other.exe
param(
    [string]$Out = 'bin\ncdu-next.exe'
)

$ErrorActionPreference = 'Stop'

if (-not (Test-Path -LiteralPath 'go.mod')) {
    Write-Error "wrong working directory (no go.mod): $PWD"
    exit 9
}

$dir = Split-Path -Parent $Out
if ($dir -and -not (Test-Path -LiteralPath $dir)) {
    New-Item -ItemType Directory -Path $dir | Out-Null
}

go build -trimpath -o $Out ./cmd/ncdu-next
if ($LASTEXITCODE -ne 0) { Write-Error 'go build failed'; exit 1 }

# The binary stamps the revision it was built from; comparing it with HEAD is the
# staleness check. A mismatch means the artifact does not reflect the tree.
$head = (git rev-parse HEAD).Trim()
$stamp = go version -m $Out | Select-String -Pattern 'vcs\.revision='
if (-not $stamp) {
    Write-Warning "no vcs.revision embedded in $Out (built outside a git checkout?)"
    exit 2
}
$rev = ($stamp.Line -split 'vcs\.revision=')[1].Trim()
if ($rev -ne $head) {
    Write-Warning "stale artifact: $Out embeds $rev, HEAD is $head"
    exit 3
}

Write-Host ("built {0} (HEAD {1})" -f $Out, $head.Substring(0, 7))
