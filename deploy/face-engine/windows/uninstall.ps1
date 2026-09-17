param(
    [string]$InstallDir = "$env:ProgramFiles\FaceSign\face-engine",
    [string]$DataDir = "$env:ProgramData\FaceSign\face-engine\data",
    [switch]$RemoveData
)

$ErrorActionPreference = "Stop"
$ServiceName = "FaceSignFaceEngine"

$identity = [Security.Principal.WindowsIdentity]::GetCurrent()
$principal = New-Object Security.Principal.WindowsPrincipal($identity)
if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    $arguments = @(
        "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", ('"' + $PSCommandPath + '"'),
        "-InstallDir", ('"' + $InstallDir + '"'),
        "-DataDir", ('"' + $DataDir + '"')
    )
    if ($RemoveData) { $arguments += "-RemoveData" }
    Start-Process powershell.exe -Verb RunAs -ArgumentList ($arguments -join ' ')
    exit 0
}

$service = Get-Service -Name $ServiceName -ErrorAction SilentlyContinue
if ($service) {
    if ($service.Status -ne "Stopped") {
        Stop-Service -Name $ServiceName -Force -ErrorAction SilentlyContinue
    }
    & sc.exe delete $ServiceName | Out-Null
}

Remove-Item -Path $InstallDir -Recurse -Force -ErrorAction SilentlyContinue
if ($RemoveData) {
    Remove-Item -Path $DataDir -Recurse -Force -ErrorAction SilentlyContinue
    Write-Host "Face engine and biometric data removed." -ForegroundColor Yellow
} else {
    Write-Host "Face engine removed. Biometric data preserved at: $DataDir" -ForegroundColor Green
}
