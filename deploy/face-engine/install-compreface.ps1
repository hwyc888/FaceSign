param(
    [string]$Version = "1.2.0",
    [string]$InstallDir = (Join-Path $PSScriptRoot "compreface"),
    [string]$ServiceUrl = "http://127.0.0.1:8000",
    [string]$FaceSignBinary = (Join-Path (Split-Path $PSScriptRoot -Parent) "facesign.exe"),
    [string]$FaceSignEnvFile = ""
)

$ErrorActionPreference = "Stop"
$DownloadUrl = "https://github.com/exadel-inc/CompreFace/releases/download/v$Version/CompreFace_$Version.zip"
$CredentialsFile = Join-Path $InstallDir "facesign-admin.json"
$GeneratedEnvFile = Join-Path $InstallDir "facesign-face.env"
$Utf8NoBom = New-Object System.Text.UTF8Encoding($false)

function Write-Utf8NoBom {
    param([string]$Path, [string]$Content)
    [System.IO.File]::WriteAllText($Path, $Content, $script:Utf8NoBom)
}

function Set-EnvValue {
    param([string]$Path, [string]$Name, [string]$Value)
    $Lines = if (Test-Path $Path) { [System.IO.File]::ReadAllLines($Path) } else { @() }
    $Found = $false
    $Updated = foreach ($Line in $Lines) {
        if ($Line -match ('^' + [Regex]::Escape($Name) + '=')) {
            $Found = $true
            "$Name=$Value"
        } else {
            $Line
        }
    }
    if (-not $Found) {
        $Updated = @($Updated) + "$Name=$Value"
    }
    Write-Utf8NoBom -Path $Path -Content (($Updated -join "`n") + "`n")
}

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

Write-Host "[FaceSign] Checking Docker and release files..." -ForegroundColor Cyan
if (-not (Test-Path $FaceSignBinary -PathType Leaf)) {
    throw "FaceSign executable was not found: $FaceSignBinary. Run this script from a complete Windows release package."
}
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
    Invoke-Compose -Arguments @("up", "-d")
} finally {
    Pop-Location
}

Write-Host "[FaceSign] Waiting for the CompreFace admin API (first startup may take several minutes)..." -ForegroundColor Cyan
$Ready = $false
for ($i = 0; $i -lt 180; $i++) {
    try {
        $Response = Invoke-WebRequest -Uri "$ServiceUrl/admin/config" -UseBasicParsing -TimeoutSec 5
        if ($Response.StatusCode -ge 200 -and $Response.StatusCode -lt 300) {
            $Ready = $true
            break
        }
    } catch {
        Start-Sleep -Seconds 2
    }
}
if (-not $Ready) {
    throw "CompreFace containers started, but the admin API did not become ready within six minutes. Check docker compose logs."
}

if (Test-Path $CredentialsFile) {
    $Credentials = Get-Content -Raw $CredentialsFile | ConvertFrom-Json
} else {
    $Email = if ($env:COMPREFACE_ADMIN_EMAIL) { $env:COMPREFACE_ADMIN_EMAIL } else { "facesign@local.invalid" }
    $Password = if ($env:COMPREFACE_ADMIN_PASSWORD) { $env:COMPREFACE_ADMIN_PASSWORD } else { [Guid]::NewGuid().ToString("N") + [Guid]::NewGuid().ToString("N") }
    $Credentials = [PSCustomObject]@{ email = $Email; password = $Password }
    New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
    Write-Utf8NoBom -Path $CredentialsFile -Content ($Credentials | ConvertTo-Json)
    & icacls $CredentialsFile /inheritance:r /grant:r "*S-1-5-18:(F)" "*S-1-5-32-544:(F)" *> $null
}

Write-Host "[FaceSign] Creating or reusing the CompreFace application and recognition service..." -ForegroundColor Cyan
$PreviousEmail = $env:COMPREFACE_ADMIN_EMAIL
$PreviousPassword = $env:COMPREFACE_ADMIN_PASSWORD
try {
    $env:COMPREFACE_ADMIN_EMAIL = $Credentials.email
    $env:COMPREFACE_ADMIN_PASSWORD = $Credentials.password
    $ProvisionOutput = & $FaceSignBinary provision-compreface --url $ServiceUrl --email $Credentials.email 2>&1
    $ProvisionExitCode = $LASTEXITCODE
} finally {
    $env:COMPREFACE_ADMIN_EMAIL = $PreviousEmail
    $env:COMPREFACE_ADMIN_PASSWORD = $PreviousPassword
}
if ($ProvisionExitCode -ne 0) {
    $ProvisionOutput | ForEach-Object { Write-Host $_ -ForegroundColor Red }
    throw "Automatic CompreFace setup failed. If this instance already belongs to another account, remove $CredentialsFile, set COMPREFACE_ADMIN_EMAIL and COMPREFACE_ADMIN_PASSWORD to that administrator, and retry."
}
$ApiKey = [string]($ProvisionOutput | Select-Object -Last 1)
if ([string]::IsNullOrWhiteSpace($ApiKey)) {
    throw "Automatic CompreFace setup returned an empty API key."
}

$GeneratedEnv = @"
FACESIGN_FACE_PROVIDER=compreface
FACESIGN_COMPREFACE_URL=$ServiceUrl
FACESIGN_COMPREFACE_API_KEY=$ApiKey
"@
Write-Utf8NoBom -Path $GeneratedEnvFile -Content $GeneratedEnv

if (-not [string]::IsNullOrWhiteSpace($FaceSignEnvFile)) {
    Set-EnvValue -Path $FaceSignEnvFile -Name "FACESIGN_FACE_PROVIDER" -Value "compreface"
    Set-EnvValue -Path $FaceSignEnvFile -Name "FACESIGN_COMPREFACE_URL" -Value $ServiceUrl
    Set-EnvValue -Path $FaceSignEnvFile -Name "FACESIGN_COMPREFACE_API_KEY" -Value $ApiKey
}

Write-Host "[FaceSign] CompreFace is ready; the recognition service and API key were configured automatically." -ForegroundColor Green
Write-Host "[FaceSign] Administrator credentials: $CredentialsFile"
if (-not [string]::IsNullOrWhiteSpace($FaceSignEnvFile)) {
    Write-Host "[FaceSign] FaceSign configuration updated: $FaceSignEnvFile"
} else {
    Write-Host "[FaceSign] Importable configuration generated: $GeneratedEnvFile"
}
