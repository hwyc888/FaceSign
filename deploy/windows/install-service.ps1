param(
    [string]$Binary = ".\facesign-windows-amd64.exe",
    [string]$InstallDir = "$env:ProgramFiles\FaceSign",
    [string]$DataDir = "$env:ProgramData\FaceSign"
)

$ErrorActionPreference = "Stop"
$identity = [Security.Principal.WindowsIdentity]::GetCurrent()
$principal = New-Object Security.Principal.WindowsPrincipal($identity)
if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    throw "Run PowerShell as Administrator."
}

$source = (Resolve-Path $Binary).Path
New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
New-Item -ItemType Directory -Force -Path $DataDir | Out-Null
$target = Join-Path $InstallDir "facesign.exe"
Copy-Item -Force $source $target

$envFile = Join-Path $InstallDir "facesign.env"
if (-not (Test-Path $envFile)) {
    $kioskKey = [Guid]::NewGuid().ToString("N")
    @"
FACESIGN_ADDR=:8080
FACESIGN_DB=$DataDir\facesign.db
FACESIGN_TIMEZONE=Asia/Shanghai
FACESIGN_SESSION_HOURS=12
FACESIGN_KIOSK_ACCESS_KEY=$kioskKey
FACESIGN_FACE_PROVIDER=disabled
FACESIGN_COMPREFACE_URL=http://127.0.0.1:8000
FACESIGN_COMPREFACE_API_KEY=
FACESIGN_FACE_SIMILARITY=0.78
FACESIGN_FACE_DETECTION_THRESHOLD=0.80
FACESIGN_TLS_CERT=
FACESIGN_TLS_KEY=
FACESIGN_COOKIE_SECURE=false
"@ | Set-Content -Encoding UTF8 $envFile
}

$existing = Get-Service -Name "FaceSign" -ErrorAction SilentlyContinue
if ($existing) {
    if ($existing.Status -ne "Stopped") { Stop-Service -Name "FaceSign" -Force }
    sc.exe delete FaceSign | Out-Null
    Start-Sleep -Seconds 1
}

New-Service -Name "FaceSign" -BinaryPathName ('"' + $target + '"') -DisplayName "FaceSign Student Attendance Server" -Description "FaceSign web and face attendance server" -StartupType Automatic | Out-Null
sc.exe failure FaceSign reset= 86400 actions= restart/5000/restart/10000/restart/30000 | Out-Null
Start-Service -Name "FaceSign"

Write-Host "FaceSign installed and started."
Write-Host "Configuration: $envFile"
Write-Host "Open: http://SERVER_IP:8080/"
