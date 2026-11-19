param(
  [ValidateSet('x86','x64','arm64')][string]$Architecture='x64',
  [Parameter(Mandatory=$true)][ValidatePattern('^[A-Za-z0-9][A-Za-z0-9.-]{2,49}$')][string]$IdentityName,
  [Parameter(Mandatory=$true)][string]$Publisher,
  [ValidatePattern('^\d+\.\d+\.\d+\.\d+$')][string]$PackageVersion='0.1.0.0'
)
$ErrorActionPreference='Stop'
$projectRoot=Split-Path $PSScriptRoot -Parent
$cpu=@{x86='386';x64='amd64';arm64='arm64'}[$Architecture]
$binary=Join-Path $projectRoot "dist/odm-0.1.0-dev-windows-$cpu/odm.exe"
if (!(Test-Path -LiteralPath $binary)) {throw 'Build the matching portable artifact first.'}
$stage=Join-Path $projectRoot ('dist/msix-stage-'+[guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path (Join-Path $stage 'Assets') -Force | Out-Null
Copy-Item -LiteralPath $binary -Destination (Join-Path $stage 'odm.exe')
foreach($name in @('LICENSE','THIRD-PARTY-NOTICES.txt','THIRD-PARTY-GO-NOTICES.txt')) {Copy-Item -LiteralPath (Join-Path $projectRoot $name) -Destination $stage}
Get-ChildItem -LiteralPath (Join-Path $projectRoot 'assets/windows') -Filter '*.png' | Copy-Item -Destination (Join-Path $stage 'Assets')
$identityXml=[System.Security.SecurityElement]::Escape($IdentityName)
$publisherXml=[System.Security.SecurityElement]::Escape($Publisher)
$manifest=@"
<?xml version="1.0" encoding="utf-8"?>
<Package xmlns="http://schemas.microsoft.com/appx/manifest/foundation/windows10" xmlns:uap="http://schemas.microsoft.com/appx/manifest/uap/windows10" xmlns:rescap="http://schemas.microsoft.com/appx/manifest/foundation/windows10/restrictedcapabilities" IgnorableNamespaces="uap rescap">
 <Identity Name="$identityXml" Publisher="$publisherXml" Version="$PackageVersion" ProcessorArchitecture="$Architecture" />
 <Properties><DisplayName>Open Download Manager</DisplayName><PublisherDisplayName>Open Download Manager contributors</PublisherDisplayName><Logo>Assets\StoreLogo.png</Logo></Properties>
 <Dependencies><TargetDeviceFamily Name="Windows.Desktop" MinVersion="10.0.19041.0" MaxVersionTested="10.0.26100.0" /></Dependencies>
 <Resources><Resource Language="en-us" /></Resources>
 <Applications><Application Id="ODM" Executable="odm.exe" EntryPoint="Windows.FullTrustApplication"><uap:VisualElements DisplayName="Open Download Manager" Description="Independent open-source download manager" BackgroundColor="transparent" Square150x150Logo="Assets\Square150x150Logo.png" Square44x44Logo="Assets\Square44x44Logo.png" /></Application></Applications>
 <Capabilities><rescap:Capability Name="runFullTrust" /><Capability Name="internetClientServer" /></Capabilities>
</Package>
"@
[System.IO.File]::WriteAllText((Join-Path $stage 'AppxManifest.xml'),$manifest,[System.Text.UTF8Encoding]::new($false))
$sdkRoot=Join-Path ${env:ProgramFiles(x86)} 'Windows Kits/10/bin'
$makeappx=Get-ChildItem -Path (Join-Path $sdkRoot '*/x64/makeappx.exe') | Sort-Object FullName -Descending | Select-Object -First 1
if (!$makeappx) {throw 'Windows SDK makeappx.exe is required.'}
$output=Join-Path $projectRoot "dist/odm-$PackageVersion-$Architecture-unsigned.msix"
& $makeappx.FullName pack /d $stage /p $output /o
if ($LASTEXITCODE) {throw 'MSIX validation/packaging failed.'}
Write-Output "Created unsigned package: $output"
Write-Output 'This is not a Store submission or a clean-install test. aria2 is not bundled; native torrents work independently.'
