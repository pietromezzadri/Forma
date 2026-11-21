# Recompiles the test sketches into the .hex files the tests load.
# Needs arduino-cli with the arduino:avr core and the Servo library:
#   arduino-cli core install arduino:avr; arduino-cli lib install Servo
# Uses $env:FORMA_ARDUINO_CLI, else arduino-cli on the PATH, else
# %LOCALAPPDATA%\arduino-cli\bin\arduino-cli.exe.
$ErrorActionPreference = 'Stop'
$cli = $env:FORMA_ARDUINO_CLI
if (-not $cli) { $cli = (Get-Command arduino-cli -ErrorAction SilentlyContinue).Source }
if (-not $cli) { $cli = "$env:LOCALAPPDATA\arduino-cli\bin\arduino-cli.exe" }
Set-Location $PSScriptRoot
foreach ($dir in Get-ChildItem sketches -Directory) {
    $s = $dir.Name
    $out = Join-Path $env:TEMP "forma-sketch-$s"
    & $cli compile --fqbn arduino:avr:uno --output-dir $out $dir.FullName | Out-Null
    Copy-Item (Join-Path $out "$s.ino.hex") "$s.hex"
    Write-Host "$s.hex"
}
