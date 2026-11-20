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
  @{ Arguments = '--listen-extra 127.0.0.1:8080'; Name = 'listen'; Expected = $null },
  @{ Arguments = '--listen'; Name = 'listen'; Expected = $null }
)
foreach ($case in $cases) {
  $actual = Get-TaskArgumentValue -Arguments $case.Arguments -Name $case.Name
  if ($actual -cne $case.Expected) {
    throw ('Get-TaskArgumentValue failed for --' + $case.Name + ': expected [' + $case.Expected + '] but got [' + $actual + ']')
  }
}
Write-Host 'Windows PowerShell 5.1 installer parser and argument tests passed.'
