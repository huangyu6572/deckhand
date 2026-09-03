# 在仓库根执行，或直接运行本脚本。产物写到本目录（product\）。
$ErrorActionPreference = "Stop"
$product = $PSScriptRoot
$root = Split-Path -Parent $product
Set-Location $root

$go = "C:\Program Files\Go\bin\go.exe"
if (-not (Test-Path $go)) { $go = "go" }

New-Item -ItemType Directory -Force -Path $product | Out-Null
New-Item -ItemType Directory -Force -Path (Join-Path $product "configs\recipes") | Out-Null

Copy-Item (Join-Path $root "configs\connections.example.yaml") (Join-Path $product "configs\connections.example.yaml") -Force
Copy-Item (Join-Path $root "configs\settings.example.yaml") (Join-Path $product "configs\settings.example.yaml") -Force
Copy-Item (Join-Path $root "configs\recipes\artifact-service.example.yaml") (Join-Path $product "configs\recipes\artifact-service.example.yaml") -Force

Write-Host "building hub.exe ..."
& $go build -o (Join-Path $product "hub.exe") ./cmd/hub
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

Write-Host "building hubd.exe ..."
& $go build -o (Join-Path $product "hubd.exe") ./cmd/hubd
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

Get-Item (Join-Path $product "hub.exe"), (Join-Path $product "hubd.exe") |
    Select-Object Name, Length, LastWriteTime | Format-Table -AutoSize
Write-Host "ok -> $product"
