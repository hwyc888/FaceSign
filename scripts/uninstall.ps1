param([string]$InstallDir = "$env:ProgramData\FaceSign")
$ErrorActionPreference = 'SilentlyContinue'
Stop-ScheduledTask -TaskName 'FaceSign'
Unregister-ScheduledTask -TaskName 'FaceSign' -Confirm:$false
Get-NetFirewallRule -DisplayName 'FaceSign Web' -ErrorAction SilentlyContinue |
  Remove-NetFirewallRule -ErrorAction SilentlyContinue
Write-Host "FaceSign startup task and firewall rule removed. Data remains in $InstallDir\data."
