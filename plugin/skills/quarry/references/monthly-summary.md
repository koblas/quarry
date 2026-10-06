# Monthly summary job

Use this when the user wants last month's summary every month without asking: a launchd job that runs `quarry sync`, then `quarry summary`, on the 1st and writes both to a log only they can read. Show the user these steps; write or load the LaunchAgent only when they ask you to.

## Reading the document

Run `quarry summary --json` and read `snapshot` (`covers_month` false or null: the store may lack the end of the month; say so), `findings`, `anomalies`, `recurring`, `net_worth` (with `changes`) and `warnings`; relay the warnings. `month`, `since`, `until` and `currency` say which month the numbers cover and in what currency, and `dates` gives the first and last transaction dates in the store.

## Run quarry summary every month

1. Find quarry's full path with `command -v quarry` (for example /Users/you/go/bin/quarry).
   launchd does not use your shell's PATH.
2. Save this as ~/Library/LaunchAgents/com.github.koblas.quarry.summary.plist, with both
   /Users/you/go/bin/quarry replaced by that path:

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

3. Load it:      launchctl bootstrap gui/$(id -u) ~/Library/LaunchAgents/com.github.koblas.quarry.summary.plist
4. Try it now:   launchctl kickstart gui/$(id -u)/com.github.koblas.quarry.summary
   then read ~/Library/Logs/quarry/summary.log.
5. To stop it:   launchctl bootout gui/$(id -u)/com.github.koblas.quarry.summary

quarry sync needs Quicken running with your file open. If Quicken was closed when the job ran,
the log shows sync's error line, then a summary that warns its snapshot was taken before the
month ended; open your Quicken file and run `quarry sync; quarry summary` in a terminal.
If the Mac is asleep at 9:00 on the 1st, launchd runs the job when it wakes.
The log holds your payees, amounts and net worth; umask 077 keeps it readable only by you.
