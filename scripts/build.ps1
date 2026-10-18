param(
 [string]$Go='go',
 [string]$Npm='npm.cmd',
 [switch]$ReuseDependencies,
 [switch]$SkipTests,
 [string]$Output=''
)
$ErrorActionPreference='Stop'
$root=Split-Path $PSScriptRoot -Parent
if(-not $Output){$Output=Join-Path $root 'dist'}
$Output=[IO.Path]::GetFullPath($Output)
New-Item -ItemType Directory -Force -Path $Output|Out-Null
Push-Location (Join-Path $root 'web')
try{
 if(-not $ReuseDependencies){& $Npm ci --audit=false --fund=false;if($LASTEXITCODE){throw 'Frontend dependency installation failed'}}
 & $Npm run build;if($LASTEXITCODE){throw 'Frontend build failed'}
 & $Npm test;if($LASTEXITCODE){throw 'Frontend tests failed'}
}finally{Pop-Location}
Push-Location $root
try{
 if(-not $SkipTests){& $Go test -buildvcs=false ./...;if($LASTEXITCODE){throw 'Backend tests failed'}}
 # Resource source/export pipeline is committed; regenerate after icon changes.
 & $Go build -buildvcs=false -trimpath -ldflags '-s -w' -o (Join-Path $Output 'MyHosting.exe') ./cmd/myhosting
 if($LASTEXITCODE){throw 'Windows executable build failed'}
 $version=(& (Join-Path $Output 'MyHosting.exe') --version).Trim()
 if($version -notmatch '^\d+\.\d+\.\d+(-[A-Za-z0-9.]+)?$'){throw 'Invalid executable version'}
 $stage=Join-Path $Output ('package-'+[guid]::NewGuid().ToString('N'))
 New-Item -ItemType Directory -Path $stage|Out-Null
 foreach($name in @('MyHosting.exe')){Copy-Item -LiteralPath (Join-Path $Output $name) -Destination $stage}
 foreach($name in @('LICENSE','NOTICE','README.md','MyHosting.example.ps1','compatibility.json')){Copy-Item -LiteralPath (Join-Path $root $name) -Destination $stage}
 foreach($dir in @('docs','branding','examples')){
  # Copy source examples only, never npm caches or installed dependencies.
  Get-ChildItem -LiteralPath (Join-Path $root $dir) -File -Recurse | Where-Object {$_.FullName -notmatch '[\\/]node_modules[\\/]|[\\/]\.next[\\/]|[\\/]\.nuxt[\\/]|[\\/]\.output[\\/]'} | ForEach-Object {
   $relative=[IO.Path]::GetRelativePath($root,$_.FullName);$target=Join-Path $stage $relative
   New-Item -ItemType Directory -Force -Path (Split-Path $target -Parent)|Out-Null;Copy-Item -LiteralPath $_.FullName -Destination $target
  }
 }
 New-Item -ItemType Directory -Path (Join-Path $stage 'scripts')|Out-Null
 Copy-Item -LiteralPath (Join-Path $PSScriptRoot 'Register-Startup.ps1') -Destination (Join-Path $stage 'scripts')
 $inventory=[Collections.Generic.List[object]]::new()
 $notice=[Text.StringBuilder]::new()
 [void]$notice.AppendLine('My Hosting third-party attribution. Managed Caddy/Node archives retain their own complete notices.').AppendLine()
 $modules=& $Go list -m -f '{{.Path}}|{{.Version}}|{{.Dir}}' all
 if($LASTEXITCODE){throw 'Go dependency inventory failed'}
 foreach($line in $modules){
  $parts=$line.Split('|');if($parts[0] -eq 'github.com/oneworkspacestudio/my-hosting'){continue}
  $inventory.Add(@{ecosystem='go';name=$parts[0];version=$parts[1]})
  $licenses=@(Get-ChildItem -LiteralPath $parts[2] -File | Where-Object {$_.Name -match '^(LICENSE|LICENCE|COPYING|NOTICE)(\..*)?$'})
  if(-not $licenses.Count){throw ('Missing license for '+$parts[0])}
  [void]$notice.AppendLine(('===== '+$parts[0]+' '+$parts[1]+' ====='))
  foreach($file in $licenses){[void]$notice.AppendLine([IO.File]::ReadAllText($file.FullName))}
 }
 $goRoot=(& $Go env GOROOT).Trim()
 [void]$notice.AppendLine('===== Go runtime =====').AppendLine([IO.File]::ReadAllText((Join-Path $goRoot 'LICENSE')))
 $lock=Get-Content -LiteralPath (Join-Path $root 'web/package-lock.json') -Raw|ConvertFrom-Json
 foreach($item in $lock.packages.PSObject.Properties){
  if(-not $item.Name){continue}
  $inventory.Add(@{ecosystem='npm';name=($item.Name -replace '^.*node_modules/','');version=$item.Value.version;dev=[bool]$item.Value.dev;license=$item.Value.license;integrity=$item.Value.integrity})
 }
 # The shipped browser bundle includes these packages; the AUI compiler generates
 # source at build time, and its license is preserved too.
 foreach($name in @('react','react-dom','scheduler','lucide-react','@codedia/parser')){
  $dir=Join-Path $root ('web/node_modules/'+$name)
  $pkg=Get-Content -LiteralPath (Join-Path $dir 'package.json') -Raw|ConvertFrom-Json
  $files=@(Get-ChildItem -LiteralPath $dir -File|Where-Object {$_.Name -match '^LICENSE(\..*)?$'})
  if(-not $files.Count){throw ('Missing frontend license: '+$name)}
  [void]$notice.AppendLine(('===== '+$name+' '+$pkg.version+' ====='))
  foreach($file in $files){[void]$notice.AppendLine([IO.File]::ReadAllText($file.FullName))}
 }
 [IO.File]::WriteAllText((Join-Path $stage 'THIRD-PARTY-LICENSES.txt'),$notice.ToString(),[Text.UTF8Encoding]::new($false))
 [IO.File]::WriteAllText((Join-Path $stage 'dependencies.json'),(@{schema=1;application='My Hosting';version=$version;components=$inventory}|ConvertTo-Json -Depth 8),[Text.UTF8Encoding]::new($false))
 $pins=[Collections.Generic.List[object]]::new()
 $runtimeSource=[IO.File]::ReadAllText((Join-Path $root 'internal/hosting/runtime.go'))
 foreach($m in [regex]::Matches($runtimeSource,'ID: "(?<id>\w+)", Version: "(?<version>[^"]+)", URL: "(?<url>[^"]+)", SHA256: "(?<sha>[a-f0-9]{64})"')){
  $pins.Add(@{id=$m.Groups['id'].Value;version=$m.Groups['version'].Value;url=$m.Groups['url'].Value;sha256=$m.Groups['sha'].Value})
 }
 if($pins.Count -ne 3){throw 'Runtime inventory does not match the three supported runtimes'}
 [IO.File]::WriteAllText((Join-Path $stage 'runtime-pins.json'),($pins|ConvertTo-Json -Depth 5),[Text.UTF8Encoding]::new($false))
 Add-Type -AssemblyName System.IO.Compression.FileSystem
 $zip=Join-Path $Output ('my-hosting-'+$version+'-windows-amd64.zip')
 if(Test-Path -LiteralPath $zip){throw 'Refusing to overwrite a release ZIP; use a fresh output folder'}
 [IO.Compression.ZipFile]::CreateFromDirectory($stage,$zip,[IO.Compression.CompressionLevel]::Optimal,$false)
 $sum=(Get-FileHash -LiteralPath $zip -Algorithm SHA256).Hash.ToLowerInvariant()
 [IO.File]::WriteAllText((Join-Path $Output 'SHA256SUMS.txt'),($sum+'  '+[IO.Path]::GetFileName($zip)+"`n"),[Text.UTF8Encoding]::new($false))
 [IO.File]::WriteAllText((Join-Path $Output 'build-receipt.json'),(@{version=$version;archive=$zip;sha256=$sum;stage=$stage}|ConvertTo-Json),[Text.UTF8Encoding]::new($false))
 Write-Output ('Packaged '+$zip+' SHA256 '+$sum)
}finally{Pop-Location}
