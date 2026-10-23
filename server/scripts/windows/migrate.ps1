param(
    [ValidateSet("up", "up-by-one", "down", "version", "status", "new")]
    [string]$Command = "up",
    [Parameter(Position = 1)]
    [string]$Name
)

$ErrorActionPreference = "Stop"
$scriptsRoot = Split-Path -Parent $PSScriptRoot
$serverRoot = Split-Path -Parent $scriptsRoot

Push-Location $serverRoot
try {
    $arguments = @($Command)
    if ($Name) {
        $arguments += $Name
    }
    & go run ./cmd/migrate @arguments
    if ($LASTEXITCODE -ne 0) {
        exit $LASTEXITCODE
    }
}
finally {
    Pop-Location
}
