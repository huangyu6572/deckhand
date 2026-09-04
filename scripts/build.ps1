# 在仓库根执行，或直接运行本脚本。产物写到 product\（exe + 使用说明，不含源码模板）。
$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
$product = Join-Path $root "product"
Set-Location $root

$go = "C:\Program Files\Go\bin\go.exe"
if (-not (Test-Path $go)) { $go = "go" }

New-Item -ItemType Directory -Force -Path $product | Out-Null

Write-Host "building hub.exe ..."
& $go build -o (Join-Path $product "hub.exe") ./cmd/hub
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

Write-Host "building hubd.exe ..."
& $go build -o (Join-Path $product "hubd.exe") ./cmd/hubd
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

Get-Item (Join-Path $product "hub.exe"), (Join-Path $product "hubd.exe") |
    Select-Object Name, Length, LastWriteTime | Format-Table -AutoSize
Write-Host "ok -> $product"
