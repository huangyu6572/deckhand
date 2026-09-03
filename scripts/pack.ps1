# 在仓库根目录或直接运行本脚本：生成 product\ 下的 exe（转调 product\build.ps1）
$ErrorActionPreference = "Stop"
& (Join-Path $PSScriptRoot "..\product\build.ps1")
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
