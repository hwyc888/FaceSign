param(
  [string]$InstallDir = "$env:ProgramData\FaceSignCameraAgent",
  [string]$ConfigPath = ".\camera-agent.json"
)
$ErrorActionPreference = 'Stop'
$source = Split-Path -Parent $PSScriptRoot

if (-not (Test-Path $ConfigPath -PathType Leaf)) {
  $example = Join-Path $source 'camera-agent.example.json'
  if (Test-Path $example) { Copy-Item $example (Join-Path $source 'camera-agent.json') -Force }
  throw 'camera-agent.json not found. Edit camera-agent.example.json, save it as camera-agent.json, then run install-camera-agent.ps1 again.'
}

$task = Get-ScheduledTask -TaskName 'FaceSignCameraAgent' -ErrorAction SilentlyContinue
if ($task) {
  Stop-ScheduledTask -TaskName 'FaceSignCameraAgent' -ErrorAction SilentlyContinue
  Unregister-ScheduledTask -TaskName 'FaceSignCameraAgent' -Confirm:$false -ErrorAction SilentlyContinue
}
Get-Process -Name 'FaceSignCameraAgent' -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue

New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
Copy-Item (Join-Path $source 'FaceSignCameraAgent.exe') $InstallDir -Force
Copy-Item $ConfigPath (Join-Path $InstallDir 'camera-agent.json') -Force
$configTarget = Join-Path $InstallDir 'camera-agent.json'
& icacls.exe $configTarget /inheritance:r /grant:r '*S-1-5-18:F' '*S-1-5-32-544:F' | Out-Null
if ($LASTEXITCODE -ne 0) { throw 'Failed to protect camera-agent.json ACL' }

$exe = Join-Path $InstallDir 'FaceSignCameraAgent.exe'
& $exe --config $configTarget --once
if ($LASTEXITCODE -ne 0) { throw 'Camera Agent connection test failed. Check server URL, Agent ID/key, and local camera settings.' }

$action = New-ScheduledTaskAction -Execute $exe -Argument ('--config "{0}"' -f $configTarget)
$trigger = New-ScheduledTaskTrigger -AtStartup
$principal = New-ScheduledTaskPrincipal -UserId 'SYSTEM' -LogonType ServiceAccount -RunLevel Highest
$settings = New-ScheduledTaskSettingsSet -RestartCount 5 -RestartInterval (New-TimeSpan -Minutes 1) -ExecutionTimeLimit ([TimeSpan]::Zero)
Register-ScheduledTask -TaskName 'FaceSignCameraAgent' -Action $action -Trigger $trigger -Principal $principal -Settings $settings -Force | Out-Null
Start-ScheduledTask -TaskName 'FaceSignCameraAgent'
Write-Host "FaceSign Camera Agent installed and started."
Write-Host "Config: $configTarget"
