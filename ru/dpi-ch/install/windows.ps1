$ErrorActionPreference = "Stop"

$Repo = "hyperion-cs/dpi-checkers"
$Platform = "windows-amd64"

if (-not $env:LOCALAPPDATA) {
    Write-Error "LOCALAPPDATA is not set"
    exit 1
}

$Arch = if ($env:PROCESSOR_ARCHITEW6432) {
    $env:PROCESSOR_ARCHITEW6432
} else {
    $env:PROCESSOR_ARCHITECTURE
}

if ($Arch -ne "AMD64") {
    Write-Error "Unsupported architecture: $Arch"
    exit 1
}

$AppDir = Join-Path $env:LOCALAPPDATA "Programs\dpich"
$LegacyAppDir = Join-Path $env:LOCALAPPDATA "dpi-ch"
$BinPath = Join-Path $AppDir "dpich.exe"

function Update-InstallPath([string]$Value) {
    $Found = $false
    $Entries = @(
        foreach ($Entry in $Value -split ";") {
            $Directory = [Environment]::ExpandEnvironmentVariables($Entry.Trim().Trim('"')).Replace("/", "\").TrimEnd("\")
            if ($Directory -eq $AppDir -or $Directory -eq $LegacyAppDir) {
                if (-not $Found) {
                    $AppDir
                    $Found = $true
                }
            } elseif ($Entry) {
                $Entry
            }
        }
        if (-not $Found) {
            $AppDir
        }
    )
    $Entries -join ";"
}

Write-Host "Platform detected: $Platform"

New-Item -ItemType Directory -Force -Path $AppDir | Out-Null
Write-Host "Install directory prepared: $AppDir"

$TmpDir = Join-Path $AppDir (".dpich-install-" + [IO.Path]::GetRandomFileName())
New-Item -ItemType Directory -Path $TmpDir | Out-Null
$TmpZip = Join-Path $TmpDir "dpich.zip"
$BackupBin = Join-Path $TmpDir "dpich.exe.bak"

try {
    $ReleaseUrl = "https://api.github.com/repos/$Repo/releases/latest"
    $Release = Invoke-RestMethod -Uri $ReleaseUrl
    Write-Host "Latest release info fetched: https://api.github.com/repos/$Repo/releases/latest"

    $Asset = $Release.assets |
        Where-Object { $_.browser_download_url -match "-$Platform\.zip$" } |
        Select-Object -First 1

    if (-not $Asset) {
        Write-Error "No release archive found for platform: $Platform"
        exit 1
    }

    Write-Host "Archive downloading: $($Asset.browser_download_url)"

    Invoke-WebRequest -Uri $Asset.browser_download_url -OutFile $TmpZip
    Write-Host "Archive downloaded: $TmpZip"

    $StagingDir = Join-Path $TmpDir "extracted"
    $StagedBin = Join-Path $StagingDir "dpich.exe"
    Expand-Archive -Path $TmpZip -DestinationPath $StagingDir

    if (-not (Test-Path -LiteralPath $StagedBin -PathType Leaf) -or (Get-Item -LiteralPath $StagedBin).Length -eq 0) {
        throw "Archive contains no usable dpich.exe binary"
    }
    if ([IO.File]::Exists($BinPath)) {
        [IO.File]::Replace($StagedBin, $BinPath, $BackupBin)
    } else {
        [IO.File]::Move($StagedBin, $BinPath)
    }

    $UserPath = [Environment]::GetEnvironmentVariable("Path", "User")
    $UpdatedPath = Update-InstallPath $UserPath
    if ($UserPath -cne $UpdatedPath) {
        [Environment]::SetEnvironmentVariable("Path", $UpdatedPath, "User")
    }
    $env:Path = Update-InstallPath $env:Path
    Write-Host "User PATH configured: $AppDir"

    Write-Host
    Write-Host "Run:"
    Write-Host "  dpich"

    Write-Host
    Write-Host "Successfully installed: $BinPath"
}
finally {
    if ([IO.File]::Exists($BackupBin) -and -not [IO.File]::Exists($BinPath)) {
        try {
            [IO.File]::Move($BackupBin, $BinPath)
        } catch {
            Write-Warning "Previous binary kept at $BackupBin"
            throw
        }
    }
    Remove-Item -LiteralPath $TmpDir -Recurse -Force -ErrorAction SilentlyContinue
}
