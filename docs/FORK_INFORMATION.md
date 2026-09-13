# Fork Information

> [!WARNING]
> This repository is a personal-use fork of Click Guardian. It is not intended to be merged into the original project. The additional features described here were built with AI assistance from GPT-5.6 Sol. Review and test the changes before you use them on another system.

## Purpose

This fork adapts Click Guardian for personal daily use on Windows. Its main addition is an administrator startup mode that lets Click Guardian protect elevated applications after the user signs in.

The changes are specific to this fork. They have not been prepared or proposed for inclusion in the original Click Guardian project.

## Additional Features

### Windows startup modes

The previous **Start with Windows** checkbox is now a selector with three modes:

- **Disabled:** Does not start Click Guardian when the user signs in.
- **Standard:** Uses the current user's Windows `Run` registry key. Click Guardian starts minimized and enables protection automatically.
- **Administrator:** Uses a Windows scheduled task with the highest available privileges. Click Guardian starts minimized, enables protection automatically, and can protect applications that run as administrator.

The modes are mutually exclusive. Changing the mode removes or replaces the startup configuration from the previous mode.

### Administrator startup task

Administrator mode creates a task named `ClickGuardian Admin Startup` for the current Windows user. The task:

- Runs when that user signs in.
- Runs with the highest available privileges.
- Starts the exact Click Guardian executable that created the task.
- Passes `--minimized` so the application opens in the system tray.
- Uses the executable's directory as its working directory.
- Has no execution time limit.
- Does not stop when the computer changes to battery power.
- Ignores a new trigger if Click Guardian is already running.

Windows requests administrator approval when the task is created or removed. Normal startup after sign-in does not show a UAC prompt.

The application refuses to replace a task with the same name when that task does not point to a recognized Click Guardian executable. This check reduces the risk of overwriting an unrelated task.

### Startup task maintenance

When Click Guardian starts from a compatible existing administrator task, it can repair task settings that are missing the minimized argument, unlimited execution time, or single-instance policy.

The elevated install and removal helpers run before the normal single-instance check. This lets an open Click Guardian window configure its startup task through a short-lived elevated helper process.

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

The regular build scripts use the end-user name `click-guardian.exe` for the Windows GUI application. The console-enabled development build is named `click-guardian-dev.exe`, making it clear which executable is intended for debugging. This replaces the previous local-build names, where the GUI executable was `click-guardian-gui.exe` and the console executable was `click-guardian.exe`.

Existing administrator startup tasks that point to the former `click-guardian-gui.exe` name remain recognized so they can be repaired, replaced, or removed safely.

### Documentation and tests

The README and user guide describe the new startup modes. Automated tests cover:

- Startup mode labels.
- Elevated helper argument handling.
- Administrator task XML contents and UTF-16 encoding.
- UTF-16 task-query parsing.
- Click Guardian executable recognition.
- Hidden Task Scheduler process settings.
- Task command timeouts.

## Daily-Use Notes

Keep the executable in a permanent location before you enable Administrator startup. The scheduled task stores the exact executable path. Moving or renaming the executable after task creation causes startup to fail.

For the existing installer layout, the expected location is:

```text
C:\Program Files (x86)\Click Guardian\click-guardian.exe
```

If the executable must move, use this sequence:

1. Disable startup in Click Guardian.
2. Move the executable.
3. Start it from the new permanent location.
4. Enable the required startup mode again.

When an update replaces the executable at the same path, existing shortcuts and the administrator startup task continue to use that path.

## Security and Support Notice

Administrator mode runs Click Guardian with elevated privileges. Only use an executable that you built from source you reviewed or obtained from a trusted source.

This fork is maintained for personal use. Do not treat it as an official release or request that the original project merge these AI-assisted additions. Users who choose to use the fork are responsible for reviewing, building, testing, and maintaining it for their systems.
