param([string]$InstallDir = "")
$ErrorActionPreference = 'SilentlyContinue'
if ([string]::IsNullOrWhiteSpace($InstallDir)) {
  $InstallDir = Split-Path -Parent $PSScriptRoot
}
$InstallDir = [IO.Path]::GetFullPath($InstallDir)
Stop-ScheduledTask -TaskName 'FaceSign'
Get-Process -Name 'FaceSign' -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue
Unregister-ScheduledTask -TaskName 'FaceSign' -Confirm:$false
Get-NetFirewallRule -DisplayName 'FaceSign Web' -ErrorAction SilentlyContinue | Remove-NetFirewallRule -ErrorAction SilentlyContinue
Get-NetFirewallRule -DisplayName 'FaceSign HTTP' -ErrorAction SilentlyContinue | Remove-NetFirewallRule -ErrorAction SilentlyContinue
Get-NetFirewallRule -DisplayName 'FaceSign HTTPS' -ErrorAction SilentlyContinue | Remove-NetFirewallRule -ErrorAction SilentlyContinue
$shortcut = Join-Path ([Environment]::GetFolderPath('CommonPrograms')) 'FaceSign\FaceSign 管理工具.lnk'
Remove-Item $shortcut -Force -ErrorAction SilentlyContinue
Write-Host "FaceSign startup task, management shortcut and firewall rules removed."
Write-Host "Data remains in $InstallDir\data."
Write-Host "The persistent TLS identity remains in $InstallDir\tls so clients do not lose trust if FaceSign is reinstalled."
