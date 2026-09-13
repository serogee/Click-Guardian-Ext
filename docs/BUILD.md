# Click Guardian Build Instructions

## Requirements

Windows builds require:

- Go 1.24 or later.
- A CGO-compatible GCC toolchain.
- GNU `windres` from the same MinGW toolchain.
- PowerShell 5.1 or later.

Release MSI creation also requires WiX Toolset and `go-msi`. Code signing is optional and requires `signtool` and a signing certificate.

## Development Commands

For the fastest edit-and-run loop:

```cmd
scripts\dev.bat
```

This uses `go run`. The program contains its Fyne window and tray icons, but it does not create a distributable Windows executable.

To build local executables:

```cmd
scripts\build.bat
```

The batch file is a wrapper for:

```powershell
.\scripts\build.ps1 -Configuration Development -Architecture amd64
```

It runs the tests and creates:

- `dist\click-guardian.exe` - Windows GUI build without a console window.
- `dist\click-guardian-dev.exe` - Development build with console output.

Both files contain the Windows Explorer icon, manifest, and file metadata. The development build has the product name `Click Guardian Dev` in Windows file properties.

## Release Command

Create local release artifacts with:

```powershell
.\scripts\build.ps1 -Configuration Release -Version 1.0.6 -Architecture amd64
```

The compatibility wrapper reads `VERSION` from `build\build.conf` when `-Version` is omitted:

```cmd
scripts\release-build.bat
```

A complete release creates:

- `dist\click-guardian.exe`
- `dist\click-guardian-v1.0.6-windows-amd64-portable.zip`
- `dist\click-guardian-v1.0.6-windows-amd64-installer.msi`
- `dist\SHA256SUMS.txt`

The release does not include `click-guardian-dev.exe`.

## Build Options

```text
-Configuration Development|Release
-Version <semantic-version>
-Architecture amd64|386
-SkipTests
-SkipInstaller
-SkipSigning
-Clean
-CI
```

Examples:

```powershell
# Rebuild only the two named development artifacts
.\scripts\build.ps1 -Configuration Development -Clean

# Test release packaging without local MSI or signing tools
.\scripts\build.ps1 -Configuration Release -Version 1.0.6 -SkipInstaller -SkipSigning
```

Tagged CI release builds cannot use `-SkipTests`.

## Icons and Windows Resources

The application has three icon paths:

- Fyne embeds the application/window icon from `internal/gui/resources`.
- The tray integration embeds its ICO resource from `internal/gui/resources`.
- `windres` embeds `build/windows/app-icon.ico` into each Windows executable.

The shared build script renders these templates into an isolated directory under `build/temp`:

- `build/windows/app.rc.template`
- `build/windows/app-manifest.xml.template`

It then creates `cmd/click-guardian/click-guardian.syso` temporarily. Go includes that file automatically. The script removes it in a `finally` block, including after a failed build. Generated `.syso` files remain ignored by Git. Builds do not rely on an existing one because it may contain stale metadata. A later build also removes staging directories left by a forcibly terminated process.

## Embedded Build Information

The script passes the following values through Go linker flags:

- `Version`
- `GitCommit`
- `BuildTime` in UTC
- `BuildBy`

Development builds use `dev+<short-commit>`. Release builds use the supplied semantic version.

## Signing

Set these environment variables before a release build:

```powershell
$env:SIGN_CERT_FILE = "C:\path\to\certificate.pfx"
$env:SIGN_CERT_PASSWORD = "certificate password"
$env:SIGN_TIMESTAMP_URL = "https://timestamp.example.com"
```

When `SIGN_CERT_FILE` is absent, the script reports that signing was skipped. When it is present, both the executable and MSI are signed and verified. Final checksums are generated after signing.

## Build Safety

The build process:

- Stops on the first failed external command.
- Does not edit `wix.json` or tracked Windows resource files.
- Uses a temporary installer configuration with a stable upgrade code and deterministic version-specific product code.
- Compiles each requested executable once.
- Deletes only known output names when `-Clean` is used.
- Copies artifacts to `dist` only after their build stage succeeds.

## Continuous Integration

The Windows build workflow calls the same PowerShell entry point:

```powershell
./scripts/build.ps1 -Configuration Development -Architecture amd64 -CI
```

Pull requests and pushes to `main` run tests, build both development artifacts, validate their PE metadata, and upload them as workflow artifacts. This workflow has read-only repository permission and cannot create a release.

## Troubleshooting

Confirm the required commands are available:

```powershell
go version
gcc --version
windres --version
```

If MSI creation tools are not installed, use `-SkipInstaller` for a local release test. Use `scripts\troubleshoot.bat` for CGO setup checks.
