# How to get a monthly summary

`quarry summary` summarizes one month: how fresh the data is, the open findings, unusually large charges, recurring charges that started in the month, and net worth at its end beside the month before.

## Run it once

```bash
quarry summary
```

summarizes last month.

```bash
quarry summary --month 2026-09
```

summarizes an earlier month; the month must have ended.

`quarry summary` reads the store and never syncs. Run `quarry sync` first, with the file open in Quicken, so the store covers the whole month.

## Run it every month

A launchd job runs `quarry sync` and then `quarry summary` on the 1st at 9:00 and appends both to a log.

1. Find quarry's full path. launchd does not use your shell's `PATH`.

    ```bash
    command -v quarry
    ```

2. Save this as `~/Library/LaunchAgents/com.github.koblas.quarry.summary.plist`, with both `/Users/you/go/bin/quarry` replaced by that path:

    ```xml
    <?xml version="1.0" encoding="UTF-8"?>
    <!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
    <plist version="1.0">
    <dict>
      <key>Label</key>
      <string>com.github.koblas.quarry.summary</string>
      <key>ProgramArguments</key>
      <array>
        <string>/bin/sh</string>
        <string>-c</string>
        <string>umask 077; mkdir -p "$HOME/Library/Logs/quarry"; { /Users/you/go/bin/quarry sync; /Users/you/go/bin/quarry summary; } >>"$HOME/Library/Logs/quarry/summary.log" 2>&amp;1</string>
      </array>
      <key>StartCalendarInterval</key>
      <dict>
        <key>Day</key><integer>1</integer>
        <key>Hour</key><integer>9</integer>
        <key>Minute</key><integer>0</integer>
      </dict>
    </dict>
    </plist>
    ```

3. Load it:

    ```bash
    launchctl bootstrap gui/$(id -u) ~/Library/LaunchAgents/com.github.koblas.quarry.summary.plist
    ```

4. Try it now, then read `~/Library/Logs/quarry/summary.log`:

    ```bash
    launchctl kickstart gui/$(id -u)/com.github.koblas.quarry.summary
    ```

To stop it:

```bash
launchctl bootout gui/$(id -u)/com.github.koblas.quarry.summary
```

!!! note "Quicken must be open when the job runs"
    `quarry sync` needs Quicken running with your file open. If Quicken was closed, the log shows sync's error line, then a summary that warns its snapshot was taken before the month ended. Open your Quicken file and run `quarry sync`, then `quarry summary`, in a terminal. If the Mac is asleep at 9:00 on the 1st, launchd runs the job when it wakes.

!!! warning "The log holds your data"
    The log holds your payees, amounts and net worth. `umask 077` keeps it readable only by you.
