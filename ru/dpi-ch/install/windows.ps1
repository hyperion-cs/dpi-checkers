$ErrorActionPreference = "Stop"

$Repo = "hyperion-cs/dpi-checkers"
$Platform = "windows-amd64"

if (-not $env:LOCALAPPDATA) {
    Write-Error "LOCALAPPDATA is not set"
    exit 1
}

$OsArch = [Runtime.InteropServices.RuntimeInformation]::OSArchitecture

if (-not [Environment]::Is64BitProcess -or $OsArch -ne "X64") {
    Write-Error "Unsupported architecture: $OsArch"
    exit 1
}

$AppDir = Join-Path $env:LOCALAPPDATA "dpi-ch"
$BinPath = Join-Path $AppDir "dpich.exe"

Write-Host "Platform detected: $Platform"

$TmpDir = New-Item -ItemType Directory -Path (Join-Path ([IO.Path]::GetTempPath()) ([IO.Path]::GetRandomFileName()))
$TmpZip = Join-Path $TmpDir "archive.zip"

try {
    New-Item -ItemType Directory -Force -Path $AppDir | Out-Null
    Write-Host "Install directory prepared: $AppDir"

    $ReleaseUrl = "https://api.github.com/repos/$Repo/releases/latest"
    $Release = Invoke-RestMethod -Uri $ReleaseUrl
    Write-Host "Latest release info fetched: https://github.com/$Repo/releases/latest"

    $Asset = $Release.assets |
        Where-Object { $_.browser_download_url -match "-$Platform\.zip$" } |
        Select-Object -First 1

    if (-not $Asset) {
        Write-Error "No release archive found for platform: $Platform"
        exit 1
    }

    Invoke-WebRequest -Uri $Asset.browser_download_url -OutFile $TmpZip
    Write-Host "Archive downloaded: $($Asset.browser_download_url)"

    Expand-Archive -Path $TmpZip -DestinationPath $AppDir -Force
    Write-Host "Archive extracted to: $AppDir"

    if (-not (Test-Path $BinPath)) {
        Write-Error "Binary not found after extraction: $BinPath"
        exit 1
    }

    $UserPath = [Environment]::GetEnvironmentVariable("Path", "User")
    $PathItems = $UserPath -split ";" | ForEach-Object { $_.TrimEnd("\") }
    $NormalizedAppDir = $AppDir.TrimEnd("\")

    if ($PathItems -contains $NormalizedAppDir) {
        Write-Host
        Write-Host "PATH already contains: $AppDir"
        Write-Host "Run:"
        Write-Host "  dpich"
    } else {
        Write-Host
        Write-Host "PATH does not contain: $AppDir"
        Write-Host
        Write-Host "Run without PATH:"
        Write-Host "  $BinPath"
        Write-Host
        Write-Host "To run simply as 'dpich', add this directory to your user PATH:"
        Write-Host "  $AppDir"
    }

    Write-Host
    Write-Host "Successfully installed: $BinPath"
}
finally {
    Remove-Item -Recurse -Force $TmpDir -ErrorAction SilentlyContinue
}