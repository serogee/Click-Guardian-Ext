[CmdletBinding()]
param(
    [ValidateSet("Development", "Release")]
    [string]$Configuration = "Development",
    [string]$Version,
    [ValidateSet("amd64", "386")]
    [string]$Architecture = "amd64",
    [switch]$SkipTests,
    [switch]$SkipInstaller,
    [switch]$SkipSigning,
    [switch]$Clean,
    [switch]$CI
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$projectRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot ".."))
$sourcePackage = Join-Path $projectRoot "cmd\click-guardian"
$generatedSyso = Join-Path $sourcePackage "click-guardian.syso"
$distDirectory = Join-Path $projectRoot "dist"
$tempRoot = Join-Path $projectRoot "build\temp"
$resourceTemplate = Join-Path $projectRoot "build\windows\app.rc.template"
$manifestTemplate = Join-Path $projectRoot "build\windows\app-manifest.xml.template"
$applicationIcon = Join-Path $projectRoot "build\windows\app-icon.ico"
$configPath = Join-Path $projectRoot "build\build.conf"
$workDirectory = Join-Path $tempRoot ("build-" + [Guid]::NewGuid().ToString("N"))

function Write-Step {
    param([string]$Message)
    Write-Host "==> $Message" -ForegroundColor Cyan
}

function Invoke-Tool {
    param(
        [Parameter(Mandatory = $true)][string]$Name,
        [Parameter(Mandatory = $true)][string[]]$Arguments,
        [string]$WorkingDirectory = $projectRoot
    )
    Push-Location -LiteralPath $WorkingDirectory
    try {
        & $Name @Arguments | ForEach-Object { Write-Host $_ }
        $exitCode = $LASTEXITCODE
        if ($exitCode -ne 0) {
            throw "$Name failed with exit code $exitCode."
        }
    }
    finally {
        Pop-Location
    }
}

function Get-BuildConfiguration {
    $values = @{}
    if (Test-Path -LiteralPath $configPath) {
        foreach ($line in Get-Content -LiteralPath $configPath) {
            if ($line -match '^\s*([^#][^=]*)=(.*)$') {
                $values[$Matches[1].Trim()] = $Matches[2].Trim()
            }
        }
    }
    return $values
}

function Get-GitValue {
    param([string[]]$Arguments, [string]$Fallback)
    try {
        $value = @(& git -C $projectRoot @Arguments 2>$null)
        if ($value.Count -gt 0 -and -not [string]::IsNullOrWhiteSpace($value[0])) {
            return $value[0].Trim()
        }
    }
    catch {
        # Build metadata has a documented fallback when Git is unavailable.
    }
    return $Fallback
}

function ConvertTo-RcString {
    param([string]$Value)
    return $Value.Replace('\', '\\').Replace('"', '\"').Replace("`r", "").Replace("`n", " ")
}

function ConvertTo-LinkerValue {
    param([string]$Value)
    return $Value.Replace('"', "'").Replace("`r", "").Replace("`n", " ")
}

function New-DeterministicGuid {
    param([string]$Value)
    $algorithm = [Security.Cryptography.MD5]::Create()
    try {
        [byte[]]$hash = $algorithm.ComputeHash([Text.Encoding]::UTF8.GetBytes($Value))
        return (New-Object -TypeName Guid -ArgumentList (, $hash)).ToString()
    }
    finally {
        $algorithm.Dispose()
    }
}

function New-WindowsResource {
    param(
        [string]$ProductName,
        [string]$FileDescription,
        [string]$OriginalFilename,
        [string]$DisplayVersion,
        [string]$NumericVersion,
        [string]$ManifestVersion,
        [string]$CompanyName,
        [string]$Copyright
    )
    $resourceDirectory = Join-Path $workDirectory "resources"
    if (Test-Path -LiteralPath $resourceDirectory) {
        Remove-Item -LiteralPath $resourceDirectory -Recurse -Force
    }
    New-Item -ItemType Directory -Path $resourceDirectory | Out-Null

    $manifest = (Get-Content -LiteralPath $manifestTemplate -Raw).
        Replace('{{MANIFEST_VERSION}}', $ManifestVersion).
        Replace('{{FILE_DESCRIPTION}}', [Security.SecurityElement]::Escape($FileDescription))
    Set-Content -LiteralPath (Join-Path $resourceDirectory "app-manifest.xml") -Value $manifest -Encoding UTF8

    $resource = (Get-Content -LiteralPath $resourceTemplate -Raw).
        Replace('{{NUMERIC_VERSION}}', $NumericVersion).
        Replace('{{COMPANY_NAME}}', (ConvertTo-RcString $CompanyName)).
        Replace('{{FILE_DESCRIPTION}}', (ConvertTo-RcString $FileDescription)).
        Replace('{{DISPLAY_VERSION}}', (ConvertTo-RcString $DisplayVersion)).
        Replace('{{INTERNAL_NAME}}', (ConvertTo-RcString ([IO.Path]::GetFileNameWithoutExtension($OriginalFilename)))).
        Replace('{{COPYRIGHT}}', (ConvertTo-RcString $Copyright)).
        Replace('{{ORIGINAL_FILENAME}}', (ConvertTo-RcString $OriginalFilename)).
        Replace('{{PRODUCT_NAME}}', (ConvertTo-RcString $ProductName))
    Set-Content -LiteralPath (Join-Path $resourceDirectory "app.rc") -Value $resource -Encoding UTF8
    Copy-Item -LiteralPath $applicationIcon -Destination (Join-Path $resourceDirectory "app-icon.ico")

    if (Test-Path -LiteralPath $generatedSyso) {
        Remove-Item -LiteralPath $generatedSyso -Force
    }
    $windresTarget = if ($Architecture -eq "amd64") { "pe-x86-64" } else { "pe-i386" }
    Write-Step "Embedding the Windows icon and metadata for $OriginalFilename"
    Invoke-Tool -Name "windres" -Arguments @("-F", $windresTarget, "app.rc", "-O", "coff", "-o", $generatedSyso) -WorkingDirectory $resourceDirectory
}

function Get-PeInformation {
    param([string]$Path)
    $stream = [IO.File]::OpenRead($Path)
    $reader = New-Object IO.BinaryReader($stream)
    try {
        $stream.Position = 0x3c
        $peOffset = $reader.ReadInt32()
        $stream.Position = $peOffset
        if ($reader.ReadUInt32() -ne 0x00004550) {
            throw "$Path is not a valid PE executable."
        }
        $machine = $reader.ReadUInt16()
        $stream.Position = $peOffset + 24 + 68
        return [PSCustomObject]@{ Machine = $machine; Subsystem = $reader.ReadUInt16() }
    }
    finally {
        $reader.Dispose()
        $stream.Dispose()
    }
}

function Test-EmbeddedIcon {
    param([string]$Path)
    if (-not ("ClickGuardian.Build.NativeIcon" -as [type])) {
        Add-Type -TypeDefinition @"
using System;
using System.Runtime.InteropServices;
namespace ClickGuardian.Build {
    public static class NativeIcon {
        [DllImport("shell32.dll", CharSet = CharSet.Unicode)]
        public static extern uint ExtractIconEx(string fileName, int iconIndex, IntPtr[] largeIcons, IntPtr[] smallIcons, uint iconCount);
    }
}
"@
    }
    return [ClickGuardian.Build.NativeIcon]::ExtractIconEx($Path, -1, $null, $null, 0) -gt 0
}

function Test-WindowsExecutable {
    param(
        [string]$Path,
        [string]$ExpectedProductName,
        [string]$ExpectedFilename,
        [ValidateSet("GUI", "Console")][string]$ExpectedSubsystem
    )
    if (-not (Test-Path -LiteralPath $Path) -or (Get-Item -LiteralPath $Path).Length -eq 0) {
        throw "Expected build artifact was not created: $Path"
    }
    $versionInfo = (Get-Item -LiteralPath $Path).VersionInfo
    if ($versionInfo.ProductName -ne $ExpectedProductName) {
        throw "ProductName for $ExpectedFilename is '$($versionInfo.ProductName)', expected '$ExpectedProductName'."
    }
    if ($versionInfo.OriginalFilename -ne $ExpectedFilename) {
        throw "OriginalFilename for $ExpectedFilename is '$($versionInfo.OriginalFilename)'."
    }
    if ($versionInfo.FileVersion -ne $script:buildVersion -or $versionInfo.ProductVersion -ne $script:buildVersion) {
        throw "Version metadata in $ExpectedFilename does not match '$script:buildVersion'."
    }
    if (-not (Test-EmbeddedIcon -Path $Path)) {
        throw "$ExpectedFilename does not contain a Windows executable icon."
    }
    $pe = Get-PeInformation -Path $Path
    $expectedMachine = if ($Architecture -eq "amd64") { 0x8664 } else { 0x014c }
    $expectedSubsystemValue = if ($ExpectedSubsystem -eq "GUI") { 2 } else { 3 }
    if ($pe.Machine -ne $expectedMachine) { throw "Unexpected PE architecture in $ExpectedFilename." }
    if ($pe.Subsystem -ne $expectedSubsystemValue) { throw "Unexpected PE subsystem in $ExpectedFilename." }
}

function New-LinkerFlags {
    param([bool]$WindowsGui)
    $parts = @("-s", "-w")
    if ($WindowsGui) { $parts += "-H=windowsgui" }
    $parts += "-X `"click-guardian/internal/version.Version=$(ConvertTo-LinkerValue $script:buildVersion)`""
    $parts += "-X `"click-guardian/internal/version.GitCommit=$(ConvertTo-LinkerValue $script:gitCommit)`""
    $parts += "-X `"click-guardian/internal/version.BuildTime=$(ConvertTo-LinkerValue $script:buildTime)`""
    $parts += "-X `"click-guardian/internal/version.BuildBy=$(ConvertTo-LinkerValue $script:buildBy)`""
    return $parts -join " "
}

function Build-WindowsExecutable {
    param(
        [string]$Filename,
        [string]$ProductName,
        [string]$Description,
        [ValidateSet("GUI", "Console")][string]$Subsystem
    )
    New-WindowsResource -ProductName $ProductName -FileDescription $Description -OriginalFilename $Filename -DisplayVersion $script:buildVersion -NumericVersion $script:numericVersion -ManifestVersion $script:manifestVersion -CompanyName $script:companyName -Copyright $script:copyright
    $outputPath = Join-Path $workDirectory $Filename
    Write-Step "Building $Filename"
    Invoke-Tool -Name "go" -Arguments @("build", "-trimpath", "-ldflags", (New-LinkerFlags -WindowsGui ($Subsystem -eq "GUI")), "-o", $outputPath, ".\cmd\click-guardian")
    Test-WindowsExecutable -Path $outputPath -ExpectedProductName $ProductName -ExpectedFilename $Filename -ExpectedSubsystem $Subsystem
    return $outputPath
}

function Invoke-Signing {
    param([string]$Path)
    if ($SkipSigning) { Write-Host "Signing skipped by request."; return }
    $certificatePath = $env:SIGN_CERT_FILE
    if ([string]::IsNullOrWhiteSpace($certificatePath)) { Write-Host "Signing skipped because SIGN_CERT_FILE is not configured."; return }
    if (-not (Test-Path -LiteralPath $certificatePath)) { throw "Signing certificate not found: $certificatePath" }
    if (-not (Get-Command "signtool" -ErrorAction SilentlyContinue)) { throw "signtool is required when SIGN_CERT_FILE is configured." }

    $arguments = @("sign", "/fd", "SHA256", "/f", $certificatePath)
    if (-not [string]::IsNullOrWhiteSpace($env:SIGN_CERT_PASSWORD)) { $arguments += @("/p", $env:SIGN_CERT_PASSWORD) }
    if (-not [string]::IsNullOrWhiteSpace($env:SIGN_TIMESTAMP_URL)) { $arguments += @("/tr", $env:SIGN_TIMESTAMP_URL, "/td", "SHA256") }
    $arguments += $Path
    Write-Step "Signing $(Split-Path -Leaf $Path)"
    Invoke-Tool -Name "signtool" -Arguments $arguments
    Invoke-Tool -Name "signtool" -Arguments @("verify", "/pa", "/v", $Path)
}

function New-PortablePackage {
    param([string]$ExecutablePath)
    $packageName = "click-guardian-v$Version-windows-$Architecture"
    $packageDirectory = Join-Path $workDirectory $packageName
    New-Item -ItemType Directory -Path $packageDirectory | Out-Null
    Copy-Item -LiteralPath $ExecutablePath -Destination (Join-Path $packageDirectory "click-guardian.exe")
    if (Test-Path -LiteralPath (Join-Path $projectRoot "LICENSE.txt")) {
        Copy-Item -LiteralPath (Join-Path $projectRoot "LICENSE.txt") -Destination (Join-Path $packageDirectory "LICENSE.txt")
    }
    $releaseReadme = @"
Click Guardian v$Version

Prevents accidental double-clicks with configurable delay protection.

Build information:
- Version: $Version
- Built: $buildTime
- Commit: $gitCommit
- Built by: $buildBy

Run click-guardian.exe, configure the delay, and select Start Protection.
See the project documentation for Windows startup and security guidance.
"@
    Set-Content -LiteralPath (Join-Path $packageDirectory "README.txt") -Value $releaseReadme -Encoding UTF8
    $zipPath = Join-Path $workDirectory "$packageName-portable.zip"
    Write-Step "Creating the portable ZIP"
    Compress-Archive -Path (Join-Path $packageDirectory "*") -DestinationPath $zipPath -Force
    return $zipPath
}

function New-RtfLicense {
    param([string]$SourcePath, [string]$DestinationPath)

    $text = Get-Content -LiteralPath $SourcePath -Raw
    $builder = New-Object Text.StringBuilder
    [void]$builder.Append('{\rtf1\ansi\ansicpg1252\deff0{\fonttbl{\f0\fnil Segoe UI;}}\viewkind4\uc1\pard\f0\fs18 ')
    foreach ($character in $text.ToCharArray()) {
        switch ($character) {
            '\' { [void]$builder.Append('\\'); continue }
            '{' { [void]$builder.Append('\{'); continue }
            '}' { [void]$builder.Append('\}'); continue }
            "`r" { continue }
            "`n" { [void]$builder.Append("\par`r`n"); continue }
        }
        $codePoint = [int][char]$character
        if ($codePoint -le 127) {
            [void]$builder.Append($character)
        }
        else {
            if ($codePoint -gt 32767) { $codePoint -= 65536 }
            [void]$builder.Append("\u$codePoint?")
        }
    }
    [void]$builder.Append('}')
    Set-Content -LiteralPath $DestinationPath -Value $builder.ToString() -Encoding ASCII
}

function New-InstallerPackage {
    param([string]$ExecutablePath)
    if ($SkipInstaller) { Write-Host "MSI creation skipped by request."; return $null }
    if (-not (Get-Command "go-msi" -ErrorAction SilentlyContinue)) { throw "go-msi is required for MSI creation. Use -SkipInstaller to build without an MSI." }
    foreach ($tool in @("candle", "light")) {
        if (-not (Get-Command $tool -ErrorAction SilentlyContinue)) {
            throw "$tool from WiX Toolset is required for MSI creation. Use -SkipInstaller to build without an MSI."
        }
    }

    $installerDirectory = Join-Path $workDirectory "installer"
    New-Item -ItemType Directory -Path $installerDirectory | Out-Null
    Copy-Item -LiteralPath $ExecutablePath -Destination (Join-Path $installerDirectory "click-guardian.exe")
    Copy-Item -LiteralPath (Join-Path $projectRoot "assets\icon.ico") -Destination (Join-Path $installerDirectory "icon.ico")
    Copy-Item -LiteralPath (Join-Path $projectRoot "templates") -Destination (Join-Path $installerDirectory "templates") -Recurse
    $licensePath = Join-Path $projectRoot "LICENSE.txt"
    if (Test-Path -LiteralPath $licensePath) {
        Copy-Item -LiteralPath $licensePath -Destination (Join-Path $installerDirectory "LICENSE.txt")
        New-RtfLicense -SourcePath $licensePath -DestinationPath (Join-Path $installerDirectory "LICENSE.rtf")
    }
    $wix = Get-Content -LiteralPath (Join-Path $projectRoot "wix.json") -Raw | ConvertFrom-Json
    $wix.version = $Version
    $productCode = New-DeterministicGuid "$($wix.'upgrade-code'):${Version}:$Architecture"
    $mainTemplatePath = Join-Path $installerDirectory "templates\main.wxs"
    $mainTemplate = Get-Content -LiteralPath $mainTemplatePath -Raw
    if (-not $mainTemplate.Contains('<Product Id="*"')) {
        throw "The MSI template does not contain the expected automatic product code marker."
    }
    $mainTemplate = $mainTemplate.Replace('<Product Id="*"', "<Product Id=`"$productCode`"")
    Set-Content -LiteralPath $mainTemplatePath -Value $mainTemplate -Encoding UTF8
    if (Test-Path -LiteralPath (Join-Path $installerDirectory "LICENSE.rtf")) { $wix.license = "LICENSE.rtf" }
    $wix | ConvertTo-Json -Depth 20 | Set-Content -LiteralPath (Join-Path $installerDirectory "wix.json") -Encoding UTF8

    $msiPath = Join-Path $workDirectory "click-guardian-v$Version-windows-$Architecture-installer.msi"
    Write-Step "Creating the MSI installer"
    Invoke-Tool -Name "go-msi" -Arguments @("make", "--msi", $msiPath, "--version", $Version, "--arch", $Architecture, "--src", "templates") -WorkingDirectory $installerDirectory
    if (-not (Test-Path -LiteralPath $msiPath)) { throw "go-msi did not create the expected installer." }
    return $msiPath
}

function New-ChecksumFile {
    param([string[]]$Paths)
    $checksumPath = Join-Path $workDirectory "SHA256SUMS.txt"
    $lines = foreach ($path in ($Paths | Sort-Object { Split-Path -Leaf $_ })) {
        $hash = Get-FileHash -LiteralPath $path -Algorithm SHA256
        "$($hash.Hash.ToLowerInvariant())  $(Split-Path -Leaf $path)"
    }
    Set-Content -LiteralPath $checksumPath -Value $lines -Encoding ASCII
    return $checksumPath
}

function Remove-BuildDirectory {
    param([string]$Path)

    $fullPath = [IO.Path]::GetFullPath($Path)
    $fullTempRoot = [IO.Path]::GetFullPath($tempRoot).TrimEnd('\') + '\'
    if (-not $fullPath.StartsWith($fullTempRoot, [StringComparison]::OrdinalIgnoreCase)) {
        throw "Refusing to remove a build directory outside build/temp: $fullPath"
    }
    if ((Split-Path -Leaf $fullPath) -notlike "build-*") {
        throw "Refusing to remove an unexpected directory: $fullPath"
    }
    if (Test-Path -LiteralPath $fullPath) {
        Remove-Item -LiteralPath $fullPath -Recurse -Force
    }
}

if ($env:OS -ne "Windows_NT") { throw "scripts/build.ps1 currently supports Windows builds only." }
foreach ($requiredFile in @($resourceTemplate, $manifestTemplate, $applicationIcon, $sourcePackage)) {
    if (-not (Test-Path -LiteralPath $requiredFile)) { throw "Required build input not found: $requiredFile" }
}
foreach ($tool in @("go", "gcc", "windres")) {
    if (-not (Get-Command $tool -ErrorAction SilentlyContinue)) { throw "$tool is required and was not found in PATH." }
}
$gccTarget = (& gcc -dumpmachine 2>$null | Select-Object -First 1)
if ($LASTEXITCODE -ne 0 -or [string]::IsNullOrWhiteSpace($gccTarget)) { throw "Unable to identify the GCC target architecture." }
if ($Architecture -eq "amd64" -and $gccTarget -notmatch 'x86_64|amd64') { throw "GCC target '$gccTarget' cannot build amd64 artifacts." }
if ($Architecture -eq "386" -and $gccTarget -notmatch 'i[3-6]86') { throw "GCC target '$gccTarget' cannot build 386 artifacts." }
if ($Configuration -eq "Release" -and $CI -and $SkipTests) { throw "CI release builds cannot skip tests." }

$buildConfig = Get-BuildConfiguration
if ($Configuration -eq "Release") {
    if ([string]::IsNullOrWhiteSpace($Version)) {
        if (-not $buildConfig.ContainsKey("VERSION")) { throw "A release version is required." }
        $Version = $buildConfig["VERSION"]
    }
    if ($Version -notmatch '^(\d+)\.(\d+)\.(\d+)$') { throw "Release version '$Version' must have the form X.Y.Z." }
    $versionParts = @($Matches[1], $Matches[2], $Matches[3])
    $numericVersion = "$($versionParts[0]),$($versionParts[1]),$($versionParts[2]),0"
    $manifestVersion = "$($versionParts[0]).$($versionParts[1]).$($versionParts[2]).0"
}
else {
    $numericVersion = "0,0,0,0"
    $manifestVersion = "0.0.0.0"
}

$commitFallback = if ([string]::IsNullOrWhiteSpace($env:GITHUB_SHA)) { "unknown" } else { $env:GITHUB_SHA.Substring(0, [Math]::Min(7, $env:GITHUB_SHA.Length)) }
$gitCommit = Get-GitValue -Arguments @("rev-parse", "--short", "HEAD") -Fallback $commitFallback
$buildTime = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")
$buildBy = $env:GITHUB_ACTOR
if ([string]::IsNullOrWhiteSpace($buildBy)) { $buildBy = Get-GitValue -Arguments @("config", "user.name") -Fallback $env:USERNAME }
if ([string]::IsNullOrWhiteSpace($buildBy)) { $buildBy = "unknown" }
$buildVersion = if ($Configuration -eq "Release") { $Version } else { "dev+$gitCommit" }
$companyName = if ($buildConfig.ContainsKey("COMPANY_NAME")) { $buildConfig["COMPANY_NAME"] } else { "Click Guardian Project" }
$copyright = if ($buildConfig.ContainsKey("COPYRIGHT")) { $buildConfig["COPYRIGHT"] } else { "Copyright $(Get-Date -Format yyyy) Click Guardian Project" }

if ($Configuration -eq "Release" -and -not $SkipInstaller) {
    foreach ($tool in @("go-msi", "candle", "light")) {
        if (-not (Get-Command $tool -ErrorAction SilentlyContinue)) {
            throw "$tool is required for MSI creation. Use -SkipInstaller to build without an MSI."
        }
    }
    foreach ($installerInput in @((Join-Path $projectRoot "wix.json"), (Join-Path $projectRoot "templates\main.wxs"), (Join-Path $projectRoot "assets\icon.ico"))) {
        if (-not (Test-Path -LiteralPath $installerInput)) { throw "Required installer input not found: $installerInput" }
    }
}
if ($Configuration -eq "Release" -and -not $SkipSigning -and -not [string]::IsNullOrWhiteSpace($env:SIGN_CERT_FILE)) {
    if (-not (Test-Path -LiteralPath $env:SIGN_CERT_FILE)) { throw "Signing certificate not found: $($env:SIGN_CERT_FILE)" }
    if (-not (Get-Command "signtool" -ErrorAction SilentlyContinue)) { throw "signtool is required when SIGN_CERT_FILE is configured." }
}

$previousGoos = $env:GOOS
$previousGoarch = $env:GOARCH
$previousCgo = $env:CGO_ENABLED
$buildMutex = New-Object -TypeName Threading.Mutex -ArgumentList @($false, "Local\ClickGuardianBuild")
$mutexAcquired = $buildMutex.WaitOne(0)
if (-not $mutexAcquired) {
    $buildMutex.Dispose()
    throw "Another Click Guardian build is already running."
}
$env:GOOS = "windows"
$env:GOARCH = $Architecture
$env:CGO_ENABLED = "1"

try {
    New-Item -ItemType Directory -Path $distDirectory -Force | Out-Null
    New-Item -ItemType Directory -Path $tempRoot -Force | Out-Null
    foreach ($staleDirectory in Get-ChildItem -LiteralPath $tempRoot -Directory -Filter "build-*") {
        Remove-BuildDirectory -Path $staleDirectory.FullName
    }
    New-Item -ItemType Directory -Path $workDirectory -Force | Out-Null
    if (Test-Path -LiteralPath $generatedSyso) { Write-Host "Removing stale generated resource: $generatedSyso"; Remove-Item -LiteralPath $generatedSyso -Force }

    if ($Clean) {
        $cleanTargets = @((Join-Path $distDirectory "click-guardian.exe"), (Join-Path $distDirectory "click-guardian-dev.exe"))
        if ($Configuration -eq "Release") {
            $cleanTargets += @((Join-Path $distDirectory "click-guardian-v$Version-windows-$Architecture-portable.zip"), (Join-Path $distDirectory "click-guardian-v$Version-windows-$Architecture-installer.msi"), (Join-Path $distDirectory "SHA256SUMS.txt"))
        }
        foreach ($target in $cleanTargets) { if (Test-Path -LiteralPath $target) { Remove-Item -LiteralPath $target -Force } }
    }

    Write-Host "Configuration: $Configuration"
    Write-Host "Version:       $buildVersion"
    Write-Host "Commit:        $gitCommit"
    Write-Host "Architecture:  $Architecture"
    Write-Host "Built by:      $buildBy"

    if (-not $SkipTests) { Write-Step "Running tests"; Invoke-Tool -Name "go" -Arguments @("test", "./...") }
    else { Write-Host "Tests skipped by request." }

    $guiExecutable = Build-WindowsExecutable -Filename "click-guardian.exe" -ProductName "Click Guardian" -Description "Click Guardian - Double-Click Protection" -Subsystem "GUI"
    if ($Configuration -eq "Development") {
        $developmentExecutable = Build-WindowsExecutable -Filename "click-guardian-dev.exe" -ProductName "Click Guardian Dev" -Description "Click Guardian - Development Build" -Subsystem "Console"
        Copy-Item -LiteralPath $guiExecutable -Destination (Join-Path $distDirectory "click-guardian.exe") -Force
        Copy-Item -LiteralPath $developmentExecutable -Destination (Join-Path $distDirectory "click-guardian-dev.exe") -Force
        Write-Host "Development artifacts:" -ForegroundColor Green
        Write-Host "  dist\click-guardian.exe"
        Write-Host "  dist\click-guardian-dev.exe"
    }
    else {
        Invoke-Signing -Path $guiExecutable
        $zipPath = New-PortablePackage -ExecutablePath $guiExecutable
        $msiPath = New-InstallerPackage -ExecutablePath $guiExecutable
        if ($null -ne $msiPath) { Invoke-Signing -Path $msiPath }
        $releaseFiles = @($zipPath)
        if ($null -ne $msiPath) { $releaseFiles += $msiPath }
        $checksumPath = New-ChecksumFile -Paths $releaseFiles

        Copy-Item -LiteralPath $guiExecutable -Destination (Join-Path $distDirectory "click-guardian.exe") -Force
        foreach ($releaseFile in ($releaseFiles + $checksumPath)) {
            Copy-Item -LiteralPath $releaseFile -Destination (Join-Path $distDirectory (Split-Path -Leaf $releaseFile)) -Force
        }
        Write-Host "Release artifacts:" -ForegroundColor Green
        Write-Host "  dist\click-guardian.exe"
        foreach ($releaseFile in ($releaseFiles + $checksumPath)) { Write-Host "  dist\$(Split-Path -Leaf $releaseFile)" }
    }
}
finally {
    $env:GOOS = $previousGoos
    $env:GOARCH = $previousGoarch
    $env:CGO_ENABLED = $previousCgo
    try {
        if (Test-Path -LiteralPath $generatedSyso) { Remove-Item -LiteralPath $generatedSyso -Force }
        Remove-BuildDirectory -Path $workDirectory
    }
    finally {
        if ($mutexAcquired) { $buildMutex.ReleaseMutex() }
        $buildMutex.Dispose()
    }
}
