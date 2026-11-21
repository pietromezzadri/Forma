<#
Builds a Forma release for Windows: one self-contained forma.exe (statically
linked, so it needs no MinGW DLLs) zipped with its fonts and the licence.

    tools/release/build.ps1 -Version 0.1.0

writes dist/forma-0.1.0-windows-x64.zip and dist/SHA256SUMS.txt. Run from
PowerShell (Git Bash can put the wrong DLLs on the path) with MSYS2's UCRT64
gcc on PATH. The release workflow (.github/workflows/release.yml) runs it on
every v* tag.
#>
param(
    [Parameter(Mandatory)][string]$Version
)
$ErrorActionPreference = 'Stop'
$root = Resolve-Path (Join-Path $PSScriptRoot '..\..')
Set-Location $root

$name = "forma-$Version-windows-x64"
$dist = Join-Path $root 'dist'
$stage = Join-Path $dist $name
Remove-Item -Recurse -Force $stage, "$stage.zip" -ErrorAction SilentlyContinue
New-Item -ItemType Directory -Force $stage | Out-Null

# -H windowsgui: no console window behind the app.
$env:CGO_ENABLED = '1'
$ld = "-s -w -H windowsgui -X forma/app.Version=$Version -extldflags=-static"
go build -trimpath -ldflags $ld -o (Join-Path $stage 'forma.exe') .
if ($LASTEXITCODE) { throw "go build failed" }

# It must run on a PC without MSYS2: only Windows' own DLLs.
$objdump = (Get-Command objdump -ErrorAction SilentlyContinue).Source
if ($objdump) {
    $dlls = & $objdump -p (Join-Path $stage 'forma.exe') | Select-String 'DLL Name: (.+)' | ForEach-Object { $_.Matches[0].Groups[1].Value.Trim() }
    $bad = $dlls | Where-Object { $_ -match '^(lib|msys|cyg)' }
    if ($bad) { throw "forma.exe needs non-system DLLs: $($bad -join ', ')" }
    Write-Host "DLLs: $($dlls -join ', ')"
} else {
    Write-Warning 'objdump not found: the DLL check was skipped'
}

New-Item -ItemType Directory -Force (Join-Path $stage 'assets') | Out-Null
Copy-Item -Recurse assets\fonts (Join-Path $stage 'assets\fonts')
Copy-Item LICENSE, README.md $stage

Compress-Archive -Path $stage -DestinationPath "$stage.zip" -CompressionLevel Optimal
$hash = (Get-FileHash "$stage.zip" -Algorithm SHA256).Hash.ToLower()
"$hash  $name.zip" | Set-Content -Encoding ascii (Join-Path $dist 'SHA256SUMS.txt')
Remove-Item -Recurse -Force $stage

$size = [math]::Round((Get-Item "$stage.zip").Length / 1MB, 1)
Write-Host "dist/$name.zip  $size MB  sha256 $hash"
