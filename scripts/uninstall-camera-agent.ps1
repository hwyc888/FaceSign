$ErrorActionPreference = 'Stop'
$InstallDir = "$env:ProgramData\FaceSignCameraAgent"
$task = Get-ScheduledTask -TaskName 'FaceSignCameraAgent' -ErrorAction SilentlyContinue
if ($task) {
  Stop-ScheduledTask -TaskName 'FaceSignCameraAgent' -ErrorAction SilentlyContinue
  Unregister-ScheduledTask -TaskName 'FaceSignCameraAgent' -Confirm:$false
}
Get-Process -Name 'FaceSignCameraAgent' -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue
Remove-Item (Join-Path $InstallDir 'FaceSignCameraAgent.exe') -Force -ErrorAction SilentlyContinue
Write-Host "FaceSign Camera Agent uninstalled. The local camera-agent.json was kept in $InstallDir."
