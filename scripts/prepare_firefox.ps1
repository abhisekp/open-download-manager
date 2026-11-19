# Extract verified test tools; does not install/register a personal browser.
param([Parameter(Mandatory=$true)][string]$Destination)
$ErrorActionPreference='Stop'
New-Item -ItemType Directory -Path $Destination -Force | Out-Null
function Get-VerifiedTool([string]$Url,[string]$Name,[string]$Algorithm,[string]$Expected) {
  $file=Join-Path $Destination $Name
  if (!(Test-Path -LiteralPath $file)) {Invoke-WebRequest -Uri $Url -OutFile $file}
  if ((Get-FileHash -LiteralPath $file -Algorithm $Algorithm).Hash.ToLowerInvariant() -ne $Expected) {
    throw "Test tool checksum mismatch: $Name"
  }
  return $file
}
$firefox=Get-VerifiedTool 'https://archive.mozilla.org/pub/firefox/releases/160.0.1/win64/en-US/Firefox%20Setup%20160.0.1.exe' 'firefox-160.0.1.exe' 'SHA512' 'e0356669960e73528d53c194ae2e3f46d663483073da16d58ba318cd67036d4023a10a23f47e1b733554dce88d8b7c299d806e06bbe31c590286ded20886eaea'
$signature=Get-AuthenticodeSignature -LiteralPath $firefox
if ($signature.Status -ne 'Valid' -or $signature.SignerCertificate.Subject -notmatch '(^|, )CN=Mozilla Corporation(,|$)') {throw 'Firefox installer publisher signature is not verified'}
$sevenZip=Get-Command 7z -ErrorAction SilentlyContinue
if ($sevenZip) {$extractor=$sevenZip.Source} else {$extractor='C:/Program Files/7-Zip/7z.exe'}
if (!(Test-Path -LiteralPath $extractor)) {throw '7-Zip is required to extract the verified Firefox test tool'}
& $extractor x $firefox ('-o'+(Join-Path $Destination 'firefox')) -y | Out-Null
if ($LASTEXITCODE) {throw 'Firefox test tool extraction failed'}
$driver=Get-VerifiedTool 'https://github.com/mozilla/geckodriver/releases/download/v0.37.1/geckodriver-v0.37.1-win64.zip' 'geckodriver.zip' 'SHA256' 'dfed9315abe8d2fbc1b6161a2ee8002452e79cf05ee92fdc653a4e26bc35edd8'
Add-Type -AssemblyName System.IO.Compression.FileSystem
$archive=[System.IO.Compression.ZipFile]::OpenRead($driver)
try {
  if ($archive.Entries.Count -ne 1 -or $archive.Entries[0].FullName -ne 'geckodriver.exe') {throw 'Unexpected driver archive contents'}
  $driverDir=Join-Path $Destination 'driver'
  New-Item -ItemType Directory -Path $driverDir -Force | Out-Null
  [System.IO.Compression.ZipFileExtensions]::ExtractToFile($archive.Entries[0],(Join-Path $driverDir 'geckodriver.exe'),$true)
} finally {$archive.Dispose()}
Write-Output 'Verified/extracted Firefox 160.0.1 and geckodriver 0.37.1; no browser installation or publisher login.'
