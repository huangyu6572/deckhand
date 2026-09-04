# Deckhand one-click install for Windows.
# Usage:
#   irm https://raw.githubusercontent.com/huangyu6572/deckhand/main/scripts/install.ps1 | iex
#   $env:DECKHAND_PREFIX = "D:\Tools\Deckhand"; irm ... | iex
#   $env:DECKHAND_FROM_SOURCE = "1"; irm ... | iex
#   powershell -File scripts\install.ps1 -Zip $env:USERPROFILE\Downloads\deckhand-windows-amd64.zip
#   $env:DECKHAND_ZIP = "$env:USERPROFILE\Downloads\deckhand-windows-amd64.zip"; irm ... | iex
#
# Default: copies hub.exe + hubd.exe, adds that folder to the current-user PATH,
# sets user env DECKHAND_HOME, and installs the agent skill.

[CmdletBinding()]
param(
    [string]$Repo = "huangyu6572/deckhand",
    [string]$Prefix = "",
    [string]$Version = "latest",
    [string]$Zip = "",
    [switch]$FromSource,
    [switch]$NoPath,
    [switch]$NoSkill
)

$ErrorActionPreference = "Stop"
$ProgressPreference = "SilentlyContinue"
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12

if ($env:DECKHAND_REPO) { $Repo = $env:DECKHAND_REPO }
if ($env:DECKHAND_PREFIX) { $Prefix = $env:DECKHAND_PREFIX }
if ($env:DECKHAND_VERSION) { $Version = $env:DECKHAND_VERSION }
if ($env:DECKHAND_ZIP) { $Zip = $env:DECKHAND_ZIP }
if ($env:DECKHAND_FROM_SOURCE -in @("1", "true", "True", "yes")) { $FromSource = $true }
if ($env:DECKHAND_NO_PATH -in @("1", "true", "True", "yes")) { $NoPath = $true }
if ($env:DECKHAND_NO_SKILL -in @("1", "true", "True", "yes")) { $NoSkill = $true }

if (-not $Prefix) {
    $Prefix = Join-Path $env:LOCALAPPDATA "Programs\Deckhand"
}

$ua = "Deckhand-install"
$api = "https://api.github.com/repos/$Repo"
$headers = @{
    "User-Agent" = $ua
    "Accept"     = "application/vnd.github+json"
}
$script:LastSkillSrc = $null

function Write-Step([string]$msg) { Write-Host ">> $msg" }
function Write-Hint([string]$msg) { Write-Host "   $msg" }

function Get-UrlCandidates([string]$url) {
    $out = New-Object System.Collections.Generic.List[string]
    if ($env:DECKHAND_MIRROR) {
        $out.Add(($env:DECKHAND_MIRROR.TrimEnd("/") + "/" + $url))
    }
    $out.Add($url)
    foreach ($m in @("https://ghfast.top", "https://gh-proxy.com", "https://ghproxy.net")) {
        $out.Add("$m/$url")
    }
    return $out
}

function Save-UrlOnce([string]$url, [string]$outFile) {
    if (Test-Path $outFile) { Remove-Item -LiteralPath $outFile -Force }
    $curl = Get-Command curl.exe -ErrorAction SilentlyContinue
    if ($curl) {
        # no -s: show a progress bar. Stall ( <1KB/s for 20s ) or connect timeout aborts so we can try a mirror.
        & curl.exe -fL --progress-bar --connect-timeout 15 --max-time 180 --retry 1 --retry-delay 1 --speed-limit 1024 --speed-time 20 -A $ua -o $outFile $url
        if ($LASTEXITCODE -ne 0) { throw "curl exit $LASTEXITCODE" }
    } else {
        Invoke-WebRequest -Uri $url -OutFile $outFile -UseBasicParsing -TimeoutSec 180 -Headers @{ "User-Agent" = $ua }
    }
    if (-not (Test-Path $outFile) -or (Get-Item -LiteralPath $outFile).Length -le 0) {
        throw "empty download"
    }
}

function Save-Url([string]$url, [string]$outFile) {
    $dir = Split-Path -Parent $outFile
    if ($dir) { New-Item -ItemType Directory -Force -Path $dir | Out-Null }
    $last = $null
    foreach ($u in (Get-UrlCandidates $url)) {
        Write-Hint $u
        try {
            Save-UrlOnce $u $outFile
            return
        } catch {
            $last = $_
            Write-Hint ("failed: " + $_.Exception.Message)
        }
    }
    throw "download failed: $url ($last)"
}

function Stop-InstalledBinaries {
    $procs = Get-Process -Name hub, hubd -ErrorAction SilentlyContinue
    if (-not $procs) { return }
    Write-Step "stopping running hub/hubd so files can be replaced"
    $procs | ForEach-Object {
        Write-Hint "$($_.ProcessName) pid=$($_.Id)"
        Stop-Process -Id $_.Id -Force -ErrorAction SilentlyContinue
    }
    Start-Sleep -Milliseconds 500
}

function Get-GoExe {
    $p = "C:\Program Files\Go\bin\go.exe"
    if (Test-Path $p) { return $p }
    $cmd = Get-Command go -ErrorAction SilentlyContinue
    if ($cmd) { return $cmd.Source }
    return $null
}

function Expand-Zip([string]$zip, [string]$dest) {
    New-Item -ItemType Directory -Force -Path $dest | Out-Null
    Expand-Archive -LiteralPath $zip -DestinationPath $dest -Force
}

function Find-PayloadDir([string]$root) {
    $hub = Get-ChildItem -LiteralPath $root -Recurse -Filter "hub.exe" -ErrorAction SilentlyContinue |
        Select-Object -First 1
    if ($hub) { return $hub.Directory.FullName }
    return $null
}

function Find-SkillMd([string]$root) {
    if (-not $root -or -not (Test-Path $root)) { return $null }
    $direct = Join-Path $root "skills\deckhand\SKILL.md"
    if (Test-Path $direct) { return $direct }
    $found = Get-ChildItem -LiteralPath $root -Recurse -Filter "SKILL.md" -ErrorAction SilentlyContinue |
        Where-Object { $_.Directory.Name -eq "deckhand" } |
        Select-Object -First 1
    if ($found) { return $found.FullName }
    return $null
}

function Copy-SkillIntoPrefix([string]$skillMd) {
    if (-not $skillMd -or -not (Test-Path $skillMd)) { return }
    $destDir = Join-Path $Prefix "skills\deckhand"
    New-Item -ItemType Directory -Force -Path $destDir | Out-Null
    Copy-Item -LiteralPath $skillMd -Destination (Join-Path $destDir "SKILL.md") -Force
    $script:LastSkillSrc = Join-Path $destDir "SKILL.md"
}

function Install-FromDir([string]$src) {
    Stop-InstalledBinaries
    New-Item -ItemType Directory -Force -Path $Prefix | Out-Null
    foreach ($name in @("hub.exe", "hubd.exe")) {
        $from = Join-Path $src $name
        if (-not (Test-Path $from)) {
            throw "missing $name in $src"
        }
        Copy-Item -LiteralPath $from -Destination (Join-Path $Prefix $name) -Force
    }
    Get-ChildItem -LiteralPath $src -Filter "*.md" -ErrorAction SilentlyContinue |
        Copy-Item -Destination $Prefix -Force
    Copy-SkillIntoPrefix (Find-SkillMd $src)
}

function Add-UserPath([string]$dir) {
    if ($NoPath) { return }
    $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
    if (-not $userPath) { $userPath = "" }
    $parts = @($userPath -split ";" | Where-Object { $_ -ne "" })
    $exists = $parts | Where-Object { $_.TrimEnd("\") -ieq $dir.TrimEnd("\") }
    if ($exists) {
        Write-Hint "PATH already has $dir"
    } else {
        $newPath = if ($userPath.Trim() -eq "") { $dir } else { "$userPath;$dir" }
        [Environment]::SetEnvironmentVariable("Path", $newPath, "User")
        Write-Hint "user PATH += $dir"
    }
    $env:Path = "$dir;$env:Path"
}

function Set-UserEnv([string]$name, [string]$value) {
    [Environment]::SetEnvironmentVariable($name, $value, "User")
    Set-Item -Path "Env:$name" -Value $value
    Write-Hint "$name = $value"
}

function Get-SkillSource {
    if ($script:LastSkillSrc -and (Test-Path $script:LastSkillSrc)) {
        return $script:LastSkillSrc
    }
    $inPrefix = Find-SkillMd $Prefix
    if ($inPrefix) { return $inPrefix }
    if ($PSScriptRoot) {
        $local = Join-Path $PSScriptRoot "..\skills\deckhand\SKILL.md"
        if (Test-Path $local) { return (Resolve-Path $local).Path }
    }
    return $null
}

function Install-UserSkill {
    if ($NoSkill) {
        Write-Hint "skip skill (DECKHAND_NO_SKILL)"
        return
    }
    Write-Step "installing agent skill"
    $src = Get-SkillSource
    $tmp = $null
    try {
        if (-not $src) {
            $url = "https://raw.githubusercontent.com/$Repo/main/skills/deckhand/SKILL.md"
            Write-Hint "download $url"
            $tmp = Join-Path $env:TEMP ("deckhand-skill-" + [guid]::NewGuid().ToString("N") + ".md")
            Save-Url $url $tmp
            $src = $tmp
        }
        Copy-SkillIntoPrefix $src
        $dests = @(
            (Join-Path $env:USERPROFILE ".cursor\skills\deckhand"),
            (Join-Path $env:USERPROFILE ".agents\skills\deckhand"),
            (Join-Path $env:USERPROFILE ".copilot\skills\deckhand")
        )
        foreach ($d in $dests) {
            New-Item -ItemType Directory -Force -Path $d | Out-Null
            Copy-Item -LiteralPath $src -Destination (Join-Path $d "SKILL.md") -Force
            Write-Hint $d
        }
    } finally {
        if ($tmp) { Remove-Item -LiteralPath $tmp -Force -ErrorAction SilentlyContinue }
    }
}

function Install-FromZipFile([string]$zipPath) {
    $zipPath = $zipPath.Trim('"')
    if (-not (Test-Path -LiteralPath $zipPath)) {
        throw "zip not found: $zipPath"
    }
    Write-Step "installing from local zip"
    Write-Hint $zipPath
    $tmp = Join-Path $env:TEMP ("deckhand-zip-" + [guid]::NewGuid().ToString("N"))
    New-Item -ItemType Directory -Force -Path $tmp | Out-Null
    try {
        Expand-Zip $zipPath $tmp
        $payload = Find-PayloadDir $tmp
        if (-not $payload) { throw "zip has no hub.exe" }
        Write-Step "installing to $Prefix"
        Install-FromDir $payload
        $fromExtract = Find-SkillMd $tmp
        if ($fromExtract) { Copy-SkillIntoPrefix $fromExtract }
        return $true
    } finally {
        Remove-Item -LiteralPath $tmp -Recurse -Force -ErrorAction SilentlyContinue
    }
}

function Get-ReleaseJson {
    $url = if ($Version -eq "latest") {
        "$api/releases/latest"
    } else {
        "$api/releases/tags/$Version"
    }
    try {
        return Invoke-RestMethod -Uri $url -Headers $headers -TimeoutSec 20
    } catch {
        $resp = $_.Exception.Response
        if ($resp -and [int]$resp.StatusCode -eq 404) { return $null }
        throw
    }
}

function Get-ReleaseZipUrl {
    $rel = $null
    try { $rel = Get-ReleaseJson } catch {
        Write-Hint ("release API: " + $_.Exception.Message)
    }
    $name = "deckhand-windows-amd64.zip"
    if ($rel) {
        $asset = $rel.assets | Where-Object { $_.name -eq $name } | Select-Object -First 1
        if ($asset -and $asset.browser_download_url) {
            return @{ Url = [string]$asset.browser_download_url; Tag = [string]$rel.tag_name }
        }
        if ($rel.tag_name) {
            return @{ Url = "https://github.com/$Repo/releases/download/$($rel.tag_name)/$name"; Tag = [string]$rel.tag_name }
        }
    }
    if ($Version -ne "latest") {
        return @{ Url = "https://github.com/$Repo/releases/download/$Version/$name"; Tag = $Version }
    }
    return @{ Url = "https://github.com/$Repo/releases/latest/download/$name"; Tag = "latest" }
}

function Install-FromRelease {
    Write-Step "checking GitHub release ($Version) ..."
    $info = Get-ReleaseZipUrl
    if (-not $info -or -not $info.Url) {
        Write-Hint "no release found"
        return $false
    }
    $tmp = Join-Path $env:TEMP ("deckhand-install-" + [guid]::NewGuid().ToString("N"))
    New-Item -ItemType Directory -Force -Path $tmp | Out-Null
    try {
        $zip = Join-Path $tmp "deckhand-windows-amd64.zip"
        Write-Step "downloading $($info.Tag) (progress below; slow GitHub will switch mirror) ..."
        try {
            Save-Url $info.Url $zip
        } catch {
            Write-Hint $_.Exception.Message
            return $false
        }
        $extract = Join-Path $tmp "extract"
        Expand-Zip $zip $extract
        $payload = Find-PayloadDir $extract
        if (-not $payload) { throw "zip has no hub.exe" }
        Write-Step "installing to $Prefix"
        Install-FromDir $payload
        $fromExtract = Find-SkillMd $extract
        if ($fromExtract) { Copy-SkillIntoPrefix $fromExtract }
        return $true
    } finally {
        Remove-Item -LiteralPath $tmp -Recurse -Force -ErrorAction SilentlyContinue
    }
}

function Install-FromSource {
    $go = Get-GoExe
    if (-not $go) {
        throw "need Go 1.24+ to build from source. Install https://go.dev/dl/ or wait for a GitHub Release."
    }
    $tmp = Join-Path $env:TEMP ("deckhand-src-" + [guid]::NewGuid().ToString("N"))
    New-Item -ItemType Directory -Force -Path $tmp | Out-Null
    try {
        $zip = Join-Path $tmp "src.zip"
        $srcUrl = "https://github.com/$Repo/archive/refs/heads/main.zip"
        Write-Step "downloading source $srcUrl"
        Save-Url $srcUrl $zip
        $extract = Join-Path $tmp "extract"
        Expand-Zip $zip $extract
        $root = Get-ChildItem -LiteralPath $extract -Directory | Select-Object -First 1
        if (-not $root) { throw "source zip is empty" }
        Write-Step "building with $go"
        Push-Location $root.FullName
        try {
            & $go build -o (Join-Path $root.FullName "product\hub.exe") ./cmd/hub
            if ($LASTEXITCODE -ne 0) { throw "go build hub failed" }
            & $go build -o (Join-Path $root.FullName "product\hubd.exe") ./cmd/hubd
            if ($LASTEXITCODE -ne 0) { throw "go build hubd failed" }
        } finally {
            Pop-Location
        }
        $product = Join-Path $root.FullName "product"
        Write-Step "installing to $Prefix"
        Install-FromDir $product
        Copy-SkillIntoPrefix (Find-SkillMd $root.FullName)
    } finally {
        Remove-Item -LiteralPath $tmp -Recurse -Force -ErrorAction SilentlyContinue
    }
}

Write-Host "Deckhand install"
Write-Hint "repo    $Repo"
Write-Hint "prefix  $Prefix"

$ok = $false
if ($Zip) {
    $ok = Install-FromZipFile $Zip
} elseif (-not $FromSource) {
    $ok = Install-FromRelease
}
if (-not $ok) {
    if ($Zip) { throw "local zip install failed" }
    if ($FromSource) {
        Write-Step "building from source (-FromSource)"
    } else {
        Write-Step "falling back to source build"
    }
    Install-FromSource
}

Write-Step "user environment"
Add-UserPath $Prefix
Set-UserEnv "DECKHAND_HOME" $Prefix
Install-UserSkill

$hub = Join-Path $Prefix "hub.exe"
$hubd = Join-Path $Prefix "hubd.exe"
if (-not ((Test-Path $hub) -and (Test-Path $hubd))) {
    throw "install finished but hub.exe / hubd.exe missing under $Prefix"
}

Write-Host ""
Write-Host "ok  $hub"
Write-Host "    $hubd"
Write-Host "    DECKHAND_HOME=$Prefix"
Write-Host ""
Write-Host "open a NEW terminal, then:"
Write-Host "    hub run --json user@192.168.1.20 -- uname -a"
Write-Host "    hub target list --json"
Write-Host ""
Write-Host "config:  %LOCALAPPDATA%\LocalAIHub\"
Write-Host "docs:    $Prefix"
Write-Host "skill:   %USERPROFILE%\.cursor\skills\deckhand\"
Write-Host "do not put passwords in yaml; use: hub secret set <name>"
