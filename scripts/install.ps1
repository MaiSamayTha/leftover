# Install Leftover on Windows.
#
#   irm https://raw.githubusercontent.com/MaiSamayTha/leftover/main/scripts/install.ps1 | iex
#
# What it does:
#   1. downloads the latest release for your CPU from GitHub
#   2. checks it against the release's published SHA-256 checksums
#   3. installs it to %LOCALAPPDATA%\Programs\Leftover (no admin rights needed)
#   4. adds that folder to your user PATH and a Start menu shortcut
#
# Set $env:LEFTOVER_VERSION = "v0.1.0" first to pick a version. To uninstall,
# run:  & ([scriptblock]::Create((irm <this url>))) -Uninstall

param([switch]$Uninstall)

$ErrorActionPreference = "Stop"
$ProgressPreference = "SilentlyContinue"

$Repo = "MaiSamayTha/leftover"
$Dir = Join-Path $env:LOCALAPPDATA "Programs\Leftover"
# The Start menu folder can be missing (unusual profiles); then there's simply
# no shortcut, rather than a failed install.
$StartMenu = [Environment]::GetFolderPath("Programs")
$Shortcut = if ($StartMenu) { Join-Path $StartMenu "Leftover.lnk" } else { $null }

function Remove-FromPath([string]$folder) {
    $path = [Environment]::GetEnvironmentVariable("Path", "User")
    if ($path) {
        $parts = $path.Split(";") | Where-Object { $_ -and ($_.TrimEnd("\") -ne $folder.TrimEnd("\")) }
        [Environment]::SetEnvironmentVariable("Path", ($parts -join ";"), "User")
    }
}

if ($Uninstall) {
    Remove-Item -Recurse -Force $Dir -ErrorAction SilentlyContinue
    if ($Shortcut) { Remove-Item -Force $Shortcut -ErrorAction SilentlyContinue }
    Remove-FromPath $Dir
    Write-Host "Leftover is uninstalled. Its data folder (settings, history and any vault) is in $env:LOCALAPPDATA\Leftover."
    return
}

$arch = if ($env:PROCESSOR_ARCHITECTURE -eq "ARM64" -or $env:PROCESSOR_ARCHITEW6432 -eq "ARM64") { "arm64" } else { "amd64" }
$version = if ($env:LEFTOVER_VERSION) { $env:LEFTOVER_VERSION } else { "latest" }
$base = if ($version -eq "latest") { "https://github.com/$Repo/releases/latest/download" } else { "https://github.com/$Repo/releases/download/$version" }
$asset = "leftover-windows-$arch.zip"

$tmp = Join-Path ([IO.Path]::GetTempPath()) ("leftover-" + [Guid]::NewGuid())
New-Item -ItemType Directory -Path $tmp | Out-Null
try {
    [Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
    Write-Host "Downloading $asset..."
    Invoke-WebRequest -UseBasicParsing -Uri "$base/$asset" -OutFile (Join-Path $tmp $asset)
    Invoke-WebRequest -UseBasicParsing -Uri "$base/checksums.txt" -OutFile (Join-Path $tmp "checksums.txt")

    $line = Get-Content (Join-Path $tmp "checksums.txt") | Where-Object { ($_ -split "\s+")[1] -in @($asset, "*$asset") } | Select-Object -First 1
    if (-not $line) { throw "$asset isn't listed in checksums.txt" }
    $want = ($line -split "\s+")[0].ToLower()
    $got = (Get-FileHash -Algorithm SHA256 (Join-Path $tmp $asset)).Hash.ToLower()
    if ($want -ne $got) { throw "Checksum mismatch for $asset (expected $want, got $got). Nothing was installed." }
    Write-Host "Checksum verified."

    Expand-Archive -Path (Join-Path $tmp $asset) -DestinationPath (Join-Path $tmp "x") -Force
    $exe = Join-Path $tmp "x\leftover.exe"
    if (-not (Test-Path $exe)) { throw "The archive didn't contain leftover.exe" }

    New-Item -ItemType Directory -Force -Path $Dir | Out-Null
    Copy-Item -Force $exe (Join-Path $Dir "leftover.exe")
    $ico = Join-Path $tmp "x\leftover.ico"
    if (Test-Path $ico) { Copy-Item -Force $ico (Join-Path $Dir "leftover.ico") }

    $path = [Environment]::GetEnvironmentVariable("Path", "User")
    if (-not $path -or -not ($path.Split(";") | Where-Object { $_.TrimEnd("\") -eq $Dir.TrimEnd("\") })) {
        [Environment]::SetEnvironmentVariable("Path", (($path, $Dir) | Where-Object { $_ }) -join ";", "User")
        $env:Path = "$env:Path;$Dir"
    }

    if ($Shortcut) {
        $shell = New-Object -ComObject WScript.Shell
        $lnk = $shell.CreateShortcut($Shortcut)
        $lnk.TargetPath = Join-Path $Dir "leftover.exe"
        $lnk.Arguments = "app"
        $lnk.WorkingDirectory = $Dir
        $lnk.Description = "See what your dev tools left behind and clear it safely"
        if (Test-Path (Join-Path $Dir "leftover.ico")) { $lnk.IconLocation = Join-Path $Dir "leftover.ico" }
        $lnk.WindowStyle = 7
        $lnk.Save()
    }

    Write-Host ""
    Write-Host ("Installed " + (& (Join-Path $Dir "leftover.exe") version))
    if ($Shortcut) { Write-Host "Open Leftover from the Start menu, or run 'leftover scan' in a new terminal." }
    else { Write-Host "Run 'leftover' in a new terminal to open it, or 'leftover scan' to scan." }
}
finally {
    Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
}
