param(
    [string]$Version = "1.2.0"
)

$ErrorActionPreference = "Stop"
$InstallDir = Join-Path $PSScriptRoot "compreface"
$DownloadUrl = "https://github.com/exadel-inc/CompreFace/releases/download/v$Version/CompreFace_$Version.zip"

function Invoke-Compose {
    param([string[]]$Arguments)
    if ($script:UseDockerPlugin) {
        & docker compose @Arguments
    } else {
        & docker-compose @Arguments
    }
    if ($LASTEXITCODE -ne 0) {
        throw "docker compose failed with exit code $LASTEXITCODE"
    }
}

Write-Host "[FaceSign] Checking Docker..." -ForegroundColor Cyan
if (-not (Get-Command docker -ErrorAction SilentlyContinue)) {
    throw "Docker was not found. Install and start Docker Desktop, then run this script again."
}
& docker info *> $null
if ($LASTEXITCODE -ne 0) {
    throw "Docker is not available. Start Docker Desktop first."
}

$script:UseDockerPlugin = $false
& docker compose version *> $null
if ($LASTEXITCODE -eq 0) {
    $script:UseDockerPlugin = $true
} elseif (-not (Get-Command docker-compose -ErrorAction SilentlyContinue)) {
    throw "Docker Compose was not found. Update Docker Desktop and try again."
}

if (-not (Test-Path (Join-Path $InstallDir "docker-compose.yml"))) {
    Write-Host "[FaceSign] Downloading official CompreFace $Version..." -ForegroundColor Cyan
    $TempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("facesign-compreface-" + [Guid]::NewGuid().ToString("N"))
    $ZipPath = Join-Path $TempRoot "compreface.zip"
    $ExtractDir = Join-Path $TempRoot "extract"
    New-Item -ItemType Directory -Force -Path $ExtractDir | Out-Null
    try {
        Invoke-WebRequest -Uri $DownloadUrl -OutFile $ZipPath -UseBasicParsing
        Expand-Archive -Path $ZipPath -DestinationPath $ExtractDir -Force
        $ComposeFile = Get-ChildItem -Path $ExtractDir -Filter "docker-compose.yml" -File -Recurse | Select-Object -First 1
        if (-not $ComposeFile) {
            throw "docker-compose.yml was not found in the downloaded archive."
        }
        New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
        Copy-Item -Path (Join-Path $ComposeFile.Directory.FullName "*") -Destination $InstallDir -Recurse -Force
    } finally {
        Remove-Item -Path $TempRoot -Recurse -Force -ErrorAction SilentlyContinue
    }
}

Write-Host "[FaceSign] Starting CompreFace..." -ForegroundColor Cyan
Push-Location $InstallDir
try {
    Invoke-Compose @("up", "-d")
} finally {
    Pop-Location
}

Write-Host "[FaceSign] Waiting for service startup (usually 30-90 seconds on first run)..." -ForegroundColor Cyan
$Ready = $false
for ($i = 0; $i -lt 45; $i++) {
    try {
        $Response = Invoke-WebRequest -Uri "http://127.0.0.1:8000/" -UseBasicParsing -TimeoutSec 3
        if ($Response.StatusCode -ge 200 -and $Response.StatusCode -lt 500) {
            $Ready = $true
            break
        }
    } catch {
        Start-Sleep -Seconds 2
    }
}

if ($Ready) {
    Write-Host "[FaceSign] CompreFace is ready: http://127.0.0.1:8000" -ForegroundColor Green
} else {
    Write-Warning "Containers started, but port 8000 is not ready yet. Open http://127.0.0.1:8000 later."
}

Write-Host ""
Write-Host "Next steps:" -ForegroundColor Yellow
Write-Host "1. Open http://127.0.0.1:8000 and finish CompreFace setup."
Write-Host "2. Create Application -> Face Recognition Service and copy the API Key."
Write-Host "3. In FaceSign -> Face Recognition Settings, select CompreFace, paste the API Key, save, and run service check."
