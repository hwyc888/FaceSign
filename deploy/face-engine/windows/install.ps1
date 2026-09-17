param(
    [string]$InstallDir = "$env:ProgramFiles\FaceSign\face-engine",
    [string]$DataDir = "$env:ProgramData\FaceSign\face-engine\data",
    [int]$Port = 18081
)

$ErrorActionPreference = "Stop"
$ServiceName = "FaceSignFaceEngine"
$LegacyTaskName = "FaceSignFaceEngine"

function Assert-Administrator {
    $identity = [Security.Principal.WindowsIdentity]::GetCurrent()
    $principal = New-Object Security.Principal.WindowsPrincipal($identity)
    if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
        $arguments = @(
            "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", ('"' + $PSCommandPath + '"'),
            "-InstallDir", ('"' + $InstallDir + '"'),
            "-DataDir", ('"' + $DataDir + '"'),
            "-Port", $Port
        )
        Start-Process powershell.exe -Verb RunAs -ArgumentList ($arguments -join ' ')
        exit 0
    }
}

Assert-Administrator

$SourceDir = $PSScriptRoot
$SourceExe = Join-Path $SourceDir "facesign-face-engine.exe"
$SourceOrt = Join-Path $SourceDir "onnxruntime.dll"
$SourceModels = Join-Path $SourceDir "models"

foreach ($required in @($SourceExe, $SourceOrt, $SourceModels)) {
    if (-not (Test-Path $required)) {
        throw "Release package is incomplete. Missing: $required"
    }
}

Write-Host "[FaceSign] Installing native CPU face engine..." -ForegroundColor Cyan

# Stop/remove the native service when upgrading.
$existing = Get-Service -Name $ServiceName -ErrorAction SilentlyContinue
if ($existing) {
    if ($existing.Status -ne "Stopped") {
        Stop-Service -Name $ServiceName -Force -ErrorAction SilentlyContinue
        $existing.WaitForStatus("Stopped", (New-TimeSpan -Seconds 20))
    }
    & sc.exe delete $ServiceName | Out-Null
    Start-Sleep -Seconds 1
}

# Remove the scheduled task used by the previous Python-based engine.
$legacyTask = Get-ScheduledTask -TaskName $LegacyTaskName -ErrorAction SilentlyContinue
if ($legacyTask) {
    Stop-ScheduledTask -TaskName $LegacyTaskName -ErrorAction SilentlyContinue
    Unregister-ScheduledTask -TaskName $LegacyTaskName -Confirm:$false -ErrorAction SilentlyContinue
}

New-Item -ItemType Directory -Force -Path $InstallDir, $DataDir | Out-Null
Copy-Item -Path $SourceExe -Destination (Join-Path $InstallDir "facesign-face-engine.exe") -Force
Copy-Item -Path $SourceOrt -Destination (Join-Path $InstallDir "onnxruntime.dll") -Force

foreach ($runtimeDll in @("msvcp140.dll", "msvcp140_1.dll", "vcruntime140.dll", "vcruntime140_1.dll", "libgcc_s_seh-1.dll", "libwinpthread-1.dll")) {
    $source = Join-Path $SourceDir $runtimeDll
    if (Test-Path $source) {
        Copy-Item -Path $source -Destination (Join-Path $InstallDir $runtimeDll) -Force
    }
}

$ModelTarget = Join-Path $InstallDir "models"
Remove-Item -Path $ModelTarget -Recurse -Force -ErrorAction SilentlyContinue
Copy-Item -Path $SourceModels -Destination $ModelTarget -Recurse -Force

# Clean only the obsolete runtime from the previous Python installer.
$LegacyRoot = Join-Path $env:ProgramData "FaceSign\face-engine"
foreach ($legacy in @("python", "models", "face_engine.py", "run-face-engine.cmd", "get-pip.py")) {
    Remove-Item -Path (Join-Path $LegacyRoot $legacy) -Recurse -Force -ErrorAction SilentlyContinue
}

$InstalledExe = Join-Path $InstallDir "facesign-face-engine.exe"
$Database = Join-Path $DataDir "faces.db"
$BinPath = '"' + $InstalledExe + '" --listen 127.0.0.1:' + $Port + ' --assets "' + $InstallDir + '" --data "' + $Database + '"'

New-Service -Name $ServiceName -BinaryPathName $BinPath -DisplayName "FaceSign Native CPU Face Engine" -StartupType Automatic | Out-Null
& sc.exe description $ServiceName "FaceSign CPU-only face recognition engine. No Python, Docker, CUDA or GPU required." | Out-Null
& sc.exe failure $ServiceName "reset= 86400" "actions= restart/5000/restart/15000/restart/30000" | Out-Null
Start-Service -Name $ServiceName

Write-Host "[FaceSign] Waiting for engine health check..." -ForegroundColor Cyan
$ready = $false
for ($i = 0; $i -lt 30; $i++) {
    try {
        $health = Invoke-RestMethod -Uri "http://127.0.0.1:$Port/health" -TimeoutSec 3
        if ($health.ok -and $health.device -eq "cpu") {
            $ready = $true
            break
        }
    } catch {
        Start-Sleep -Seconds 1
    }
}
if (-not $ready) {
    throw "FaceSignFaceEngine did not become ready. Run: sc.exe query FaceSignFaceEngine"
}

Write-Host ""
Write-Host "[FaceSign] Native CPU face engine is ready." -ForegroundColor Green
Write-Host "  Python: not required"
Write-Host "  Docker: not required"
Write-Host "  GPU/CUDA: not required"
Write-Host "  URL: http://127.0.0.1:$Port"
Write-Host "  Data: $Database"
Write-Host ""
Write-Host "In FaceSign -> Face Recognition, select Local CPU Engine and click Check Service." -ForegroundColor Yellow
