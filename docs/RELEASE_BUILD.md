# Release Build Guide

## Local Release Candidate

Build the same release artifacts that GitHub Actions produces:

```powershell
.\scripts\build.ps1 -Configuration Release -Version 1.0.6 -Architecture amd64 -Clean
```

The older command remains available as a wrapper and reads the default version from `build\build.conf`:

```cmd
scripts\release-build.bat
```

The shared builder runs tests, generates clean Windows resources, builds the GUI executable once, optionally signs it, creates the portable ZIP and MSI, optionally signs the MSI, and writes final SHA-256 checksums.

## Release Artifacts

For version `1.0.6`, the release output is:

```text
dist/
|-- click-guardian.exe
|-- click-guardian-v1.0.6-windows-amd64-portable.zip
|-- click-guardian-v1.0.6-windows-amd64-installer.msi
`-- SHA256SUMS.txt
```

The development console executable is not distributed in a release.

## Version and Resource Data

The release version must use semantic version form such as `1.0.6`. The script uses its numeric portion for Windows version resources and passes these values to the application through linker flags:

- Release version
- Git commit
- UTC build time
- Builder name

Resource inputs are generated from `build/windows/app.rc.template` and `build/windows/app-manifest.xml.template`. The source templates and `wix.json` are never rewritten during a build.

## Local Tooling

In addition to Go, GCC, and `windres`, complete release packaging requires:

- WiX Toolset
- `go-msi`
- `signtool` when signing is configured

To test the release executable and portable ZIP without an MSI:

```powershell
.\scripts\build.ps1 -Configuration Release -Version 1.0.6 -SkipInstaller -SkipSigning
```

## Signing Configuration

Provide signing settings through environment variables rather than committed configuration:

```text
SIGN_CERT_FILE
SIGN_CERT_PASSWORD
SIGN_TIMESTAMP_URL
```

The GitHub workflow expects these repository settings:

- Secret `WINDOWS_SIGNING_CERT_BASE64`
- Secret `WINDOWS_SIGNING_CERT_PASSWORD`
- Variable `WINDOWS_SIGNING_TIMESTAMP_URL`

Signing is optional. If the certificate secret is not configured, the workflow creates unsigned artifacts and the log states that signing was skipped.

## GitHub Release Flow

1. Merge the release-ready changes into `main`.
2. Confirm the Windows build workflow passes.
3. Create and push an annotated version tag:

   ```cmd
   git tag -a v1.0.6 -m "Release version 1.0.6"
   git push origin v1.0.6
   ```

4. `.github/workflows/release.yml` checks out the existing tag and verifies that its commit is part of `main` history.
5. The workflow installs the required compiler and MSI tools.
6. It calls `build.ps1 -Configuration Release` with the version derived from the tag.
7. It uploads the executable and packages as workflow artifacts.
8. It creates a draft GitHub Release from the verified tag and attaches the ZIP, MSI, and checksum file.
9. Inspect the draft and publish it manually.

The workflow can also be started manually with `workflow_dispatch`, but it still requires an existing valid version tag.

## Why Releases Are Drafted

Draft creation keeps the final publication decision manual while preserving an automated and repeatable build. It gives the maintainer a chance to inspect file names, signatures, checksums, release notes, and installation behavior before publication.

## Failure Behavior

The release stops when tests, resource generation, compilation, metadata validation, signing, or packaging fails. Temporary `.syso` and staging files are removed even after failure. A failed release job does not publish a GitHub Release.
