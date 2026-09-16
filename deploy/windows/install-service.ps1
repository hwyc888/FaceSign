param(
    [string]$Binary = (Join-Path $PSScriptRoot "facesign.exe"),
    [string]$InstallDir = "$env:ProgramFiles\FaceSign",
    [string]$DataDir = "$env:ProgramData\FaceSign",
    [switch]$SkipFaceEngine
)

$ErrorActionPreference = "Stop"
$Utf8NoBom = New-Object System.Text.UTF8Encoding($false)

function Write-Utf8NoBom {
    param([string]$Path, [string]$Content)
    [System.IO.File]::WriteAllText($Path, $Content, $script:Utf8NoBom)
}

$Identity = [Security.Principal.WindowsIdentity]::GetCurrent()
$Principal = New-Object Security.Principal.WindowsPrincipal($Identity)
if (-not $Principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    throw "Run PowerShell as Administrator."
}

$Source = (Resolve-Path $Binary).Path
New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
New-Item -ItemType Directory -Force -Path $DataDir | Out-Null
$Target = Join-Path $InstallDir "facesign.exe"
$EnvFile = Join-Path $InstallDir "facesign.env"

if (-not (Test-Path $EnvFile)) {
    $KioskKey = [Guid]::NewGuid().ToString("N")
    $InitialConfig = @"
FACESIGN_ADDR=:8080
FACESIGN_DB=$DataDir\facesign.db
FACESIGN_TIMEZONE=Asia/Shanghai
FACESIGN_SESSION_HOURS=12
FACESIGN_KIOSK_ACCESS_KEY=$KioskKey
FACESIGN_FACE_PROVIDER=disabled
FACESIGN_COMPREFACE_URL=http://127.0.0.1:8000
FACESIGN_COMPREFACE_API_KEY=
FACESIGN_FACE_SIMILARITY=0.78
FACESIGN_FACE_DETECTION_THRESHOLD=0.80
FACESIGN_TLS_CERT=
FACESIGN_TLS_KEY=
FACESIGN_COOKIE_SECURE=false
"@
    Write-Utf8NoBom -Path $EnvFile -Content $InitialConfig
}

if (-not $SkipFaceEngine) {
    $EngineScript = Join-Path $PSScriptRoot "face-engine\install-compreface.ps1"
    if (-not (Test-Path $EngineScript -PathType Leaf)) {
        throw "The release package is incomplete: $EngineScript is missing."
    }
    & $EngineScript `
        -InstallDir (Join-Path $DataDir "compreface") `
        -FaceSignBinary $Source `
        -FaceSignEnvFile $EnvFile
}

$Existing = Get-Service -Name "FaceSign" -ErrorAction SilentlyContinue
if ($Existing -and $Existing.Status -ne "Stopped") {
    Stop-Service -Name "FaceSign" -Force
    $Existing.WaitForStatus([System.ServiceProcess.ServiceControllerStatus]::Stopped, [TimeSpan]::FromSeconds(30))
}
Copy-Item -Force $Source $Target

$QuotedTarget = '"' + $Target + '"'
if ($Existing) {
    & sc.exe config FaceSign binPath= $QuotedTarget start= auto | Out-Null
    if ($LASTEXITCODE -ne 0) {
        throw "Unable to update the FaceSign Windows service."
    }
} else {
    New-Service -Name "FaceSign" -BinaryPathName $QuotedTarget -DisplayName "FaceSign Student Attendance Server" -Description "FaceSign web and face attendance server" -StartupType Automatic | Out-Null
}
& sc.exe failure FaceSign reset= 86400 actions= restart/5000/restart/10000/restart/30000 | Out-Null
Start-Service -Name "FaceSign"

Write-Host "[FaceSign] Waiting for the Windows service..." -ForegroundColor Cyan
$Ready = $false
for ($i = 0; $i -lt 30; $i++) {
    try {
        $Health = Invoke-RestMethod -Uri "http://127.0.0.1:8080/api/health" -TimeoutSec 3
        if ($Health.ok) {
            $Ready = $true
            break
        }
    } catch {
        Start-Sleep -Seconds 1
    }
}
if (-not $Ready) {
    throw "FaceSign Windows service did not pass its health check. Run Get-Service FaceSign and check the Windows Event Log."
}

if (-not $SkipFaceEngine) {
    $FaceStatus = Invoke-RestMethod -Uri "http://127.0.0.1:8080/api/kiosk/status" -TimeoutSec 10
    if (-not $FaceStatus.face_ready) {
        throw "FaceSign started, but face recognition self-check failed: $($FaceStatus.face_message)"
    }
}

if ($SkipFaceEngine) {
    Write-Host "[FaceSign] FaceSign installed; face recognition was skipped by request." -ForegroundColor Green
} else {
    Write-Host "[FaceSign] FaceSign and CompreFace installed and passed their health checks." -ForegroundColor Green
}
Write-Host "[FaceSign] Configuration: $EnvFile"
Write-Host "[FaceSign] Open: http://SERVER_IP:8080/"
