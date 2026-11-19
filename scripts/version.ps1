$ErrorActionPreference='Stop'
$versionJson=python (Join-Path $PSScriptRoot 'version.py') --json
if ($LASTEXITCODE) {throw 'Invalid application VERSION'}
$versionJson | ConvertFrom-Json
