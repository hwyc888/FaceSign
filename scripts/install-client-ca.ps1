param(
  [Parameter(Mandatory = $true)]
  [string]$Server,
  [int]$HTTPPort = 8080,
  [int]$HTTPSPort = 8443,
  [switch]$Machine,
  [string]$ExpectedThumbprint = ""
)
$ErrorActionPreference = 'Stop'

$serverHost = $Server.Trim()
if ($serverHost -match '^https?://') {
  $serverHost = ([Uri]$serverHost).Host
}
$serverHost = $serverHost.Trim('[',']')
if (-not $serverHost) { throw 'Server cannot be empty.' }

if ($Machine) {
  $principal = New-Object Security.Principal.WindowsPrincipal([Security.Principal.WindowsIdentity]::GetCurrent())
  if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    throw 'Use an Administrator PowerShell window with -Machine, or omit -Machine to trust FaceSign for the current Windows user only.'
  }
  $store = 'Cert:\LocalMachine\Root'
} else {
  $store = 'Cert:\CurrentUser\Root'
}

$download = New-Object System.UriBuilder
$download.Scheme = 'http'
$download.Host = $serverHost
$download.Port = $HTTPPort
$download.Path = 'facesign-root-ca.crt'

$temp = Join-Path $env:TEMP 'FaceSign-Root-CA.crt'
Invoke-WebRequest -Uri $download.Uri.AbsoluteUri -OutFile $temp -UseBasicParsing

$root = New-Object System.Security.Cryptography.X509Certificates.X509Certificate2($temp)
$thumbprint = $root.Thumbprint.ToUpperInvariant()
if ($ExpectedThumbprint) {
  $expected = ($ExpectedThumbprint -replace '[^0-9A-Fa-f]', '').ToUpperInvariant()
  if ($thumbprint -ne $expected) {
    throw "Root CA thumbprint mismatch. Downloaded=$thumbprint Expected=$expected"
  }
}

Import-Certificate -FilePath $temp -CertStoreLocation $store | Out-Null
$installed = Get-ChildItem $store | Where-Object { $_.Thumbprint -eq $thumbprint } | Select-Object -First 1
if (-not $installed) { throw 'FaceSign root CA was not installed into the Windows trust store.' }

$secure = New-Object System.UriBuilder
$secure.Scheme = 'https'
$secure.Host = $serverHost
$secure.Port = $HTTPSPort
$secure.Path = 'api/version'
$versionInfo = Invoke-RestMethod -Uri $secure.Uri.AbsoluteUri -TimeoutSec 5
if (-not $versionInfo.version) { throw 'HTTPS trust test failed.' }

$browser = New-Object System.UriBuilder
$browser.Scheme = 'https'
$browser.Host = $serverHost
$browser.Port = $HTTPSPort
$browser.Path = '/'

Write-Host "FaceSign root CA installed successfully."
Write-Host "Store:       $store"
Write-Host "Thumbprint:  $thumbprint"
Write-Host "HTTPS test:  OK"
Write-Host "Open:        $($browser.Uri.AbsoluteUri)"
Write-Host "If Edge/Chrome was already open, reopen the FaceSign tab before testing the local camera."
