& (Join-Path $PSScriptRoot 'install.ps1') -Models 'large-v3'
if ($LASTEXITCODE -ne 0) {
    exit $LASTEXITCODE
}
