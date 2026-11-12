# Installs sb, the StringBird CLI, from GitHub releases on Windows.
#
#   irm https://raw.githubusercontent.com/string-bird/string-bird-cli/main/install.ps1 | iex
#
# Environment:
#   SB_VERSION      release tag to install, e.g. v0.1.0 (default or "latest": the latest release)
#   SB_INSTALL_DIR  where to put sb.exe (default: %LOCALAPPDATA%\Programs\sb)
#   SB_DOWNLOAD_BASE  releases URL (default: https://github.com/string-bird/string-bird-cli/releases)
#
# The archive is checked against the release's checksums.txt before installing,
# and the folder is added to the user's PATH.
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'

$base = $env:SB_DOWNLOAD_BASE
if (-not $base) { $base = 'https://github.com/string-bird/string-bird-cli/releases' }
$base = $base.TrimEnd('/')

$arch = switch ($env:PROCESSOR_ARCHITECTURE) {
    'AMD64' { 'amd64' }
    'ARM64' { 'arm64' }
    default { throw "install.ps1: unsupported architecture: $env:PROCESSOR_ARCHITECTURE" }
}

$tag = $env:SB_VERSION
if (-not $tag -or $tag -eq 'latest') {
    # The latest release redirects to .../releases/tag/<tag>; no API call needed.
    $resp = Invoke-WebRequest -Uri "$base/latest" -UseBasicParsing -MaximumRedirection 5
    $final = $resp.BaseResponse.ResponseUri
    if (-not $final) { $final = $resp.BaseResponse.RequestMessage.RequestUri }
    $tag = ($final.AbsoluteUri -split '/')[-1]
    if (-not $tag.StartsWith('v')) { throw "install.ps1: no release found (got $final)" }
}
if (-not $tag.StartsWith('v')) { $tag = "v$tag" }
$version = $tag.Substring(1)

$dir = $env:SB_INSTALL_DIR
if (-not $dir) { $dir = Join-Path $env:LOCALAPPDATA 'Programs\sb' }

$archive = "sb_${version}_windows_${arch}.zip"
$tmp = Join-Path ([IO.Path]::GetTempPath()) ([IO.Path]::GetRandomFileName())
New-Item -ItemType Directory -Path $tmp | Out-Null
try {
    Write-Host "Downloading sb $tag (windows/$arch)"
    Invoke-WebRequest -Uri "$base/download/$tag/$archive" -OutFile (Join-Path $tmp $archive) -UseBasicParsing
    Invoke-WebRequest -Uri "$base/download/$tag/checksums.txt" -OutFile (Join-Path $tmp 'checksums.txt') -UseBasicParsing

    $want = Get-Content (Join-Path $tmp 'checksums.txt') |
        ForEach-Object { $f = $_ -split '\s+'; if ($f[1] -eq $archive) { $f[0] } } |
        Select-Object -First 1
    if (-not $want) { throw "install.ps1: $archive isn't in checksums.txt" }
    $got = (Get-FileHash (Join-Path $tmp $archive) -Algorithm SHA256).Hash.ToLower()
    if ($want -ne $got) { throw "install.ps1: checksum mismatch for ${archive}: want $want, got $got" }

    Expand-Archive -Path (Join-Path $tmp $archive) -DestinationPath (Join-Path $tmp 'x')
    New-Item -ItemType Directory -Force -Path $dir | Out-Null
    Move-Item -Force (Join-Path $tmp 'x\sb.exe') (Join-Path $dir 'sb.exe')
} finally {
    Remove-Item -Recurse -Force $tmp
}
Write-Host "Installed sb $tag to $dir\sb.exe"

$userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
if (-not (($userPath -split ';') -contains $dir)) {
    [Environment]::SetEnvironmentVariable('Path', ($(if ($userPath) { "$userPath;" } else { '' }) + $dir), 'User')
    Write-Host "Added $dir to your PATH; open a new terminal to run sb."
}
