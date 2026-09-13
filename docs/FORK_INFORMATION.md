# Fork Information

> [!WARNING]
> Click Guardian Ext is a personal-use fork of the original Click Guardian project. It is not intended to be merged into the original project. The additional features described here were built with AI assistance from GPT-5.6 Sol. Review and test the changes before you use them on another system.

## Purpose

This fork is named **Click Guardian Ext** so that users can distinguish it from the original Click Guardian application. It adapts the original project for personal daily use on Windows. Its main addition is an administrator startup mode that lets Click Guardian Ext protect elevated applications after the user signs in.

Click Guardian Ext starts its independent release sequence at version `1.0.0`. That first Ext release is based on Click Guardian `1.0.5`.

The changes are specific to this fork. They have not been prepared or proposed for inclusion in the original Click Guardian project.

## Additional Features

### Windows startup modes

The previous **Start with Windows** checkbox is now a selector with three modes:

- **Disabled:** Does not start Click Guardian Ext when the user signs in.
- **Standard:** Uses the current user's Windows `Run` registry key. Click Guardian Ext starts minimized and enables protection automatically.
- **Administrator:** Uses a Windows scheduled task with the highest available privileges. Click Guardian Ext starts minimized, enables protection automatically, and can protect applications that run as administrator.

The modes are mutually exclusive. Changing the mode removes or replaces the startup configuration from the previous mode.

### Administrator startup task

Administrator mode creates a task named `ClickGuardianExt Admin Startup` for the current Windows user. The task:

- Runs when that user signs in.
- Runs with the highest available privileges.
- Starts the exact Click Guardian Ext executable that created the task.
- Passes `--minimized` so the application opens in the system tray.
- Uses the executable's directory as its working directory.
- Has no execution time limit.
- Does not stop when the computer changes to battery power.
- Ignores a new trigger if Click Guardian Ext is already running.

Windows requests administrator approval when the task is created or removed. Normal startup after sign-in does not show a UAC prompt.

The application refuses to replace a task with the same name when that task does not point to a recognized Click Guardian Ext executable. This check reduces the risk of overwriting an unrelated task.

### Startup task maintenance

When Click Guardian Ext starts from a compatible existing administrator task, it can repair task settings that are missing the minimized argument, unlimited execution time, or single-instance policy.

The elevated install and removal helpers run before the normal single-instance check. This lets an open Click Guardian Ext window configure its startup task through a short-lived elevated helper process.

### Settings responsiveness

Opening Settings reads the locally recorded startup mode and does not wait for Task Scheduler. Task Scheduler checks occur only when the startup selection changes, and they run outside the graphical interface thread.

Task Scheduler commands:

- Run without a Command Prompt window.
- Use a three-second timeout for status queries.
- Use a 15-second timeout for task creation and removal.
- Return detailed errors to the Settings dialog when an elevated helper fails.

### Task XML compatibility

The administrator task definition is written as UTF-16LE with a byte-order mark and a matching `UTF-16` XML declaration. This format prevents the Windows Task Scheduler error `unable to switch the encoding`.

### Development build names

The regular build scripts use the end-user name `click-guardian-ext.exe` for the Windows GUI application. The console-enabled development build is named `click-guardian-ext-dev.exe`, making it clear which executable is intended for debugging.

### Separation from the original application

Click Guardian Ext uses its own executable names, application ID, configuration directory, registry values, scheduled-task name, single-instance mutex, installer identity, installation directory, and shortcuts. The original Click Guardian and Click Guardian Ext can therefore be installed and run independently. Click Guardian Ext does not take ownership of the original application's startup entries or settings.

Existing Click Guardian settings and startup entries are not migrated. Configure Click Guardian Ext separately after its first launch, and disable the original application's startup mode separately if it is no longer needed.

### Reproducible build and release workflow

Development, CI, and release builds now share `scripts/build.ps1`. It generates Windows icon, manifest, and version resources for each executable, validates the compiled PE metadata, and removes generated resources after the build. Development builds create the GUI and console-enabled executables, while releases distribute only the versioned GUI application.

Release packaging no longer rewrites tracked resource, manifest, or installer configuration files. A tagged GitHub workflow runs the same release build used locally, produces a portable ZIP, MSI, and SHA-256 checksum file, and creates a draft GitHub Release for manual inspection before publication.

### Documentation and tests

The README and user guide describe the new startup modes. Automated tests cover:

- Startup mode labels.
- Elevated helper argument handling.
- Administrator task XML contents and UTF-16 encoding.
- UTF-16 task-query parsing.
- Click Guardian Ext executable recognition.
- Hidden Task Scheduler process settings.
- Task command timeouts.

## Daily-Use Notes

Keep the executable in a permanent location before you enable Administrator startup. The scheduled task stores the exact executable path. Moving or renaming the executable after task creation causes startup to fail.

For the existing installer layout, the expected location is:

```text
C:\Program Files (x86)\Click Guardian Ext\click-guardian-ext.exe
```

If the executable must move, use this sequence:

1. Disable startup in Click Guardian Ext.
2. Move the executable.
3. Start it from the new permanent location.
4. Enable the required startup mode again.

When an update replaces the executable at the same path, existing shortcuts and the administrator startup task continue to use that path.

## Security and Support Notice

Administrator mode runs Click Guardian Ext with elevated privileges. Only use an executable that you built from source you reviewed or obtained from a trusted source.

This fork is maintained for personal use. Do not treat it as an official release or request that the original project merge these AI-assisted additions. Users who choose to use the fork are responsible for reviewing, building, testing, and maintaining it for their systems.
