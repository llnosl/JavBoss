[CmdletBinding()]
param(
    [ValidateSet('medium', 'large-v3', 'both')]
    [string]$Models = 'medium',

    [string]$RuntimeUrl = 'https://github.com/Purfview/whisper-standalone-win/releases/download/Faster-Whisper-XXL/Faster-Whisper-XXL_r245.4_windows.7z'
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$toolRoot = $PSScriptRoot
$runtimeRoot = Join-Path $toolRoot 'Faster-Whisper-XXL'
$cacheBase = if ($env:LOCALAPPDATA) {
    Join-Path $env:LOCALAPPDATA 'JavBoss\downloads'
} else {
    Join-Path ([System.IO.Path]::GetTempPath()) 'JavBoss\downloads'
}
$runtimeArchive = Join-Path $cacheBase 'Faster-Whisper-XXL_r245.4_windows.7z'

function Invoke-FileDownload {
    param(
        [Parameter(Mandatory = $true)][string]$Uri,
        [Parameter(Mandatory = $true)][string]$Destination
    )

    $destinationDirectory = Split-Path -Parent $Destination
    New-Item -ItemType Directory -Force -Path $destinationDirectory | Out-Null

    if ((Test-Path -LiteralPath $Destination) -and (Get-Item -LiteralPath $Destination).Length -gt 0) {
        Write-Host "[skip] $Destination"
        return
    }

    $partial = "$Destination.part"
    $curl = Get-Command 'curl.exe' -ErrorAction SilentlyContinue
    if ($null -ne $curl) {
        & $curl.Source --fail --location --retry 5 --retry-delay 2 --retry-all-errors --continue-at - --output $partial $Uri
        if ($LASTEXITCODE -ne 0) {
            throw "curl download failed with exit code $LASTEXITCODE`: $Uri"
        }
    } else {
        Remove-Item -LiteralPath $partial -Force -ErrorAction SilentlyContinue
        Invoke-WebRequest -Uri $Uri -OutFile $partial -UseBasicParsing
    }

    if (-not (Test-Path -LiteralPath $partial) -or (Get-Item -LiteralPath $partial).Length -eq 0) {
        throw "Downloaded file is empty: $Uri"
    }

    Move-Item -LiteralPath $partial -Destination $Destination -Force
}

function Expand-RuntimeArchive {
    param(
        [Parameter(Mandatory = $true)][string]$Archive,
        [Parameter(Mandatory = $true)][string]$Destination
    )

    New-Item -ItemType Directory -Force -Path $Destination | Out-Null
    $sevenZip = Get-Command '7z.exe' -ErrorAction SilentlyContinue
    if ($null -eq $sevenZip) {
        $sevenZip = Get-Command '7zz.exe' -ErrorAction SilentlyContinue
    }

    if ($null -ne $sevenZip) {
        & $sevenZip.Source x -y "-o$Destination" $Archive
    } else {
        $tar = Get-Command 'tar.exe' -ErrorAction SilentlyContinue
        if ($null -eq $tar) {
            throw '7z.exe, 7zz.exe, or tar.exe is required to extract the Faster-Whisper-XXL archive.'
        }
        & $tar.Source -xf $Archive -C $Destination
    }

    if ($LASTEXITCODE -ne 0) {
        throw "Archive extraction failed with exit code $LASTEXITCODE"
    }
}

function Install-Runtime {
    if (Test-Path -LiteralPath (Join-Path $runtimeRoot 'faster-whisper-xxl.exe')) {
        Write-Host "[skip] $runtimeRoot\faster-whisper-xxl.exe"
        return
    }

    Write-Host '[download] Faster-Whisper-XXL runtime'
    Invoke-FileDownload -Uri $RuntimeUrl -Destination $runtimeArchive

    $stageName = 'javboss-whisper-' + [Guid]::NewGuid().ToString('N')
    $stage = Join-Path ([System.IO.Path]::GetTempPath()) $stageName
    $stageFullPath = [System.IO.Path]::GetFullPath($stage)
    $tempFullPath = [System.IO.Path]::GetFullPath([System.IO.Path]::GetTempPath())
    try {
        Expand-RuntimeArchive -Archive $runtimeArchive -Destination $stageFullPath
        $executable = Get-ChildItem -LiteralPath $stageFullPath -Filter 'faster-whisper-xxl.exe' -File -Recurse | Select-Object -First 1
        if ($null -eq $executable) {
            throw 'faster-whisper-xxl.exe was not found in the downloaded archive.'
        }

        New-Item -ItemType Directory -Force -Path $runtimeRoot | Out-Null
        Get-ChildItem -LiteralPath $executable.Directory.FullName -Force | Copy-Item -Destination $runtimeRoot -Recurse -Force
    } finally {
        if ($stageFullPath.StartsWith($tempFullPath, [System.StringComparison]::OrdinalIgnoreCase) -and
            (Split-Path -Leaf $stageFullPath).StartsWith('javboss-whisper-', [System.StringComparison]::OrdinalIgnoreCase) -and
            (Test-Path -LiteralPath $stageFullPath)) {
            Remove-Item -LiteralPath $stageFullPath -Recurse -Force
        }
    }
}

$modelCatalog = @{
    'medium' = @{
        Repository = 'Systran/faster-whisper-medium'
        Revision = '4d9f76bb96174a5625e9ed85e89be563d98d528c'
        Directory = 'faster-whisper-medium'
        Files = @('config.json', 'model.bin', 'tokenizer.json', 'vocabulary.txt')
    }
    'large-v3' = @{
        Repository = 'Systran/faster-whisper-large-v3'
        Revision = '53ecf83a5bedc5597eb8c8b34eac29e5345520ff'
        Directory = 'faster-whisper-large-v3'
        Files = @('config.json', 'model.bin', 'preprocessor_config.json', 'tokenizer.json', 'vocabulary.json')
    }
}

function Install-Model {
    param([Parameter(Mandatory = $true)][string]$Name)

    $spec = $modelCatalog[$Name]
    $modelDirectory = Join-Path (Join-Path $runtimeRoot '_models') $spec.Directory
    New-Item -ItemType Directory -Force -Path $modelDirectory | Out-Null

    foreach ($file in $spec.Files) {
        $uri = "https://huggingface.co/$($spec.Repository)/resolve/$($spec.Revision)/$file`?download=true"
        $destination = Join-Path $modelDirectory $file
        Write-Host "[download] $Name/$file"
        Invoke-FileDownload -Uri $uri -Destination $destination
    }

    $modelBinary = Join-Path $modelDirectory 'model.bin'
    if (-not (Test-Path -LiteralPath $modelBinary) -or (Get-Item -LiteralPath $modelBinary).Length -eq 0) {
        throw "Model installation failed: $modelBinary"
    }
}

Install-Runtime

$selectedModels = if ($Models -eq 'both') { @('medium', 'large-v3') } else { @($Models) }
foreach ($modelName in $selectedModels) {
    Install-Model -Name $modelName
}

$runtimeExecutable = Join-Path $runtimeRoot 'faster-whisper-xxl.exe'
if (-not (Test-Path -LiteralPath $runtimeExecutable)) {
    throw "Runtime installation failed: $runtimeExecutable"
}

Write-Host "[done] runtime: $runtimeExecutable"
foreach ($modelName in $selectedModels) {
    Write-Host "[done] model: $modelName"
}
