param([string]$Python = "python")
$ErrorActionPreference = "Stop"
$voiceRoot = Join-Path $PSScriptRoot "voice"
& $Python -m venv (Join-Path $voiceRoot ".venv")
if ($LASTEXITCODE -ne 0) { throw "Python 3.12+ is required. Pass its executable with -Python." }
$voicePython = Join-Path $voiceRoot ".venv\Scripts\python.exe"
& $voicePython -m pip install --disable-pip-version-check -r (Join-Path $voiceRoot "requirements-lock.txt")
if ($LASTEXITCODE -ne 0) { throw "Speech packages could not be installed." }
Push-Location $voiceRoot
try {
    npm.cmd ci
    if ($LASTEXITCODE -ne 0) { throw "Voice worker dependencies could not be installed." }
    $env:HF_HUB_DISABLE_TELEMETRY = "1"
    $env:HF_HUB_DISABLE_XET = "1"
    & $voicePython (Join-Path $voiceRoot "download_models.py")
    if ($LASTEXITCODE -ne 0) { throw "Speech models could not be downloaded." }
} finally { Pop-Location }
Write-Host "Local English voice is ready. Start Teamrise and enable voice in Discord settings."
