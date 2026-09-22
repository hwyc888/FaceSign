param(
  [string]$InstallDir = "$env:ProgramData\FaceSign",
  [string]$Listen = "0.0.0.0:8080"
)
$ErrorActionPreference = 'Stop'
$source = Split-Path -Parent $PSScriptRoot

$ModelCommit = 'de5287c66e9e37e9f804686bf63f5a0974f68f72'
$ModelBaseUrl = "https://raw.githubusercontent.com/hwyc888/FaceSign/$ModelCommit/models"
$Models = @(
  @{
    Name = 'face_detection_yunet_2023mar.onnx'
    SHA256 = '8F2383E4DD3CFBB4553EA8718107FC0423210DC964F9F4280604804ED2552FA4'
  },
  @{
    Name = 'face_recognition_sface_2021dec.onnx'
    SHA256 = '0BA9FBFA01B5270C96627C4EF784DA859931E02F04419C829E83484087C34E79'
  },
  @{
    Name = 'anti-spoof-mn3.onnx'
    SHA256 = 'C4C99AF04603B62D7E44F6F4DAEB33E0DAECCC696008C0B1D62F6F5CEBBB3262'
  }
)

function Test-ModelFile {
  param(
    [string]$Path,
    [string]$SHA256
  )
  if (-not (Test-Path $Path -PathType Leaf)) { return $false }
  return (Get-FileHash $Path -Algorithm SHA256).Hash -eq $SHA256
}

function Ensure-FaceModels {
  param([string]$TargetDir)

  New-Item -ItemType Directory -Force -Path $TargetDir | Out-Null
  $sourceModels = Join-Path $source 'models'

  foreach ($model in $Models) {
    $target = Join-Path $TargetDir $model.Name
    if (Test-ModelFile -Path $target -SHA256 $model.SHA256) {
      Write-Host "Model ready: $($model.Name)"
      continue
    }

    $sourceModel = Join-Path $sourceModels $model.Name
    if (Test-ModelFile -Path $sourceModel -SHA256 $model.SHA256) {
      Copy-Item $sourceModel $target -Force
      Write-Host "Model installed from local models folder: $($model.Name)"
      continue
    }

    $url = "$ModelBaseUrl/$($model.Name)"
    $temp = "$target.download"
    Write-Host "Downloading model once: $($model.Name)"
    try {
      Invoke-WebRequest -Uri $url -OutFile $temp -UseBasicParsing
      if (-not (Test-ModelFile -Path $temp -SHA256 $model.SHA256)) {
        throw "Model checksum verification failed: $($model.Name)"
      }
      Move-Item $temp $target -Force
    } finally {
      Remove-Item $temp -Force -ErrorAction SilentlyContinue
    }
  }
}

# Stop every older FaceSign instance before replacing files.
$oldTask = Get-ScheduledTask -TaskName 'FaceSign' -ErrorAction SilentlyContinue
if ($oldTask) {
  Stop-ScheduledTask -TaskName 'FaceSign' -ErrorAction SilentlyContinue
  Start-Sleep -Milliseconds 500
  Unregister-ScheduledTask -TaskName 'FaceSign' -Confirm:$false -ErrorAction SilentlyContinue
}
Get-Process -Name 'FaceSign' -ErrorAction SilentlyContinue |
  Stop-Process -Force -ErrorAction SilentlyContinue
Start-Sleep -Milliseconds 700

$remaining = Get-Process -Name 'FaceSign' -ErrorAction SilentlyContinue
if ($remaining) {
  throw 'Old FaceSign.exe is still running. Close it and run this installer again.'
}

New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
Copy-Item (Join-Path $source 'FaceSign.exe') $InstallDir -Force
Copy-Item (Join-Path $source 'onnxruntime.dll') $InstallDir -Force
foreach ($dll in @('msvcp140.dll','msvcp140_1.dll','vcruntime140.dll','vcruntime140_1.dll','libgcc_s_seh-1.dll','libwinpthread-1.dll')) {
  $p = Join-Path $source $dll
  if (Test-Path $p) { Copy-Item $p $InstallDir -Force }
}

# Models are deliberately not included in every program build. Existing valid
# models are kept; a fresh machine downloads the fixed repository models once.
Ensure-FaceModels -TargetDir (Join-Path $InstallDir 'models')

New-Item -ItemType Directory -Force -Path (Join-Path $InstallDir 'data') | Out-Null
Remove-Item (Join-Path $InstallDir 'facesign-error.log') -Force -ErrorAction SilentlyContinue

$exe = Join-Path $InstallDir 'FaceSign.exe'
$db = Join-Path $InstallDir 'data\facesign.db'
$args = '--listen {0} --open-browser=false --assets "{1}" --data "{2}"' -f $Listen, $InstallDir, $db
$action = New-ScheduledTaskAction -Execute $exe -Argument $args
$trigger = New-ScheduledTaskTrigger -AtStartup
$principal = New-ScheduledTaskPrincipal -UserId 'SYSTEM' -LogonType ServiceAccount -RunLevel Highest
$settings = New-ScheduledTaskSettingsSet -RestartCount 3 -RestartInterval (New-TimeSpan -Minutes 1) -ExecutionTimeLimit ([TimeSpan]::Zero)
Register-ScheduledTask -TaskName 'FaceSign' -Action $action -Trigger $trigger -Principal $principal -Settings $settings -Force | Out-Null

$port = ($Listen -split ':')[-1]
if ($Listen.StartsWith('0.0.0.0:') -or $Listen.StartsWith(':')) {
  Get-NetFirewallRule -DisplayName 'FaceSign Web' -ErrorAction SilentlyContinue |
    Remove-NetFirewallRule -ErrorAction SilentlyContinue
  New-NetFirewallRule -DisplayName 'FaceSign Web' -Direction Inbound -Action Allow -Protocol TCP -LocalPort $port -Profile Domain,Private | Out-Null
}

Start-ScheduledTask -TaskName 'FaceSign'

$versionInfo = $null
for ($i = 0; $i -lt 20; $i++) {
  try {
    $versionInfo = Invoke-RestMethod -Uri "http://127.0.0.1:$port/api/version" -TimeoutSec 2
    if ($versionInfo.version) { break }
  } catch {
    Start-Sleep -Milliseconds 500
  }
}
if (-not $versionInfo.version) {
  $errorLog = Join-Path $InstallDir 'facesign-error.log'
  if (Test-Path $errorLog) {
    throw "FaceSign failed to start: $(Get-Content $errorLog -Raw)"
  }
  throw 'FaceSign failed to start. Check C:\ProgramData\FaceSign\data\facesign-startup.log and Windows Task Scheduler.'
}

$url = "http://127.0.0.1:$port/?v=$($versionInfo.version)"
Write-Host "FaceSign upgraded and started."
Write-Host "Version: $($versionInfo.version)"
Write-Host "Models:  pinned at $ModelCommit"
Write-Host "Local:   $url"
if ($Listen.StartsWith('0.0.0.0:') -or $Listen.StartsWith(':')) {
  Write-Host "LAN:     http://<this-PC-IP>:$port/"
}
Start-Process $url
