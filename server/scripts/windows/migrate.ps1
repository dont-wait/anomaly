param(
    [ValidateSet("up", "up-by-one", "down", "version", "status")]
    [string]$Command = "up"
)

$ErrorActionPreference = "Stop"
$scriptsRoot = Split-Path -Parent $PSScriptRoot
$serverRoot = Split-Path -Parent $scriptsRoot

Push-Location $serverRoot
try {
    & go run ./cmd/migrate $Command
    if ($LASTEXITCODE -ne 0) {
        exit $LASTEXITCODE
    }
}
finally {
    Pop-Location
}
