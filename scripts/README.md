# Build Scripts

The Windows build commands use `build.ps1` as their shared implementation.

## Quick Reference

```cmd
scripts\dev.bat
scripts\build.bat
scripts\release-build.bat
scripts\troubleshoot.bat
```

- `dev.bat` runs the application directly with `go run`.
- `build.bat` creates `click-guardian-ext.exe` and `click-guardian-ext-dev.exe` with Windows icons and metadata.
- `release-build.bat` creates versioned release packages through the same PowerShell builder.
- `troubleshoot.bat` diagnoses Go and CGO setup problems.
- `build.sh` is the legacy multi-platform build script; Windows release automation uses `build.ps1`.

Direct PowerShell usage:

```powershell
.\scripts\build.ps1 -Configuration Development
.\scripts\build.ps1 -Configuration Release -Version 1.0.0
```

See [Build Instructions](../docs/BUILD.md) and [Release Build Guide](../docs/RELEASE_BUILD.md).
