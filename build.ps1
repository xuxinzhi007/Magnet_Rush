param(
  [ValidateSet("wasm", "windows", "all")]
  [string]$Target = "all"
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $MyInvocation.MyCommand.Path
Set-Location $root
New-Item -ItemType Directory -Force "dist" | Out-Null

if ($Target -eq "wasm" -or $Target -eq "all") {
  $env:GOOS = "js"
  $env:GOARCH = "wasm"
  go build -o "dist/game.wasm" .
  Copy-Item (Join-Path $root "index.html") (Join-Path $root "dist/index.html") -Force
  Copy-Item (Join-Path $root "wasm_exec.js") (Join-Path $root "dist/wasm_exec.js") -Force
}

if ($Target -eq "windows" -or $Target -eq "all") {
  Remove-Item Env:GOOS -ErrorAction SilentlyContinue
  Remove-Item Env:GOARCH -ErrorAction SilentlyContinue
  go build -o "dist/magnet-rush.exe" .
}

Write-Host "Build complete: $Target"
