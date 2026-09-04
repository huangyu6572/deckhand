# Build product\ then zip a GitHub Release asset:
#   dist\deckhand-windows-amd64.zip
$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
$product = Join-Path $root "product"
$dist = Join-Path $root "dist"
$zipName = "deckhand-windows-amd64.zip"
$zip = Join-Path $dist $zipName

& (Join-Path $PSScriptRoot "build.ps1")
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

foreach ($name in @("hub.exe", "hubd.exe")) {
    $p = Join-Path $product $name
    if (-not (Test-Path $p)) { throw "missing $p" }
}

New-Item -ItemType Directory -Force -Path $dist | Out-Null
if (Test-Path $zip) { Remove-Item -LiteralPath $zip -Force }

$stage = Join-Path $dist "stage-windows-amd64"
if (Test-Path $stage) { Remove-Item -LiteralPath $stage -Recurse -Force }
New-Item -ItemType Directory -Force -Path $stage | Out-Null

Copy-Item (Join-Path $product "hub.exe") $stage
Copy-Item (Join-Path $product "hubd.exe") $stage
Get-ChildItem -LiteralPath $product -Filter "*.md" | Copy-Item -Destination $stage
$skillSrc = Join-Path $root "skills\deckhand"
if (Test-Path $skillSrc) {
    $skillDst = Join-Path $stage "skills\deckhand"
    New-Item -ItemType Directory -Force -Path $skillDst | Out-Null
    Copy-Item (Join-Path $skillSrc "SKILL.md") $skillDst
}

Compress-Archive -Path (Join-Path $stage "*") -DestinationPath $zip -Force
Remove-Item -LiteralPath $stage -Recurse -Force

Get-Item $zip | Select-Object Name, Length, LastWriteTime | Format-Table -AutoSize
Write-Host "ok -> $zip"
