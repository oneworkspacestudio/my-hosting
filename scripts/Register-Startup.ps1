param(
 [string]$Program = (Join-Path (Split-Path $PSScriptRoot -Parent) 'MyHosting.exe'),
 [string]$Data = (Join-Path $env:LOCALAPPDATA 'OneWorkspace/MyHosting'),
 [switch]$Remove
)
$ErrorActionPreference='Stop'
# One task per current user; never use SYSTEM or store account credentials.
$sid=[Security.Principal.WindowsIdentity]::GetCurrent().User.Value
$name='OneWorkspace-MyHosting-'+$sid
if($Remove){Unregister-ScheduledTask -TaskName $name -Confirm:$false -ErrorAction SilentlyContinue;return}
$exe=(Resolve-Path -LiteralPath $Program).Path
if($exe.Contains('"') -or $Data.Contains('"')){throw 'Paths cannot contain quote characters'}
$identity=[Security.Principal.WindowsIdentity]::GetCurrent().Name
$action=New-ScheduledTaskAction -Execute $exe -Argument ('--data "'+[IO.Path]::GetFullPath($Data)+'" --listen 127.0.0.1:9007 --origin http://127.0.0.1:9007 --no-browser run')
$trigger=New-ScheduledTaskTrigger -AtLogOn -User $identity
$principal=New-ScheduledTaskPrincipal -UserId $identity -LogonType Interactive -RunLevel Limited
$settings=New-ScheduledTaskSettingsSet -MultipleInstances IgnoreNew -ExecutionTimeLimit ([TimeSpan]::Zero) -RestartCount 3 -RestartInterval (New-TimeSpan -Minutes 1) -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries
if(Get-ScheduledTask -TaskName $name -ErrorAction SilentlyContinue){throw 'Startup task already exists; remove it explicitly before replacing it'}
Register-ScheduledTask -TaskName $name -Action $action -Trigger $trigger -Principal $principal -Settings $settings -Description 'My Hosting under the signed-in Windows user; no SYSTEM builds.' | Out-Null
Write-Output 'My Hosting will start when this Windows user signs in. Existing project data is preserved.'
