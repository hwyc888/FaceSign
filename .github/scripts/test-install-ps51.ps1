param()
$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path -Parent (Split-Path -Parent $PSScriptRoot)
$scriptsDir = Join-Path $repoRoot 'scripts'
$scripts = @('install.ps1','upgrade.ps1','uninstall.ps1')
foreach ($name in $scripts) {
  $path = Join-Path $scriptsDir $name
  $bytes = [IO.File]::ReadAllBytes($path)
  if ($bytes.Length -lt 3 -or $bytes[0] -ne 239 -or $bytes[1] -ne 187 -or $bytes[2] -ne 191) {
    throw ($name + ' must be UTF-8 with BOM for Windows PowerShell 5.1')
  }
  $tokens = $null
  $errors = $null
  $ast = [System.Management.Automation.Language.Parser]::ParseFile($path, [ref]$tokens, [ref]$errors)
  if ($errors.Count -gt 0) {
    $details = ($errors | ForEach-Object { $_.ToString() }) -join [Environment]::NewLine
    throw ($name + ' has Windows PowerShell 5.1 parse errors:' + [Environment]::NewLine + $details)
  }
}

$installerPath = Join-Path $scriptsDir 'install.ps1'
$tokens = $null
$errors = $null
$installerAst = [System.Management.Automation.Language.Parser]::ParseFile($installerPath, [ref]$tokens, [ref]$errors)
$installText = [IO.File]::ReadAllText($installerPath)
if ($installText -match '(?m)^trap\s*\{') {
  throw 'install.ps1 must not mask the original exception with a global trap'
}

$functionAst = $installerAst.Find({
  param($node)
  return (($node -is [System.Management.Automation.Language.FunctionDefinitionAst]) -and ($node.Name -eq 'Get-TaskArgumentValue'))
}, $true)
if ($null -eq $functionAst) {
  throw 'Get-TaskArgumentValue function was not found in install.ps1'
}
Invoke-Expression $functionAst.Extent.Text

$cases = @(
  @{ Arguments = '--listen 0.0.0.0:8080 --https-listen "0.0.0.0:8443"'; Name = 'listen'; Expected = '0.0.0.0:8080' },
  @{ Arguments = '--listen 0.0.0.0:8080 --https-listen "0.0.0.0:8443"'; Name = 'https-listen'; Expected = '0.0.0.0:8443' },
  @{ Arguments = '--tls-hosts "facesign.local,10.0.0.2" --open-browser=false'; Name = 'tls-hosts'; Expected = 'facesign.local,10.0.0.2' },
  @{ Arguments = '--assets "D:\FaceSign Old\facesign-windows-amd64-full" --data "D:\FaceSign Old\data\facesign.db"'; Name = 'assets'; Expected = 'D:\FaceSign Old\facesign-windows-amd64-full' },
  @{ Arguments = '--listen-extra 127.0.0.1:8080'; Name = 'listen'; Expected = $null },
  @{ Arguments = '--listen'; Name = 'listen'; Expected = $null }
)
foreach ($case in $cases) {
  $actual = Get-TaskArgumentValue -Arguments $case.Arguments -Name $case.Name
  if ($actual -cne $case.Expected) {
    throw ('Get-TaskArgumentValue failed for --' + $case.Name + ': expected [' + $case.Expected + '] but got [' + $actual + ']')
  }
}

$byFunctionAst = $installerAst.Find({
  param($node)
  return (($node -is [System.Management.Automation.Language.FunctionDefinitionAst]) -and ($node.Name -eq 'Ensure-ByFaceModels'))
}, $true)
if ($null -eq $byFunctionAst) {
  throw 'Ensure-ByFaceModels function was not found in install.ps1'
}
Invoke-Expression $byFunctionAst.Extent.Text

$originalByModelDir = $env:FACESIGN_BY_MODEL_DIR
try {
  $env:FACESIGN_BY_MODEL_DIR = ''
  $testRoot = Join-Path $env:RUNNER_TEMP 'facesign-ps51-model-discovery'
  Remove-Item -LiteralPath $testRoot -Recurse -Force -ErrorAction SilentlyContinue
  $source = Join-Path $testRoot 'new-package'
  $InstallDir = $source
  $previousAssetsDir = Join-Path $testRoot 'old-install'
  $oldByDir = Join-Path $previousAssetsDir 'models\by'
  $targetByDir = Join-Path $InstallDir 'models\by'
  New-Item -ItemType Directory -Force -Path $oldByDir | Out-Null
  [IO.File]::WriteAllBytes((Join-Path $oldByDir 'scrfd_10g.onnx'), [byte[]](1,2,3,4,5,6))
  [IO.File]::WriteAllBytes((Join-Path $oldByDir 'arcface_w600k_r50.onnx'), [byte[]](7,8,9,10,11,12))
  $scrfdHash = (Get-FileHash -LiteralPath (Join-Path $oldByDir 'scrfd_10g.onnx') -Algorithm SHA256).Hash.ToLowerInvariant()
  $arcfaceHash = (Get-FileHash -LiteralPath (Join-Path $oldByDir 'arcface_w600k_r50.onnx') -Algorithm SHA256).Hash.ToLowerInvariant()
  $testManifest = [string[]]@(
    [string]::Format('{0}  {1}', $scrfdHash, 'scrfd_10g.onnx'),
    [string]::Format('{0}  {1}', $arcfaceHash, 'arcface_w600k_r50.onnx')
  )
  [IO.File]::WriteAllLines((Join-Path $oldByDir 'SHA256SUMS'), $testManifest, [Text.Encoding]::ASCII)
  Ensure-ByFaceModels
  foreach ($name in @('scrfd_10g.onnx','arcface_w600k_r50.onnx','SHA256SUMS')) {
    if (-not (Test-Path -LiteralPath (Join-Path $targetByDir $name) -PathType Leaf)) {
      throw ('Previous installation model recovery failed; missing ' + $name)
    }
  }
  $recoveredHash = (Get-FileHash -LiteralPath (Join-Path $targetByDir 'scrfd_10g.onnx') -Algorithm SHA256).Hash.ToLowerInvariant()
  if ($recoveredHash -ne $scrfdHash) {
    throw 'SCRFD model bytes changed during recovery from the previous installation'
  }

  $source = Join-Path $testRoot 'package-without-manifest'
  $InstallDir = $source
  $previousAssetsDir = Join-Path $testRoot 'old-install-without-manifest'
  $oldByDir = Join-Path $previousAssetsDir 'models\by'
  $targetByDir = Join-Path $InstallDir 'models\by'
  New-Item -ItemType Directory -Force -Path $oldByDir | Out-Null
  [IO.File]::WriteAllBytes((Join-Path $oldByDir 'scrfd_10g.onnx'), [byte[]](21,22,23,24))
  [IO.File]::WriteAllBytes((Join-Path $oldByDir 'arcface_w600k_r50.onnx'), [byte[]](31,32,33,34))
  Ensure-ByFaceModels
  if (-not (Test-Path -LiteralPath (Join-Path $targetByDir 'SHA256SUMS') -PathType Leaf)) {
    throw 'Installer did not generate a local SHA256SUMS for previously installed models'
  }
  $manifestText = Get-Content -LiteralPath (Join-Path $targetByDir 'SHA256SUMS') -Raw
  if ($manifestText -notmatch 'scrfd_10g\.onnx' -or $manifestText -notmatch 'arcface_w600k_r50\.onnx') {
    throw 'Generated SHA256SUMS does not contain both required face model entries'
  }
} finally {
  $env:FACESIGN_BY_MODEL_DIR = $originalByModelDir
  Remove-Item -LiteralPath $testRoot -Recurse -Force -ErrorAction SilentlyContinue
}
Write-Host 'Windows PowerShell 5.1 installer parser, argument and By-model recovery tests passed.'
