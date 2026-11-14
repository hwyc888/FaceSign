param(
  [string]$InstallDir = "",
  [switch]$RemoveProgramFiles
)
$ErrorActionPreference = 'Stop'
if ([string]::IsNullOrWhiteSpace($InstallDir)) {
  $InstallDir = Split-Path -Parent $PSScriptRoot
}
$InstallDir = [IO.Path]::GetFullPath($InstallDir)
$driveRoot = [IO.Path]::GetPathRoot($InstallDir)
if ([string]::Equals($InstallDir.TrimEnd('\','/'), $driveRoot.TrimEnd('\','/'), [StringComparison]::OrdinalIgnoreCase)) {
  throw 'Refusing to uninstall from a drive root.'
}
if ($RemoveProgramFiles) {
  foreach ($required in @('FaceSign.exe', 'FaceSignManager.exe')) {
    if (-not (Test-Path (Join-Path $InstallDir $required) -PathType Leaf)) {
      throw "Refusing to remove program files because $required was not found in $InstallDir."
    }
  }
}

Stop-ScheduledTask -TaskName 'FaceSign' -ErrorAction SilentlyContinue
Get-Process -Name 'FaceSign' -ErrorAction SilentlyContinue |
  Stop-Process -Force -ErrorAction SilentlyContinue
$deadline = (Get-Date).AddSeconds(10)
while (Get-Process -Name 'FaceSign' -ErrorAction SilentlyContinue) {
  if ((Get-Date) -ge $deadline) {
    throw 'FaceSign is still running; close it and retry uninstall.'
  }
  Start-Sleep -Milliseconds 250
}
Unregister-ScheduledTask -TaskName 'FaceSign' -Confirm:$false -ErrorAction SilentlyContinue

Get-NetFirewallRule -DisplayName 'FaceSign Web' -ErrorAction SilentlyContinue | Remove-NetFirewallRule -ErrorAction SilentlyContinue
Get-NetFirewallRule -DisplayName 'FaceSign HTTP' -ErrorAction SilentlyContinue | Remove-NetFirewallRule -ErrorAction SilentlyContinue
Get-NetFirewallRule -DisplayName 'FaceSign HTTPS' -ErrorAction SilentlyContinue | Remove-NetFirewallRule -ErrorAction SilentlyContinue

$faceSignMenu = Join-Path ([Environment]::GetFolderPath('CommonPrograms')) 'FaceSign'
$shortcut = Join-Path $faceSignMenu 'FaceSign 管理工具.lnk'
Remove-Item $shortcut -Force -ErrorAction SilentlyContinue
if (Test-Path $faceSignMenu -PathType Container) {
  if (-not (Get-ChildItem -LiteralPath $faceSignMenu -Force | Select-Object -First 1)) {
    Remove-Item $faceSignMenu -Force -ErrorAction SilentlyContinue
  }
}

Write-Host 'FaceSign startup task, management shortcut and firewall rules removed.'
if ($RemoveProgramFiles) {
  $escapedInstallDir = $InstallDir.Replace("'", "''")
  $cleanupPath = Join-Path $env:TEMP ("FaceSign-uninstall-" + [guid]::NewGuid().ToString('N') + '.ps1')
  $cleanupScript = @'
$ErrorActionPreference = 'SilentlyContinue'
Start-Sleep -Seconds 2
$root = '__FACESIGN_INSTALL_DIR__'
$programItems = @(
  'FaceSign.exe',
  'FaceSignManager.exe',
  'onnxruntime.dll',
  'ffmpeg.exe',
  'msvcp140.dll',
  'msvcp140_1.dll',
  'vcruntime140.dll',
  'vcruntime140_1.dll',
  'libgcc_s_seh-1.dll',
  'libwinpthread-1.dll',
  'models',
  'scripts',
  'licenses',
  'README.md',
  'facesign-error.log'
)
foreach ($name in $programItems) {
  Remove-Item -LiteralPath (Join-Path $root $name) -Recurse -Force -ErrorAction SilentlyContinue
}
if (Test-Path -LiteralPath $root -PathType Container) {
  if (-not (Get-ChildItem -LiteralPath $root -Force | Select-Object -First 1)) {
    Remove-Item -LiteralPath $root -Force -ErrorAction SilentlyContinue
  }
}
Remove-Item -LiteralPath $MyInvocation.MyCommand.Path -Force -ErrorAction SilentlyContinue
'@
  $cleanupScript = $cleanupScript.Replace('__FACESIGN_INSTALL_DIR__', $escapedInstallDir)
  Set-Content -LiteralPath $cleanupPath -Value $cleanupScript -Encoding UTF8
  $cleanupArguments = '-NoProfile -NonInteractive -ExecutionPolicy Bypass -File "{0}"' -f $cleanupPath
  Start-Process -FilePath 'powershell.exe' -ArgumentList $cleanupArguments -WindowStyle Hidden | Out-Null
  Write-Host 'FaceSign program files will be removed. The data and tls directories are preserved.'
} else {
  Write-Host "Program files remain in $InstallDir."
}
Write-Host "Data remains in $InstallDir\data."
Write-Host "The persistent TLS identity remains in $InstallDir\tls so clients do not lose trust if FaceSign is reinstalled."
