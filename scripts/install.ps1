param(
  [string]$InstallDir = "$env:ProgramData\FaceSign",
  [string]$Listen = "127.0.0.1:8080"
)
$ErrorActionPreference = 'Stop'
$source = Split-Path -Parent $PSScriptRoot
New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
Copy-Item (Join-Path $source 'FaceSign.exe') $InstallDir -Force
Copy-Item (Join-Path $source 'onnxruntime.dll') $InstallDir -Force
foreach ($dll in @('msvcp140.dll','msvcp140_1.dll','vcruntime140.dll','vcruntime140_1.dll','libgcc_s_seh-1.dll','libwinpthread-1.dll')) {
  $p = Join-Path $source $dll
  if (Test-Path $p) { Copy-Item $p $InstallDir -Force }
}
Copy-Item (Join-Path $source 'models') $InstallDir -Recurse -Force
New-Item -ItemType Directory -Force -Path (Join-Path $InstallDir 'data') | Out-Null
$exe = Join-Path $InstallDir 'FaceSign.exe'
$args = "--listen $Listen --assets `"$InstallDir`" --data `"$InstallDir\data\facesign.db`""
$action = New-ScheduledTaskAction -Execute $exe -Argument $args
$trigger = New-ScheduledTaskTrigger -AtStartup
$principal = New-ScheduledTaskPrincipal -UserId 'SYSTEM' -LogonType ServiceAccount -RunLevel Highest
$settings = New-ScheduledTaskSettingsSet -RestartCount 3 -RestartInterval (New-TimeSpan -Minutes 1) -ExecutionTimeLimit ([TimeSpan]::Zero)
Register-ScheduledTask -TaskName 'FaceSign' -Action $action -Trigger $trigger -Principal $principal -Settings $settings -Force | Out-Null
Start-ScheduledTask -TaskName 'FaceSign'
Write-Host "FaceSign installed. Open http://127.0.0.1:8080/ on this PC."
