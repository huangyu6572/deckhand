# 转调 scripts\build.ps1，产物写入 product\
$ErrorActionPreference = "Stop"
& (Join-Path $PSScriptRoot "build.ps1")
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
