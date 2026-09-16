param(
    [switch]$RemoveData
)

$ErrorActionPreference = "Stop"
$service = Get-Service -Name "FaceSign" -ErrorAction SilentlyContinue
if ($service) {
    if ($service.Status -ne "Stopped") { Stop-Service -Name "FaceSign" -Force }
    sc.exe delete FaceSign | Out-Null
}
Remove-Item -Recurse -Force "$env:ProgramFiles\FaceSign" -ErrorAction SilentlyContinue
if ($RemoveData) {
    Remove-Item -Recurse -Force "$env:ProgramData\FaceSign" -ErrorAction SilentlyContinue
}
Write-Host "FaceSign service removed."
