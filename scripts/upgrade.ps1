param(
  [string]$InstallDir = "",
  [bool]$OpenBrowser = $true
)
$ErrorActionPreference = 'Stop'
if ([string]::IsNullOrWhiteSpace($InstallDir)) {
  $InstallDir = Split-Path -Parent $PSScriptRoot
}
$InstallDir = [IO.Path]::GetFullPath($InstallDir)

$identity = [Security.Principal.WindowsIdentity]::GetCurrent()
$principal = New-Object Security.Principal.WindowsPrincipal($identity)
if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
  throw 'FaceSign upgrade requires Administrator privileges. Open PowerShell as Administrator and run scripts\upgrade.ps1 again.'
}

$installedExe = Join-Path $InstallDir 'FaceSign.exe'
$task = Get-ScheduledTask -TaskName 'FaceSign' -ErrorAction SilentlyContinue
if (-not $task -or -not (Test-Path $installedExe -PathType Leaf)) {
  throw 'No existing FaceSign installation was found. Use scripts\install.ps1 for the first installation.'
}

$installer = Join-Path $PSScriptRoot 'install.ps1'
if (-not (Test-Path $installer -PathType Leaf)) {
  throw 'The release package is missing scripts\install.ps1.'
}

Write-Host "Upgrading FaceSign in place: $InstallDir"
Write-Host 'The database, TLS identity, models and service arguments will be preserved. The startup task will be refreshed for the current signed-in Windows user so USB cameras remain available.'
& $installer -InstallDir $InstallDir -OpenBrowser:$OpenBrowser
