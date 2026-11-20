param(
  [string]$InstallDir = "",
  [string]$Listen = "0.0.0.0:8080",
  [string]$HTTPSListen = "0.0.0.0:8443",
  [string]$TLSHosts = "",
  [string]$LogPath = "",
  [bool]$OpenBrowser = $true
)
$ErrorActionPreference = 'Stop'
if (($PSVersionTable.PSVersion.Major -lt 5) -or
    (($PSVersionTable.PSVersion.Major -eq 5) -and ($PSVersionTable.PSVersion.Minor -lt 1))) {
  throw 'FaceSign installer requires Windows PowerShell 5.1 or later.'
}
$source = Split-Path -Parent $PSScriptRoot
if ([string]::IsNullOrWhiteSpace($InstallDir)) {
  $InstallDir = $source
}
$source = [IO.Path]::GetFullPath($source)
$InstallDir = [IO.Path]::GetFullPath($InstallDir)
$installInPlace = ($source -ieq $InstallDir)
if ([string]::IsNullOrWhiteSpace($LogPath)) {
  $LogPath = Join-Path $InstallDir 'data\facesign-install.log'
}
$LogPath = [IO.Path]::GetFullPath($LogPath)
New-Item -ItemType Directory -Force -Path (Split-Path -Parent $LogPath) | Out-Null
try { Start-Transcript -Path $LogPath -Append -Force | Out-Null } catch { }
# Do not install a global trap here. A bare "throw" inside a PowerShell trap
# can replace the original exception with ScriptHalted and hide the real cause.
# Let the original terminating error reach the manager's diagnostic output.

function Get-FaceSignInteractiveUser {
  $identity = [Security.Principal.WindowsIdentity]::GetCurrent()
  if (($null -ne $identity) -and ($null -ne $identity.User) -and ($identity.Name -notmatch '(?i)\\SYSTEM$')) {
    return $identity.Name
  }

  $userName = [string](Get-CimInstance Win32_ComputerSystem -ErrorAction SilentlyContinue).UserName
  if ([string]::IsNullOrWhiteSpace($userName) -or $userName -match '(?i)\\SYSTEM$') {
    throw 'FaceSign USB camera startup registration requires a signed-in Windows desktop user. Sign in to Windows and run FaceSignManager.exe -> 安装/注册本目录 again.'
  }
  return $userName
}

$interactiveUser = Get-FaceSignInteractiveUser
$currentIdentity = [Security.Principal.WindowsIdentity]::GetCurrent()
$interactiveUserSid = $null
if (($null -ne $currentIdentity) -and ($null -ne $currentIdentity.User) -and ($currentIdentity.Name -ieq $interactiveUser)) {
  $interactiveUserSid = $currentIdentity.User.Value
}
if ([string]::IsNullOrWhiteSpace($interactiveUserSid)) {
  try {
    $account = New-Object -TypeName System.Security.Principal.NTAccount -ArgumentList $interactiveUser
    $sidObject = $account.Translate([System.Security.Principal.SecurityIdentifier])
    $interactiveUserSid = $sidObject.Value
  } catch {
    throw ('无法解析当前 Windows 登录用户的 SID：{0}。请使用该用户登录后重新运行安装。' -f $interactiveUser)
  }
}

$ModelCommit = 'de5287c66e9e37e9f804686bf63f5a0974f68f72'
$ModelBaseUrl = 'https://raw.githubusercontent.com/hwyc888/FaceSign/' + $ModelCommit + '/models'
$activeBackend = ([string]$env:FACESIGN_LIVENESS_BACKEND).Trim().ToLowerInvariant()
if (@('mn3','minifas') -notcontains $activeBackend) { $activeBackend = 'silent' }
$Models = @(
  @{
    Name = 'yolox_nano.onnx'
    SHA256 = 'C789161ED43C8269FCD4E67C67EEEB4E80C622DA2EB296A20BC6007BD18A0B7D'
    URL = 'https://github.com/Megvii-BaseDetection/YOLOX/releases/download/0.1.1rc0/yolox_nano.onnx'
  }
)
switch ($activeBackend) {
  'mn3' {
    $Models += @{
      Name = 'anti-spoof-mn3.onnx'
      SHA256 = 'C4C99AF04603B62D7E44F6F4DAEB33E0DAECCC696008C0B1D62F6F5CEBBB3262'
    }
  }
  'minifas' {
    $Models += @{
      Name = 'minifasnet-v2se.onnx'
      SHA256 = 'AF2381B88F38769222ED93379E12444E2A50814575DE1C46170DE570C55A42B6'
      URL = 'https://github.com/facenox/face-antispoof-onnx/releases/download/v1.0.0/best_model.onnx'
    }
  }
  default {
    $Models += @(
      @{
        Name = 'MiniFASNetV2.onnx'
        SHA256 = 'B32929ADC2D9C34B9486F8C4C7BC97C1B69BC0EA9BEFEFC380E4FAAE4E463907'
        URL = 'https://github.com/yakhyo/face-anti-spoofing/releases/download/weights/MiniFASNetV2.onnx'
      },
      @{
        Name = 'MiniFASNetV1SE.onnx'
        SHA256 = 'EBAB7F90C7833FBCCD46D3A555410E78D969DB5438E169B6524BE444862B3676'
        URL = 'https://github.com/yakhyo/face-anti-spoofing/releases/download/weights/MiniFASNetV1SE.onnx'
      }
    )
  }
}

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
      Write-Host ('Model ready: ' + $model.Name)
      continue
    }

    $sourceModel = Join-Path $sourceModels $model.Name
    if (Test-ModelFile -Path $sourceModel -SHA256 $model.SHA256) {
      Copy-Item $sourceModel $target -Force
      Write-Host ('Model installed from local models folder: ' + $model.Name)
      continue
    }

    if (-not [string]::IsNullOrWhiteSpace([string]$model.URL)) {
      $url = $model.URL
    } else {
      $url = $ModelBaseUrl + '/' + $model.Name
    }
    $temp = $target + '.download'
    Write-Host ('Downloading model once: ' + $model.Name)
    try {
      Invoke-WebRequest -Uri $url -OutFile $temp -UseBasicParsing
      if (-not (Test-ModelFile -Path $temp -SHA256 $model.SHA256)) {
        throw ('Model checksum verification failed: ' + $model.Name)
      }
      Move-Item $temp $target -Force
    } finally {
      Remove-Item $temp -Force -ErrorAction SilentlyContinue
    }
  }
}

function Ensure-ByFaceModels {
  $sourceDir = Join-Path $source 'models\by'
  $customModelDir = ([string]$env:FACESIGN_BY_MODEL_DIR).Trim()
  if ([string]::IsNullOrWhiteSpace($customModelDir)) {
    $targetDir = Join-Path $InstallDir 'models\by'
  } else {
    $targetDir = [IO.Path]::GetFullPath($customModelDir)
  }
  New-Item -ItemType Directory -Force -Path $targetDir | Out-Null

  $requiredNames = @('scrfd_10g.onnx','arcface_w600k_r50.onnx')
  $candidateDirs = @()
  if (-not [string]::IsNullOrWhiteSpace($customModelDir)) {
    $candidateDirs += $targetDir
  }
  if (-not [string]::IsNullOrWhiteSpace($previousAssetsDir)) {
    $candidateDirs += (Join-Path $previousAssetsDir 'models\by')
  }
  $candidateDirs += $targetDir
  $candidateDirs += $sourceDir

  $programDataPath = [string]$env:ProgramData
  if (-not [string]::IsNullOrWhiteSpace($programDataPath)) {
    $candidateDirs += (Join-Path $programDataPath 'FaceSign\models\by')
  }
  $localAppDataPath = [Environment]::GetFolderPath('LocalApplicationData')
  if (-not [string]::IsNullOrWhiteSpace($localAppDataPath)) {
    $candidateDirs += (Join-Path $localAppDataPath 'FaceSign\models\by')
  }

  # A portable package is often extracted to a new folder next to the previous
  # installation. Check the same-named sibling folder without scanning drives.
  $sourceParent = Split-Path -Parent $source
  $sourceLeaf = Split-Path -Leaf $source
  $grandParent = Split-Path -Parent $sourceParent
  if (-not [string]::IsNullOrWhiteSpace($grandParent)) {
    $candidateDirs += (Join-Path (Join-Path $grandParent $sourceLeaf) 'models\by')
  }

  $seenDirs = @{}
  $uniqueCandidates = @()
  foreach ($candidate in $candidateDirs) {
    if ([string]::IsNullOrWhiteSpace($candidate)) { continue }
    $candidate = [IO.Path]::GetFullPath($candidate)
    $key = $candidate.ToLowerInvariant()
    if (-not $seenDirs.ContainsKey($key)) {
      $seenDirs[$key] = $true
      $uniqueCandidates += $candidate
    }
  }

  $selectedDir = $null
  $selectedManifest = $null
  $candidateProblems = @()
  foreach ($candidate in $uniqueCandidates) {
    $hasAllModels = $true
    foreach ($name in $requiredNames) {
      if (-not (Test-Path -LiteralPath (Join-Path $candidate $name) -PathType Leaf)) {
        $hasAllModels = $false
        break
      }
    }
    if (-not $hasAllModels) { continue }

    $candidateManifest = Join-Path $candidate 'SHA256SUMS'
    $candidateHashes = @{}
    if (Test-Path -LiteralPath $candidateManifest -PathType Leaf) {
      foreach ($line in Get-Content -LiteralPath $candidateManifest) {
        if ($line -match '^([0-9a-fA-F]{64})\s+[*]?(.+)$') {
          $candidateHashes[$matches[2].Trim()] = $matches[1].ToLowerInvariant()
        }
      }
      $manifestValid = $true
      foreach ($name in $requiredNames) {
        if (-not $candidateHashes.ContainsKey($name)) {
          $manifestValid = $false
          break
        }
        $actual = (Get-FileHash -LiteralPath (Join-Path $candidate $name) -Algorithm SHA256).Hash.ToLowerInvariant()
        if ($actual -ne $candidateHashes[$name]) {
          $manifestValid = $false
          break
        }
      }
      if (-not $manifestValid) {
        $candidateProblems += ($candidate + '（SHA256SUMS 缺项或校验不匹配）')
        continue
      }
    } else {
      # Older working installations may predate the local manifest. Keep the
      # original weights and create a checksum manifest after copying them.
      $candidateHashes = @{}
      foreach ($name in $requiredNames) {
        $candidateHashes[$name] = (Get-FileHash -LiteralPath (Join-Path $candidate $name) -Algorithm SHA256).Hash.ToLowerInvariant()
      }
    }

    $selectedDir = $candidate
    $selectedManifest = $candidateManifest
    $selectedHashes = $candidateHashes
    break
  }

  if ([string]::IsNullOrWhiteSpace($selectedDir)) {
    $checked = $uniqueCandidates -join '; '
    $details = ''
    if ($candidateProblems.Count -gt 0) {
      $details = '；发现但未通过校验：' + ($candidateProblems -join '; ')
    }
    throw ('未找到可用的 SCRFD / ArcFace 身份模型。已检查：' + $checked + $details + '。请将原安装目录的 models\by 文件夹保留/复制到当前目录，或设置 FACESIGN_BY_MODEL_DIR 指向包含 scrfd_10g.onnx 和 arcface_w600k_r50.onnx 的目录。安装尚未停止或覆盖现有 FaceSign 服务。')
  }

  if ($selectedDir -ine [IO.Path]::GetFullPath($targetDir)) {
    foreach ($name in $requiredNames) {
      Copy-Item -LiteralPath (Join-Path $selectedDir $name) -Destination (Join-Path $targetDir $name) -Force
    }
    Write-Host ('Reused SCRFD / ArcFace weights from existing model directory: ' + $selectedDir)
  } else {
    Write-Host ('Using SCRFD / ArcFace weights in: ' + $selectedDir)
  }

  $manifestPath = Join-Path $targetDir 'SHA256SUMS'
  if ((Test-Path -LiteralPath $selectedManifest -PathType Leaf) -and ($selectedDir -ine [IO.Path]::GetFullPath($targetDir))) {
    Copy-Item -LiteralPath $selectedManifest -Destination $manifestPath -Force
  }
  if (-not (Test-Path -LiteralPath $manifestPath -PathType Leaf)) {
    $manifestLines = @()
    foreach ($name in $requiredNames) {
      $hash = (Get-FileHash -LiteralPath (Join-Path $targetDir $name) -Algorithm SHA256).Hash.ToLowerInvariant()
      $manifestLines += ($hash + '  ' + $name)
    }
    Set-Content -LiteralPath $manifestPath -Value $manifestLines -Encoding ASCII
    Write-Warning '原安装目录没有 SHA256SUMS；已根据保留的现有人脸模型生成本地完整性清单。'
  }

  foreach ($name in $requiredNames) {
    $modelPath = Join-Path $targetDir $name
    if (-not (Test-Path -LiteralPath $modelPath -PathType Leaf)) {
      throw ('缺少新模型文件：' + $modelPath)
    }
    if (-not $selectedHashes.ContainsKey($name)) {
      throw ('SHA256SUMS 缺少 ' + $name + ' 的校验值')
    }
    $actual = (Get-FileHash -LiteralPath $modelPath -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($actual -ne $selectedHashes[$name]) {
      throw ('新模型 SHA256 校验失败：' + $name)
    }
    Write-Host ('By face model ready: ' + $name)
  }
}

function Remove-ObsoleteLivenessModels {
  $modelDir = Join-Path $InstallDir 'models'
  $keep = @('yolox_nano.onnx')
  switch ($activeBackend) {
    'mn3' { $keep += 'anti-spoof-mn3.onnx' }
    'minifas' { $keep += 'minifasnet-v2se.onnx' }
    default { $keep += @('MiniFASNetV2.onnx','MiniFASNetV1SE.onnx') }
  }

  foreach ($name in @('anti-spoof-mn3.onnx','minifasnet-v2se.onnx','MiniFASNetV2.onnx','MiniFASNetV1SE.onnx')) {
    if ($keep -notcontains $name) {
      Remove-Item (Join-Path $modelDir $name) -Force -ErrorAction SilentlyContinue
    }
  }
}

function Get-TaskArgumentValue {
  param(
    [string]$Arguments,
    [string]$Name
  )
  if ([string]::IsNullOrWhiteSpace($Arguments) -or [string]::IsNullOrWhiteSpace($Name)) {
    return $null
  }

  # Parse the generated task argument string without regex or newer parser
  # syntax. This keeps the installer compatible with Windows PowerShell 5.1.
  $needle = '--' + $Name
  $lowerArguments = $Arguments.ToLowerInvariant()
  $lowerNeedle = $needle.ToLowerInvariant()
  $searchFrom = 0
  $foundIndex = -1
  while ($searchFrom -lt $Arguments.Length) {
    $index = $lowerArguments.IndexOf($lowerNeedle, $searchFrom)
    if ($index -lt 0) { break }

    $beforeOK = ($index -eq 0)
    if ($index -gt 0) {
      $beforeOK = [char]::IsWhiteSpace($Arguments[$index - 1])
    }
    $valueStart = $index + $needle.Length
    $afterOK = ($valueStart -lt $Arguments.Length)
    if ($afterOK) {
      $afterOK = [char]::IsWhiteSpace($Arguments[$valueStart])
    }
    if ($beforeOK -and $afterOK) {
      $foundIndex = $valueStart
      break
    }
    $searchFrom = $index + 1
  }
  if ($foundIndex -lt 0) { return $null }

  $cursor = $foundIndex
  while (($cursor -lt $Arguments.Length) -and [char]::IsWhiteSpace($Arguments[$cursor])) {
    $cursor++
  }
  if ($cursor -ge $Arguments.Length) { return $null }

  if ($Arguments[$cursor] -eq '"') {
    $cursor++
    $end = $Arguments.IndexOf('"', $cursor)
    if ($end -lt 0) { return $null }
    return $Arguments.Substring($cursor, $end - $cursor)
  }

  $end = $cursor
  while (($end -lt $Arguments.Length) -and (-not [char]::IsWhiteSpace($Arguments[$end]))) {
    $end++
  }
  if ($end -eq $cursor) { return $null }
  return $Arguments.Substring($cursor, $end - $cursor)
}

# Read the existing scheduled task before model preflight. Its --assets path
# is the most reliable source for recovering identity weights when the user
# extracts a new package into a different folder.
$oldTask = Get-ScheduledTask -TaskName 'FaceSign' -ErrorAction SilentlyContinue
$oldAction = $null
$oldActionArgs = ''
$previousAssetsDir = $null
if ($oldTask) {
  $oldAction = @($oldTask.Actions)[0]
  if ($oldAction) {
    $oldActionArgs = [string]$oldAction.Arguments
    $previousAssetsDir = Get-TaskArgumentValue -Arguments $oldActionArgs -Name 'assets'
    if ([string]::IsNullOrWhiteSpace($previousAssetsDir) -and -not [string]::IsNullOrWhiteSpace([string]$oldAction.Execute)) {
      $previousAssetsDir = Split-Path -Parent ([string]$oldAction.Execute)
    }
  }
}

# Validate all models before stopping the old service. Reuse the previous
# scheduled task's asset directory, ProgramData/cache locations and the
# same-named sibling installation before asking the user to restore weights.
Ensure-FaceModels -TargetDir (Join-Path $InstallDir 'models')
Ensure-ByFaceModels

# Stop the current instance before replacing program files. During an upgrade
# the scheduled task is deliberately kept in place so custom ports/TLS hosts
# and the user's startup-enabled state survive the update.
$isUpgrade = $null -ne $oldTask -and (Test-Path (Join-Path $InstallDir 'FaceSign.exe') -PathType Leaf)
$taskWasDisabled = $false
if ($oldTask) {
  $taskWasDisabled = $oldTask.State -eq 'Disabled'

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
  Where-Object { $_.ExecutablePath -and ($_.ExecutablePath -ieq $installedFFmpeg) } |
  ForEach-Object { Stop-Process -Id $_.ProcessId -Force -ErrorAction SilentlyContinue }
Start-Sleep -Milliseconds 700

$remaining = Get-Process -Name 'FaceSign' -ErrorAction SilentlyContinue
if ($remaining) {
  throw 'Old FaceSign.exe is still running. Close it and run this installer again.'
}

New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
if ($installInPlace) {
  foreach ($required in @('FaceSign.exe','FaceSignManager.exe','onnxruntime.dll')) {
    if (-not (Test-Path (Join-Path $InstallDir $required) -PathType Leaf)) {
      throw ('The current FaceSign directory is incomplete; missing ' + $required)
    }
  }
  if (-not (Test-Path (Join-Path $InstallDir 'ffmpeg.exe') -PathType Leaf)) {
    throw 'This package has no ffmpeg.exe. Use the full portable package when installing the current directory as a service.'
  }
  Write-Host ('Installing FaceSign in place; program files stay in: ' + $InstallDir)
} else {
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
}

# Model files were preflighted before stopping the previous service.
New-Item -ItemType Directory -Force -Path (Join-Path $InstallDir 'data') | Out-Null
Remove-Item (Join-Path $InstallDir 'facesign-error.log') -Force -ErrorAction SilentlyContinue

$exe = Join-Path $InstallDir 'FaceSign.exe'
$db = Join-Path $InstallDir 'data\facesign.db'
$tlsDir = Join-Path $InstallDir 'tls'
$faceArgs = '--listen {0} --https-listen {1} --http-redirect=true --tls-dir "{2}" --open-browser=false --background=true --assets "{3}" --data "{4}"' -f $Listen, $HTTPSListen, $tlsDir, $InstallDir, $db
if ($TLSHosts) {
  $faceArgs += ' --tls-hosts "{0}"' -f $TLSHosts
}
$existingTask = Get-ScheduledTask -TaskName 'FaceSign' -ErrorAction SilentlyContinue
$settings = New-ScheduledTaskSettingsSet -RestartCount 3 -RestartInterval (New-TimeSpan -Minutes 1) -ExecutionTimeLimit ([TimeSpan]::Zero) -MultipleInstances IgnoreNew
$action = New-ScheduledTaskAction -Execute $exe -Argument $faceArgs

# Windows USB/DirectShow cameras belong to the signed-in desktop session.
# SYSTEM Session 0 cannot reliably enumerate them, so install/upgrade always
# migrates the startup task to the current interactive Windows user.
$trigger = New-ScheduledTaskTrigger -AtLogOn -User $interactiveUser
$principal = New-ScheduledTaskPrincipal -UserId $interactiveUser -LogonType Interactive -RunLevel Highest
if ($existingTask) {
  Unregister-ScheduledTask -TaskName 'FaceSign' -Confirm:$false
}
Register-ScheduledTask -TaskName 'FaceSign' -Action $action -Trigger $trigger -Principal $principal -Settings $settings -Force | Out-Null

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

$userAcl = '*' + $interactiveUserSid + ':(OI)(CI)F'
& icacls.exe $tlsDir /inheritance:r /grant:r '*S-1-5-18:(OI)(CI)F' '*S-1-5-32-544:(OI)(CI)F' $userAcl | Out-Null
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
    $startupError = Get-Content $errorLog -Raw
    throw ('FaceSign failed to start: ' + $startupError)
  }
  throw "FaceSign failed to start. Check $InstallDir\data\facesign-startup.log and Windows Task Scheduler."
}

$url = 'https://127.0.0.1:' + $httpsPort + '/?v=' + $versionInfo.version
Remove-ObsoleteLivenessModels
if ($isUpgrade) {
  $operation = 'upgraded in place'
} elseif ($installInPlace) {
  $operation = 'registered in the current directory'
} else {
  $operation = 'installed'
}
Write-Host ('FaceSign ' + $operation + ' and started for interactive user: ' + $interactiveUser)
Write-Host ('Version:       ' + $versionInfo.version)
Write-Host "Models:        $activeBackend liveness + YOLOX-Nano; SCRFD/ArcFace verified from models/by/SHA256SUMS"
Write-Host "Root CA:       $rootCAPath"
Write-Host "CA Thumbprint: $rootThumbprint"
Write-Host "Local HTTPS:   $url"
$lanIPs = @(Get-NetIPAddress -AddressFamily IPv4 -AddressState Preferred -ErrorAction SilentlyContinue |
  Where-Object { $_.IPAddress -ne '127.0.0.1' -and -not $_.IPAddress.StartsWith('169.254.') } |
  Select-Object -ExpandProperty IPAddress -Unique)
foreach ($ip in $lanIPs) {
  $lanURL = "https://{0}:{1}/" -f $ip, $httpsPort
  Write-Host ('LAN HTTPS:     ' + $lanURL)
  Write-Host ("Client CA:     .\scripts\install-client-ca.ps1 -Server {0} -HTTPPort {1} -HTTPSPort {2} -ExpectedThumbprint {3}" -f $ip, $port, $httpsPort, $rootThumbprint)
}
if ($TLSHosts) {
  Write-Host ('Extra TLS SAN: ' + $TLSHosts)
}

$commonPrograms = [Environment]::GetFolderPath('CommonPrograms')
$faceSignMenu = Join-Path $commonPrograms 'FaceSign'
New-Item -ItemType Directory -Force -Path $faceSignMenu | Out-Null
$shortcutPath = Join-Path $faceSignMenu 'FaceSign 管理工具.lnk'
$managerPath = Join-Path $InstallDir 'FaceSignManager.exe'
$wsh = New-Object -ComObject WScript.Shell
$shortcut = $wsh.CreateShortcut($shortcutPath)
$shortcut.TargetPath = $managerPath
$shortcut.Arguments = ('--install-dir "{0}"' -f $InstallDir)
$shortcut.WorkingDirectory = $InstallDir
$shortcut.Description = 'FaceSign 服务管理工具'
$shortcut.Save()

Write-Host ('Manager:       ' + $managerPath)
Write-Host "Start Menu:    FaceSign\FaceSign 管理工具"
if ($OpenBrowser) {
  Start-Process $url
}
try { Stop-Transcript | Out-Null } catch { }
