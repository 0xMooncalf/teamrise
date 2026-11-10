$ErrorActionPreference = 'Stop'
Push-Location $PSScriptRoot
try {
    Push-Location (Join-Path $PSScriptRoot 'web')
    try {
        & npm.cmd ci
        if ($LASTEXITCODE -ne 0) { throw 'npm ci failed' }
        & npm.cmd run build
        if ($LASTEXITCODE -ne 0) { throw 'Frontend build failed' }
    } finally { Pop-Location }
    & go test -timeout 40s ./...
    if ($LASTEXITCODE -ne 0) { throw 'Go tests failed' }
    & go vet ./...
    if ($LASTEXITCODE -ne 0) { throw 'Go vet failed' }
    & go build -o teamrise.exe ./cmd/ladder
    if ($LASTEXITCODE -ne 0) { throw 'Go build failed' }
    Write-Host 'Built teamrise.exe. Run Start-Teamrise.cmd.'
} finally { Pop-Location }
