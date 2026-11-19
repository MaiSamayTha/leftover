# Builds Leftover-Setup.exe, a normal Windows installer: Start menu entry,
# optional desktop shortcut, the 'leftover' command on PATH, and an
# uninstaller in Settings > Apps. Runs on Windows with Go and Inno Setup 6.
#
#   powershell -File scripts/package-windows.ps1 -Version 0.1.1 -Out dist

param(
    [Parameter(Mandatory = $true)][string]$Version,
    [Parameter(Mandatory = $true)][string]$Out
)

$ErrorActionPreference = "Stop"
$Version = $Version.TrimStart("v")
$root = Split-Path -Parent $PSScriptRoot
$build = Join-Path ([IO.Path]::GetTempPath()) ("leftover-win-" + [Guid]::NewGuid())
New-Item -ItemType Directory -Force -Path $Out | Out-Null
$Out = (Resolve-Path $Out).Path

$env:GOOS = "windows"
$env:CGO_ENABLED = "0"
foreach ($arch in "amd64", "arm64") {
    $env:GOARCH = $arch
    $dir = Join-Path $build $arch
    New-Item -ItemType Directory -Force -Path $dir | Out-Null
    Push-Location $root
    try {
        # leftover.exe is the command-line tool. leftoverw.exe is the same
        # program built without a console window, for the Start menu.
        go build -trimpath -ldflags "-s -w -X main.version=$Version" -o (Join-Path $dir "leftover.exe") ./cmd/leftover
        if ($LASTEXITCODE) { throw "go build failed for $arch" }
        go build -trimpath -ldflags "-s -w -H=windowsgui -X main.version=$Version" -o (Join-Path $dir "leftoverw.exe") ./cmd/leftover
        if ($LASTEXITCODE) { throw "go build (windowsgui) failed for $arch" }
    }
    finally { Pop-Location }
}
Remove-Item Env:GOOS, Env:GOARCH

$iscc = (Get-Command iscc.exe -ErrorAction SilentlyContinue).Source
if (-not $iscc) {
    foreach ($p in "${env:ProgramFiles(x86)}\Inno Setup 6\ISCC.exe", "$env:ProgramFiles\Inno Setup 6\ISCC.exe", "$env:LOCALAPPDATA\Programs\Inno Setup 6\ISCC.exe") {
        if (Test-Path $p) { $iscc = $p; break }
    }
}
if (-not $iscc) { throw "Inno Setup 6 (iscc.exe) isn't installed; get it from https://jrsoftware.org/isdl.php or 'choco install innosetup'" }

& $iscc /Q "/DAppVersion=$Version" "/DBuildDir=$build" "/DOutDir=$Out" (Join-Path $root "packaging\windows\leftover.iss")
if ($LASTEXITCODE) { throw "iscc failed" }
Remove-Item -Recurse -Force $build -ErrorAction SilentlyContinue
Write-Host "package-windows: wrote $(Join-Path $Out 'Leftover-Setup.exe')"
