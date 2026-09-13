# Click Guardian Ext MSI Installer Guide

The shared release builder creates the Windows MSI from a staged copy of the executable, icon, templates, license, and installer configuration.

## Requirements

- Windows
- Go
- WiX Toolset with `candle.exe` and `light.exe` available in `PATH`
- `go-msi`

Install the repository-pinned `go-msi` tool and its locked dependencies with:

```powershell
Push-Location build\tools
go install -mod=readonly tool
Pop-Location
```

## Build the Installer

Run a complete release build:

```powershell
.\scripts\build.ps1 -Configuration Release -Version 1.0.0
```

Or use the compatibility wrapper, which reads the default version from `build\build.conf`:

```cmd
scripts\release-build.bat
```

The versioned installer is created as:

```text
dist\click-guardian-ext-v1.0.0-windows-amd64-installer.msi
```

## Staged Configuration

The build script reads the tracked `wix.json` as a base and writes a modified copy under `build/temp`. It does not modify the tracked file.

For each version, it:

- Keeps the stable `upgrade-code` so Windows recognizes later versions as upgrades.
- Derives a deterministic version-specific `product-code` and inserts it into the staged WiX template.
- Sets the release version.
- Stages `click-guardian-ext.exe`, `assets/icon.ico`, the license, and `templates/` beside the generated configuration.
- Converts `LICENSE.txt` to the RTF form required by the WiX license dialog.
- Runs `go-msi` entirely inside that staging directory.
- Removes the staging directory after the build.

The installer contains only the release GUI executable. It does not contain `click-guardian-ext-dev.exe`.

## Signing

When `SIGN_CERT_FILE` is configured, the release builder signs and verifies `click-guardian-ext.exe` before it creates the installer. It then signs and verifies the MSI. Checksums are calculated after signing.

See [Release Build Guide](RELEASE_BUILD.md) for local and GitHub signing configuration.

## Troubleshooting

### WiX tools not found

Confirm that `candle.exe` and `light.exe` are available in a new terminal after WiX installation.

### go-msi not found

Confirm that the Go binary directory is in `PATH`:

```powershell
go env GOPATH
go-msi --version
```

### Build without an installer

Use this when testing the executable and portable package on a machine without WiX or `go-msi`:

```powershell
.\scripts\build.ps1 -Configuration Release -Version 1.0.0 -SkipInstaller
```

### Icon or source file not found

Confirm these tracked inputs exist:

- `assets/icon.ico`
- `templates/main.wxs`
- `wix.json`
- `LICENSE.txt`

The builder stops before publishing partial release packages when a required installer input is missing.
