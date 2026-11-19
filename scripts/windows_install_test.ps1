# Disposable GitHub-hosted runner only. Never installs a test trust root on a user's PC.
$ErrorActionPreference='Stop'
if ($env:GITHUB_ACTIONS -ne 'true' -or $env:RUNNER_ENVIRONMENT -ne 'github-hosted') {
  throw 'This installation test requires a disposable GitHub-hosted runner.'
}
$identity='OpenDownloadManager.Development'
if (Get-AppxPackage -Name $identity) {throw 'Development package already installed; refusing to replace it.'}
$testRoot=Join-Path $env:RUNNER_TEMP ('odm-install-'+[guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $testRoot | Out-Null
$cert=$null
try {
  $cert=New-SelfSignedCertificate -Type Custom -Subject "CN=$identity" -KeyUsage DigitalSignature -FriendlyName 'ODM disposable CI test only' -CertStoreLocation 'Cert:\CurrentUser\My' -TextExtension @('2.5.29.37={text}1.3.6.1.5.5.7.3.3','2.5.29.19={text}')
  $cerPath=Join-Path $testRoot 'test.cer'
  Export-Certificate -Cert $cert -FilePath $cerPath | Out-Null
  Import-Certificate -FilePath $cerPath -CertStoreLocation 'Cert:\LocalMachine\TrustedPeople' | Out-Null
  $sdkRoot=Join-Path ${env:ProgramFiles(x86)} 'Windows Kits/10/bin'
  $signer=Get-ChildItem -Path (Join-Path $sdkRoot '*/x64/signtool.exe') | Sort-Object FullName -Descending | Select-Object -First 1
  if (!$signer) {throw 'Windows SDK signtool missing'}
  $original=Join-Path $PSScriptRoot '../dist/odm-0.1.0.0-x64-unsigned.msix'
  $signed=Join-Path $testRoot 'odm-test.msix'
  Copy-Item -LiteralPath $original -Destination $signed
  & $signer.FullName sign /fd SHA256 /sha1 $cert.Thumbprint /s My $signed
  if ($LASTEXITCODE) {throw 'Test signing failed'}
  Add-AppxPackage -Path $signed
  $installed=Get-AppxPackage -Name $identity
  if (!$installed -or $installed.Version -ne '0.1.0.0') {throw 'Installation not registered'}
  python (Join-Path $PSScriptRoot 'native_mode_test.py') --exe (Join-Path $installed.InstallLocation 'odm.exe')
  if ($LASTEXITCODE) {throw 'Installed application test failed'}
  & (Join-Path $PSScriptRoot 'package-msix.ps1') -Architecture x64 -IdentityName $identity -Publisher "CN=$identity" -PackageVersion '0.1.0.1' -OutputDirectory $testRoot
  $upgrade=Join-Path $testRoot 'odm-0.1.0.1-x64-unsigned.msix'
  & $signer.FullName sign /fd SHA256 /sha1 $cert.Thumbprint /s My $upgrade
  if ($LASTEXITCODE) {throw 'Upgrade test signing failed'}
  Add-AppxPackage -Path $upgrade
  $installed=Get-AppxPackage -Name $identity
  if ($installed.Version -ne '0.1.0.1') {throw 'Upgrade version mismatch'}
  python (Join-Path $PSScriptRoot 'native_mode_test.py') --exe (Join-Path $installed.InstallLocation 'odm.exe')
  if ($LASTEXITCODE) {throw 'Upgraded application test failed'}
  Remove-AppxPackage -Package $installed.PackageFullName
  if (Get-AppxPackage -Name $identity) {throw 'Uninstall left registration behind'}
  Write-Output 'PASS: Windows x64 MSIX install, launch, version upgrade and uninstall with disposable test trust'
} finally {
  Get-AppxPackage -Name $identity | Remove-AppxPackage
  if ($cert) {
    foreach ($store in @('Cert:\LocalMachine\TrustedPeople','Cert:\CurrentUser\My')) {
      $path=Join-Path $store $cert.Thumbprint
      if (Test-Path $path) {Remove-Item -LiteralPath $path -Force}
    }
  }
}
