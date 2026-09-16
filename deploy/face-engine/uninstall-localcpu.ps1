param(
    [string]$InstallDir = "$env:ProgramData\FaceSign\face-engine",
    [switch]$RemoveData
)

$ErrorActionPreference = "Stop"
$TaskName = "FaceSignFaceEngine"

$identity = [Security.Principal.WindowsIdentity]::GetCurrent()
$principal = New-Object Security.Principal.WindowsPrincipal($identity)
if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    throw "Run this script from an Administrator PowerShell window."
}

$task = Get-ScheduledTask -TaskName $TaskName -ErrorAction SilentlyContinue
if ($task) {
    Stop-ScheduledTask -TaskName $TaskName -ErrorAction SilentlyContinue
    Unregister-ScheduledTask -TaskName $TaskName -Confirm:$false
}

if (Test-Path $InstallDir) {
    if ($RemoveData) {
        Remove-Item -Path $InstallDir -Recurse -Force
    } else {
        Get-ChildItem -Path $InstallDir -Force | Where-Object { $_.Name -ne "data" } | Remove-Item -Recurse -Force
        Write-Host "Face samples were preserved at: $InstallDir\data" -ForegroundColor Yellow
    }
}

Write-Host "FaceSign local CPU face engine was uninstalled." -ForegroundColor Green
