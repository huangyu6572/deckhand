# Install the Deckhand agent skill for Cursor / Copilot / agents.
# Usage:
#   irm https://raw.githubusercontent.com/huangyu6572/deckhand/main/scripts/install-skill.ps1 | iex
#   powershell -File scripts\install-skill.ps1

[CmdletBinding()]
param(
    [string]$Repo = "huangyu6572/deckhand"
)

$ErrorActionPreference = "Stop"
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12

if ($env:DECKHAND_REPO) { $Repo = $env:DECKHAND_REPO }

$here = $PSScriptRoot
$localSkill = $null
if ($here) {
    $candidate = Join-Path $here "..\skills\deckhand\SKILL.md"
    if (Test-Path $candidate) { $localSkill = (Resolve-Path $candidate).Path }
}

$dests = @(
    (Join-Path $env:USERPROFILE ".cursor\skills\deckhand"),
    (Join-Path $env:USERPROFILE ".agents\skills\deckhand"),
    (Join-Path $env:USERPROFILE ".copilot\skills\deckhand")
)

function Install-SkillFile([string]$src, [string]$destDir) {
    New-Item -ItemType Directory -Force -Path $destDir | Out-Null
    Copy-Item -LiteralPath $src -Destination (Join-Path $destDir "SKILL.md") -Force
    Write-Host "ok  $(Join-Path $destDir 'SKILL.md')"
}

if ($localSkill) {
    Write-Host "Deckhand skill (local)"
    foreach ($d in $dests) { Install-SkillFile $localSkill $d }
} else {
    $url = "https://raw.githubusercontent.com/$Repo/main/skills/deckhand/SKILL.md"
    Write-Host "Deckhand skill"
    Write-Host "   $url"
    $tmp = Join-Path $env:TEMP ("deckhand-skill-" + [guid]::NewGuid().ToString("N") + ".md")
    try {
        Invoke-WebRequest -Uri $url -OutFile $tmp -UseBasicParsing -Headers @{ "User-Agent" = "Deckhand-skill-install" }
        foreach ($d in $dests) { Install-SkillFile $tmp $d }
    } finally {
        Remove-Item -LiteralPath $tmp -Force -ErrorAction SilentlyContinue
    }
}

Write-Host ""
Write-Host "open a NEW chat in Cursor, then ask it to run a remote command with hub."
