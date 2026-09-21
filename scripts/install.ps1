param(
  [string]$InstallDir = "$env:ProgramData\FaceSign",
  [string]$Listen = "0.0.0.0:8080"
)
$ErrorActionPreference = 'Stop'
$source = Split-Path -Parent $PSScriptRoot

# Stop every older FaceSign instance before replacing files. This prevents an
# old scheduled-task process from continuing to own port 8080 after upgrade.
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
Copy-Item (Join-Path $source 'models') $InstallDir -Recurse -Force
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
Write-Host "Local:   $url"
if ($Listen.StartsWith('0.0.0.0:') -or $Listen.StartsWith(':')) {
  Write-Host "LAN:     http://<this-PC-IP>:$port/"
}
Start-Process $url
