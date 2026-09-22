param([string]$InstallDir = "$env:ProgramData\FaceSign")
$ErrorActionPreference = 'SilentlyContinue'
Stop-ScheduledTask -TaskName 'FaceSign'
Unregister-ScheduledTask -TaskName 'FaceSign' -Confirm:$false
Get-NetFirewallRule -DisplayName 'FaceSign Web' -ErrorAction SilentlyContinue | Remove-NetFirewallRule -ErrorAction SilentlyContinue
Get-NetFirewallRule -DisplayName 'FaceSign HTTP' -ErrorAction SilentlyContinue | Remove-NetFirewallRule -ErrorAction SilentlyContinue
Get-NetFirewallRule -DisplayName 'FaceSign HTTPS' -ErrorAction SilentlyContinue | Remove-NetFirewallRule -ErrorAction SilentlyContinue
Write-Host "FaceSign startup task and firewall rules removed."
Write-Host "Data remains in $InstallDir\data."
Write-Host "The persistent TLS identity remains in $InstallDir\tls so clients do not lose trust if FaceSign is reinstalled."
