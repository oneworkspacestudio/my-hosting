# Standalone launch template. Keep credentials out of this file/source control.
$ErrorActionPreference = 'Stop'
$program = Join-Path $PSScriptRoot 'MyHosting.exe'
$privateData = Join-Path $env:LOCALAPPDATA 'OneWorkspace/MyHosting'
& $program --data $privateData --listen '127.0.0.1:9007' --origin 'http://127.0.0.1:9007' run
exit $LASTEXITCODE
