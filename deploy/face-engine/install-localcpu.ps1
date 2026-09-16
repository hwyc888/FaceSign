param(
    [string]$InstallDir = "$env:ProgramData\FaceSign\face-engine",
    [int]$Port = 18081
)

$ErrorActionPreference = "Stop"
$PythonVersion = "3.11.9"
$PythonZipUrl = "https://www.python.org/ftp/python/$PythonVersion/python-$PythonVersion-embed-amd64.zip"
$GetPipUrl = "https://bootstrap.pypa.io/get-pip.py"
$YuNetUrl = "https://github.com/opencv/opencv_zoo/raw/main/models/face_detection_yunet/face_detection_yunet_2023mar.onnx"
$SFaceUrl = "https://github.com/opencv/opencv_zoo/raw/main/models/face_recognition_sface/face_recognition_sface_2021dec.onnx"
$TaskName = "FaceSignFaceEngine"

function Assert-Administrator {
    $identity = [Security.Principal.WindowsIdentity]::GetCurrent()
    $principal = New-Object Security.Principal.WindowsPrincipal($identity)
    if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
        Write-Host "[FaceSign] Administrator privileges are required. Re-launching..." -ForegroundColor Yellow
        $arguments = @("-NoProfile", "-ExecutionPolicy", "Bypass", "-File", ('"' + $PSCommandPath + '"'), "-InstallDir", ('"' + $InstallDir + '"'), "-Port", $Port)
        Start-Process powershell.exe -Verb RunAs -ArgumentList ($arguments -join ' ')
        exit 0
    }
}

function Download-File([string]$Url, [string]$Destination, [long]$MinimumBytes = 1) {
    if (Test-Path $Destination) {
        $existing = (Get-Item $Destination).Length
        if ($existing -ge $MinimumBytes) { return }
        Remove-Item $Destination -Force
    }
    Write-Host "[FaceSign] Downloading $Url" -ForegroundColor Cyan
    Invoke-WebRequest -Uri $Url -OutFile $Destination -UseBasicParsing
    $size = (Get-Item $Destination).Length
    if ($size -lt $MinimumBytes) {
        throw "Downloaded file is unexpectedly small: $Destination ($size bytes)"
    }
}

Assert-Administrator

$PythonDir = Join-Path $InstallDir "python"
$ModelsDir = Join-Path $InstallDir "models"
$DataDir = Join-Path $InstallDir "data"
$TempDir = Join-Path $env:TEMP ("facesign-localcpu-" + [Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Force -Path $InstallDir, $ModelsDir, $DataDir, $TempDir | Out-Null

$existingTask = Get-ScheduledTask -TaskName $TaskName -ErrorAction SilentlyContinue
if ($existingTask) {
    Stop-ScheduledTask -TaskName $TaskName -ErrorAction SilentlyContinue
}

try {
    $PythonExe = Join-Path $PythonDir "python.exe"
    if (-not (Test-Path $PythonExe)) {
        $PythonZip = Join-Path $TempDir "python.zip"
        Download-File $PythonZipUrl $PythonZip 10000000
        New-Item -ItemType Directory -Force -Path $PythonDir | Out-Null
        Expand-Archive -Path $PythonZip -DestinationPath $PythonDir -Force
    }

    $PthFile = Get-ChildItem -Path $PythonDir -Filter "python*._pth" -File | Select-Object -First 1
    if (-not $PthFile) { throw "Embedded Python _pth file was not found." }
    @(
        "python311.zip",
        ".",
        "Lib\site-packages",
        "import site"
    ) | Set-Content -Path $PthFile.FullName -Encoding ASCII

    $SitePackages = Join-Path $PythonDir "Lib\site-packages"
    New-Item -ItemType Directory -Force -Path $SitePackages | Out-Null

    $GetPip = Join-Path $TempDir "get-pip.py"
    Download-File $GetPipUrl $GetPip 100000
    Write-Host "[FaceSign] Preparing isolated Python runtime..." -ForegroundColor Cyan
    & $PythonExe $GetPip --disable-pip-version-check --no-warn-script-location
    if ($LASTEXITCODE -ne 0) { throw "get-pip failed with exit code $LASTEXITCODE" }

    Write-Host "[FaceSign] Installing CPU-only face engine dependencies..." -ForegroundColor Cyan
    & $PythonExe -m pip install --disable-pip-version-check --no-warn-script-location --upgrade `
        "numpy==1.26.4" `
        "opencv-python-headless==4.10.0.84" `
        "fastapi==0.115.6" `
        "uvicorn==0.34.0" `
        "python-multipart==0.0.20"
    if ($LASTEXITCODE -ne 0) { throw "pip install failed with exit code $LASTEXITCODE" }

    $SourceEngine = Join-Path $PSScriptRoot "face_engine.py"
    if (-not (Test-Path $SourceEngine)) { throw "face_engine.py is missing from the release package." }
    Copy-Item -Path $SourceEngine -Destination (Join-Path $InstallDir "face_engine.py") -Force

    Download-File $YuNetUrl (Join-Path $ModelsDir "face_detection_yunet_2023mar.onnx") 100000
    Download-File $SFaceUrl (Join-Path $ModelsDir "face_recognition_sface_2021dec.onnx") 10000000

    $Launcher = Join-Path $InstallDir "run-face-engine.cmd"
    $LauncherContent = @"
@echo off
cd /d "$InstallDir"
"$PythonExe" "$InstallDir\face_engine.py" --host 127.0.0.1 --port $Port --models "$ModelsDir" --data "$DataDir\faces.db"
"@
    Set-Content -Path $Launcher -Value $LauncherContent -Encoding ASCII

    Write-Host "[FaceSign] Registering startup task..." -ForegroundColor Cyan
    $action = New-ScheduledTaskAction -Execute "cmd.exe" -Argument ("/c `"" + $Launcher + "`"")
    $trigger = New-ScheduledTaskTrigger -AtStartup
    $settings = New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries -RestartCount 999 -RestartInterval (New-TimeSpan -Minutes 1) -ExecutionTimeLimit ([TimeSpan]::Zero)
    Register-ScheduledTask -TaskName $TaskName -Action $action -Trigger $trigger -Settings $settings -User "SYSTEM" -RunLevel Highest -Force | Out-Null
    Start-ScheduledTask -TaskName $TaskName

    Write-Host "[FaceSign] Waiting for local CPU face engine..." -ForegroundColor Cyan
    $ready = $false
    for ($i = 0; $i -lt 60; $i++) {
        try {
            $health = Invoke-RestMethod -Uri "http://127.0.0.1:$Port/health" -TimeoutSec 3
            if ($health.ok) { $ready = $true; break }
        } catch {
            Start-Sleep -Seconds 2
        }
    }
    if (-not $ready) {
        throw "The local CPU face engine did not become ready on port $Port. Check Task Scheduler task '$TaskName'."
    }

    Write-Host "" 
    Write-Host "[FaceSign] Local CPU face engine is ready." -ForegroundColor Green
    Write-Host "  Device: CPU only (GPU/CUDA not required)"
    Write-Host "  Docker: not required"
    Write-Host "  URL: http://127.0.0.1:$Port"
    Write-Host ""
    Write-Host "In FaceSign -> Face Recognition Settings:" -ForegroundColor Yellow
    Write-Host "  Engine: Local CPU Engine"
    Write-Host "  Service URL: http://127.0.0.1:$Port"
    Write-Host "  API Key: leave blank"
    Write-Host "  Recommended similarity threshold: 0.72"
    Write-Host "Then save and click Check Service."
}
finally {
    Remove-Item -Path $TempDir -Recurse -Force -ErrorAction SilentlyContinue
}
