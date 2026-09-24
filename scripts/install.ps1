param(
  [string]$InstallDir = "$env:ProgramData\FaceSign",
  [string]$Listen = "0.0.0.0:8080",
  [string]$HTTPSListen = "0.0.0.0:8443",
  [string]$TLSHosts = "",
  [bool]$OpenBrowser = $true
)
$ErrorActionPreference = 'Stop'
$source = Split-Path -Parent $PSScriptRoot

$ModelCommit = 'de5287c66e9e37e9f804686bf63f5a0974f68f72'
$ModelBaseUrl = "https://raw.githubusercontent.com/hwyc888/FaceSign/$ModelCommit/models"
$Models = @(
  @{
    Name = 'face_detection_yunet_2023mar.onnx'
    SHA256 = '8F2383E4DD3CFBB4553EA8718107FC0423210DC964F9F4280604804ED2552FA4'
  },
  @{
    Name = 'face_recognition_sface_2021dec.onnx'
    SHA256 = '0BA9FBFA01B5270C96627C4EF784DA859931E02F04419C829E83484087C34E79'
  },
  @{
    Name = 'anti-spoof-mn3.onnx'
    SHA256 = 'C4C99AF04603B62D7E44F6F4DAEB33E0DAECCC696008C0B1D62F6F5CEBBB3262'
  },
  @{
    Name = 'yolox_nano.onnx'
    SHA256 = 'C789161ED43C8269FCD4E67C67EEEB4E80C622DA2EB296A20BC6007BD18A0B7D'
    URL = 'https://github.com/Megvii-BaseDetection/YOLOX/releases/download/0.1.1rc0/yolox_nano.onnx'
  }
)

function Test-ModelFile {
  param(
    [string]$Path,
    [string]$SHA256
  )
  if (-not (Test-Path $Path -PathType Leaf)) { return $false }
  return (Get-FileHash $Path -Algorithm SHA256).Hash -eq $SHA256
}

function Ensure-FaceModels {
  param([string]$TargetDir)

  New-Item -ItemType Directory -Force -Path $TargetDir | Out-Null
  $sourceModels = Join-Path $source 'models'

  foreach ($model in $Models) {
    $target = Join-Path $TargetDir $model.Name
    if (Test-ModelFile -Path $target -SHA256 $model.SHA256) {
      Write-Host "Model ready: $($model.Name)"
      continue
    }

    $sourceModel = Join-Path $sourceModels $model.Name
    if (Test-ModelFile -Path $sourceModel -SHA256 $model.SHA256) {
      Copy-Item $sourceModel $target -Force
      Write-Host "Model installed from local models folder: $($model.Name)"
      continue
    }

    $url = if ($model.URL) { $model.URL } else { "$ModelBaseUrl/$($model.Name)" }
    $temp = "$target.download"
    Write-Host "Downloading model once: $($model.Name)"
    try {
      Invoke-WebRequest -Uri $url -OutFile $temp -UseBasicParsing
      if (-not (Test-ModelFile -Path $temp -SHA256 $model.SHA256)) {
        throw "Model checksum verification failed: $($model.Name)"
      }
      Move-Item $temp $target -Force
    } finally {
      Remove-Item $temp -Force -ErrorAction SilentlyContinue
    }
  }
}

function Get-TaskArgumentValue {
  param(
    [string]$Arguments,
    [string]$Name
  )
  if ([string]::IsNullOrWhiteSpace($Arguments)) { return $null }
  $pattern = '(?:^|\s)--' + [regex]::Escape($Name) + '\s+(?:"([^"]*)"|(\S+))'
  $match = [regex]::Match($Arguments, $pattern)
  if (-not $match.Success) { return $null }
  if ($match.Groups[1].Success) { return $match.Groups[1].Value }
  return $match.Groups[2].Value
}

# Stop the current instance before replacing files. During an upgrade the
# scheduled task is deliberately kept in place so custom ports/TLS hosts and
# the user's startup-enabled state survive the update.
$oldTask = Get-ScheduledTask -TaskName 'FaceSign' -ErrorAction SilentlyContinue
$isUpgrade = $null -ne $oldTask -and (Test-Path (Join-Path $InstallDir 'FaceSign.exe') -PathType Leaf)
$taskWasDisabled = $false
if ($oldTask) {
  $taskWasDisabled = $oldTask.State -eq 'Disabled'
  $oldAction = @($oldTask.Actions)[0]
  $oldActionArgs = [string]$oldAction.Arguments

  if (-not $PSBoundParameters.ContainsKey('Listen')) {
    $value = Get-TaskArgumentValue -Arguments $oldActionArgs -Name 'listen'
    if ($null -ne $value) { $Listen = $value }
  }
  if (-not $PSBoundParameters.ContainsKey('HTTPSListen')) {
    $value = Get-TaskArgumentValue -Arguments $oldActionArgs -Name 'https-listen'
    if ($null -ne $value) { $HTTPSListen = $value }
  }
  if (-not $PSBoundParameters.ContainsKey('TLSHosts')) {
    $value = Get-TaskArgumentValue -Arguments $oldActionArgs -Name 'tls-hosts'
    if ($null -ne $value) { $TLSHosts = $value }
  }

  Stop-ScheduledTask -TaskName 'FaceSign' -ErrorAction SilentlyContinue
  Start-Sleep -Milliseconds 500
}
Get-Process -Name 'FaceSign' -ErrorAction SilentlyContinue |
  Stop-Process -Force -ErrorAction SilentlyContinue

$installedFFmpeg = Join-Path $InstallDir 'ffmpeg.exe'
Get-CimInstance Win32_Process -Filter "Name='ffmpeg.exe'" -ErrorAction SilentlyContinue |
  Where-Object { $_.ExecutablePath -and ([string]::Equals($_.ExecutablePath, $installedFFmpeg, [System.StringComparison]::OrdinalIgnoreCase)) } |
  ForEach-Object { Stop-Process -Id $_.ProcessId -Force -ErrorAction SilentlyContinue }
Start-Sleep -Milliseconds 700

$remaining = Get-Process -Name 'FaceSign' -ErrorAction SilentlyContinue
if ($remaining) {
  throw 'Old FaceSign.exe is still running. Close it and run this installer again.'
}

New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
Copy-Item (Join-Path $source 'FaceSign.exe') $InstallDir -Force
Copy-Item (Join-Path $source 'FaceSignManager.exe') $InstallDir -Force
Copy-Item (Join-Path $source 'onnxruntime.dll') $InstallDir -Force
$sourceFFmpeg = Join-Path $source 'ffmpeg.exe'
if (Test-Path $sourceFFmpeg -PathType Leaf) {
  Copy-Item $sourceFFmpeg $InstallDir -Force
} elseif (Test-Path $installedFFmpeg -PathType Leaf) {
  Write-Host 'Upgrade package has no ffmpeg.exe; keeping the installed FFmpeg runtime.'
} else {
  throw 'This package has no ffmpeg.exe and the existing installation has no reusable FFmpeg runtime.'
}
$sourceLicenses = Join-Path $source 'licenses'
if (Test-Path $sourceLicenses -PathType Container) {
  New-Item -ItemType Directory -Force -Path (Join-Path $InstallDir 'licenses') | Out-Null
  Copy-Item (Join-Path $sourceLicenses '*') (Join-Path $InstallDir 'licenses') -Recurse -Force
}
foreach ($dll in @('msvcp140.dll','msvcp140_1.dll','vcruntime140.dll','vcruntime140_1.dll','libgcc_s_seh-1.dll','libwinpthread-1.dll')) {
  $p = Join-Path $source $dll
  if (Test-Path $p) { Copy-Item $p $InstallDir -Force }
}

# The installer supports both release variants. Existing valid installed models
# are reused first. The full package can supply local models; the lightweight
# package downloads only models that are actually missing or invalid.
Ensure-FaceModels -TargetDir (Join-Path $InstallDir 'models')

New-Item -ItemType Directory -Force -Path (Join-Path $InstallDir 'data') | Out-Null
Remove-Item (Join-Path $InstallDir 'facesign-error.log') -Force -ErrorAction SilentlyContinue

$exe = Join-Path $InstallDir 'FaceSign.exe'
$db = Join-Path $InstallDir 'data\facesign.db'
$tlsDir = Join-Path $InstallDir 'tls'
$faceArgs = '--listen {0} --https-listen {1} --http-redirect=true --tls-dir "{2}" --open-browser=false --assets "{3}" --data "{4}"' -f $Listen, $HTTPSListen, $tlsDir, $InstallDir, $db
if ($TLSHosts) {
  $faceArgs += ' --tls-hosts "{0}"' -f $TLSHosts
}
$existingTask = Get-ScheduledTask -TaskName 'FaceSign' -ErrorAction SilentlyContinue
$configurationExplicit = $PSBoundParameters.ContainsKey('Listen') -or
  $PSBoundParameters.ContainsKey('HTTPSListen') -or
  $PSBoundParameters.ContainsKey('TLSHosts')

if ($existingTask) {
  $existingAction = @($existingTask.Actions)[0]
  $existingExecute = [Environment]::ExpandEnvironmentVariables(([string]$existingAction.Execute).Trim('"'))
  $executeChanged = -not [string]::Equals($existingExecute, $exe, [System.StringComparison]::OrdinalIgnoreCase)
  if ($configurationExplicit -or $executeChanged) {
    $action = New-ScheduledTaskAction -Execute $exe -Argument $faceArgs
    Set-ScheduledTask -TaskName 'FaceSign' -Action $action | Out-Null
  }
} else {
  $action = New-ScheduledTaskAction -Execute $exe -Argument $faceArgs
  $trigger = New-ScheduledTaskTrigger -AtStartup
  $principal = New-ScheduledTaskPrincipal -UserId 'SYSTEM' -LogonType ServiceAccount -RunLevel Highest
  $settings = New-ScheduledTaskSettingsSet -RestartCount 3 -RestartInterval (New-TimeSpan -Minutes 1) -ExecutionTimeLimit ([TimeSpan]::Zero)
  Register-ScheduledTask -TaskName 'FaceSign' -Action $action -Trigger $trigger -Principal $principal -Settings $settings -Force | Out-Null
}

$port = ($Listen -split ':')[-1]
$httpsPort = ($HTTPSListen -split ':')[-1]
Get-NetFirewallRule -DisplayName 'FaceSign Web' -ErrorAction SilentlyContinue | Remove-NetFirewallRule -ErrorAction SilentlyContinue
Get-NetFirewallRule -DisplayName 'FaceSign HTTP' -ErrorAction SilentlyContinue | Remove-NetFirewallRule -ErrorAction SilentlyContinue
Get-NetFirewallRule -DisplayName 'FaceSign HTTPS' -ErrorAction SilentlyContinue | Remove-NetFirewallRule -ErrorAction SilentlyContinue
if ($Listen.StartsWith('0.0.0.0:') -or $Listen.StartsWith(':')) {
  New-NetFirewallRule -DisplayName 'FaceSign HTTP' -Direction Inbound -Action Allow -Protocol TCP -LocalPort $port -Profile Domain,Private | Out-Null
}
if ($HTTPSListen.StartsWith('0.0.0.0:') -or $HTTPSListen.StartsWith(':')) {
  New-NetFirewallRule -DisplayName 'FaceSign HTTPS' -Direction Inbound -Action Allow -Protocol TCP -LocalPort $httpsPort -Profile Domain,Private | Out-Null
}

if ($taskWasDisabled) {
  Enable-ScheduledTask -TaskName 'FaceSign' | Out-Null
}
try {
  Start-ScheduledTask -TaskName 'FaceSign'
} finally {
  if ($taskWasDisabled) {
    Disable-ScheduledTask -TaskName 'FaceSign' | Out-Null
  }
}

$rootCAPath = Join-Path $tlsDir 'facesign-root-ca.crt'
$rootReady = $false
for ($i = 0; $i -lt 30; $i++) {
  try {
    $response = Invoke-WebRequest -Uri "http://127.0.0.1:$port/facesign-root-ca.crt" -UseBasicParsing -TimeoutSec 2
    if ($response.StatusCode -eq 200 -and (Test-Path $rootCAPath -PathType Leaf)) {
      $rootReady = $true
      break
    }
  } catch {
    Start-Sleep -Milliseconds 500
  }
}
if (-not $rootReady) {
  throw 'FaceSign root CA was not generated or could not be downloaded from the HTTP bootstrap endpoint.'
}

& icacls.exe $tlsDir /inheritance:r /grant:r '*S-1-5-18:(OI)(CI)F' '*S-1-5-32-544:(OI)(CI)F' | Out-Null
if ($LASTEXITCODE -ne 0) { throw 'Failed to protect the FaceSign TLS directory ACL.' }

$rootCertificate = Import-Certificate -FilePath $rootCAPath -CertStoreLocation 'Cert:\LocalMachine\Root'
$rootThumbprint = $rootCertificate.Thumbprint.ToUpperInvariant()

$versionInfo = $null
for ($i = 0; $i -lt 20; $i++) {
  try {
    $versionInfo = Invoke-RestMethod -Uri "https://127.0.0.1:$httpsPort/api/version" -TimeoutSec 2
    if ($versionInfo.version) { break }
  } catch {
    Start-Sleep -Milliseconds 500
  }
}
if (-not $versionInfo.version) {
  $errorLog = Join-Path $InstallDir 'facesign-error.log'
  if (Test-Path $errorLog) {
    throw "FaceSign failed to start: $(Get-Content $errorLog -Raw)"
  }
  throw 'FaceSign failed to start. Check C:\ProgramData\FaceSign\data\facesign-startup.log and Windows Task Scheduler.'
}

$url = "https://127.0.0.1:$httpsPort/?v=$($versionInfo.version)"
$operation = if ($isUpgrade) { 'upgraded in place' } else { 'installed' }
Write-Host "FaceSign $operation and started."
Write-Host "Version:       $($versionInfo.version)"
Write-Host "Models:        face models pinned at $ModelCommit; YOLOX-Nano pinned by SHA-256"
Write-Host "Root CA:       $rootCAPath"
Write-Host "CA Thumbprint: $rootThumbprint"
Write-Host "Local HTTPS:   $url"
$lanIPs = @(Get-NetIPAddress -AddressFamily IPv4 -AddressState Preferred -ErrorAction SilentlyContinue |
  Where-Object { $_.IPAddress -ne '127.0.0.1' -and -not $_.IPAddress.StartsWith('169.254.') } |
  Select-Object -ExpandProperty IPAddress -Unique)
foreach ($ip in $lanIPs) {
  $lanURL = "https://{0}:{1}/" -f $ip, $httpsPort
  Write-Host "LAN HTTPS:     $lanURL"
  Write-Host ("Client CA:     .\scripts\install-client-ca.ps1 -Server {0} -HTTPPort {1} -HTTPSPort {2} -ExpectedThumbprint {3}" -f $ip, $port, $httpsPort, $rootThumbprint)
}
if ($TLSHosts) {
  Write-Host "Extra TLS SAN: $TLSHosts"
}

$commonPrograms = [Environment]::GetFolderPath('CommonPrograms')
$faceSignMenu = Join-Path $commonPrograms 'FaceSign'
New-Item -ItemType Directory -Force -Path $faceSignMenu | Out-Null
$shortcutPath = Join-Path $faceSignMenu 'FaceSign 管理工具.lnk'
$managerPath = Join-Path $InstallDir 'FaceSignManager.exe'
$wsh = New-Object -ComObject WScript.Shell
$shortcut = $wsh.CreateShortcut($shortcutPath)
$shortcut.TargetPath = $managerPath
$shortcut.Arguments = '--install-dir "{0}"' -f $InstallDir
$shortcut.WorkingDirectory = $InstallDir
$shortcut.Description = 'FaceSign 服务管理工具'
$shortcut.Save()

Write-Host "Manager:       $managerPath"
Write-Host "Start Menu:    FaceSign\FaceSign 管理工具"
if ($OpenBrowser) {
  Start-Process $url
}
