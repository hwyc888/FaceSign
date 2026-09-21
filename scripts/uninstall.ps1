param([string]$InstallDir = "$env:ProgramData\FaceSign")
$ErrorActionPreference = 'SilentlyContinue'
Stop-ScheduledTask -TaskName 'FaceSign'
Unregister-ScheduledTask -TaskName 'FaceSign' -Confirm:$false
Write-Host "FaceSign startup task removed. Data remains in $InstallDir\data."
