# Specification: Claude install for Claude Code and Claude Desktop

<!-- spec-check: v1 -->

## Intent & Goal

**Primary Goal**: `quarry claude install` and `quarry claude uninstall` act on whichever of Claude Code and Claude Desktop is on this Mac. Claude Code keeps the existing plugin route (skill + MCP server, via the `claude` command). Claude Desktop gets quarry's MCP server as the `mcpServers.quarry` entry in `~/Library/Application Support/Claude/claude_desktop_config.json`, with quarry's absolute path, so a Desktop user never hand-edits JSON.

**Out of Scope**: any `--only`, `--desktop` or `--scope` flag (auto-detect only; the mcp-install spec's reserved `--desktop` flag is withdrawn); `--json` output for these commands (existing refusal stays); Windows and Linux Desktop paths (quarry is macOS-only); a `.mcpb` Desktop extension; installing the skill into Desktop; detecting whether Desktop is running; any key in the Desktop config other than `mcpServers.quarry`; any read or write of `~/.claude.json`, `~/.claude/**` or `.mcp.json`.

**Business Rules**: quarry still never edits Claude Code's files — it only runs the `claude` command. In Claude Desktop's config quarry changes only `mcpServers.quarry`, keeps every other value, saves the file it replaces, and refuses a file it cannot parse or an entry it does not own. An absent target is skipped and named; neither present is a refusal. No test runs the real `claude` or touches the real home directory.

## Business Rules & Invariants
- **BR-D1 target order.** Claude Code is handled fully first (its stdout lines, then its stderr hints or failure), then Claude Desktop the same way. When Desktop is absent, every Code-only output stays byte-identical and only gains a suffix line.
- **BR-D2 Code detected.** `claude` resolves on PATH. Any `LookPath` error = absent → skip S1 (not a refusal).
- **BR-D3 Desktop detected.** `$HOME/Library/Application Support/Claude` exists and is a folder (stat follows a link). Not-exist → S2. Exists but not a folder → S2b. Other stat error → D12. `$HOME` unset → D11. macOS paths only; no GOOS branches (precedent `cmd/quarry/run.go:162`).
- **BR-D4 "ours".** An entry is ours when it starts quarry as an MCP server: key `quarry` under `mcpServers`, whose `command` is a string with last path element `quarry` (bare `"quarry"` included) and whose `args` is exactly `["mcp"]`. Any other value under key `quarry` is foreign (non-object, missing/extra arg, other command). Only key `quarry` is examined.
- **BR-D5 binary path written.** Intent: the path Desktop can still start after this process exits and after upgrades. If `LookPath("quarry")` returns an absolute path with no error and it is `os.SameFile` as `os.Executable()`, write the LookPath path verbatim (keeps a Homebrew symlink stable). Otherwise write `os.Executable()` verbatim. If the chosen path is a temporary build (under `os.TempDir()`, or a path element starting with `go-build`) → D9.
- **BR-D6 install decision.** Ours, same path (string-equal `command`) → no-op, no write, no backup. Ours, other path → replace only `command`; keep `args`, `env`, other keys. Absent → add `{"command": <path>, "args": ["mcp"]}`. Foreign → D4, file untouched.
- **BR-D7 uninstall decision.** Refuses only when it cannot tell whether our entry is there (D1 unparseable, D5su symlink, D6 unreadable) or the entry is foreign (D4u). Every shape that cannot hold our entry → DN, no write: missing file, empty/whitespace-only, non-object top level, `mcpServers` absent/null/non-object, folder at the config path. Removing ours keeps `mcpServers` even when it becomes `{}`. Uninstall never creates and never deletes the file.
- **BR-D8 other values preserved.** After quarry writes, every value other than `mcpServers.quarry` parses to the same value as before; only whitespace and object key order may change. (Illustration: decode other values as raw JSON / `json.Number`, never `float64`; encode without HTML escaping so `<>&` stay literal; 2-space indent, trailing newline. Duplicate keys collapse to the last one — no row.)
- **BR-D9 write safety.** At every instant the config path holds the whole old file or the whole new one; a failure leaves the old one. (Illustration: backup first; temp file in same folder with original mode; fsync; rename over; remove temp on any failure before rename. Folder-fsync error after a successful rename is not a failure.) quarry never follows or replaces a symlink at the config path (lstat before open, so a FIFO cannot block the read).
- **BR-D10 backup.** `~/Library/Application Support/Claude/claude_desktop_config.json.before-quarry`. Written only when quarry writes the config and a config file existed before (including empty). Overwritten each such write. Mode `0600`. Replaced by rename (a symlink at the backup name is replaced, never followed). If the backup cannot be written (D7), the config is not touched.
- **BR-D11 modes.** A replaced config keeps its original mode. A config created by quarry is `0600`.
- **BR-D12 restart line.** DQ/DQu printed only when quarry wrote the config this run (Added, Updated, Removed). Not for already / not-in.
- **BR-D13 exit.** `0` when every present target succeeded (skips, hints, warnings still success). `1` when any present target failed or refused, neither target present, or interrupt. `2` usage. A Code failure does not stop the Desktop step, except interrupt: the Code interrupt line ends the run, Desktop not attempted. A Desktop failure does not undo a Code success.
- **BR-D14 path form.** Every path goes through `claudePath(home, p)` (`internal/cli/render_claude.go:155`) then `%q`, as R7 today. The manual entry `<E>` in D5s and D10 is a fixed single-line form `"quarry": {"command": <cmd>, "args": ["mcp"]}` with `": "` and `", "` separators; `<cmd>` is the path as a JSON string without HTML escaping (D10 keeps the literal placeholder `"<the path command -v quarry prints>"`). The file quarry writes stays BR-D8 form.
- **BR-D16 Code failures print in place.** Every Claude Code failure prints inside the Code block, never at process end. The `default:` arm of `reportClaudeFailure` writes `quarry: claude <verb>: <lead><err.Error()>` via `writeClaudeLine` (`<lead>` = existing `installDoneLead`/`uninstallDoneLead`, empty when no step ran) and returns `ReportedError`; Desktop then runs as for any Code failure; exit 1. Existing `assert.Same` pins on a bare returned error change to asserting the stderr line + `ReportedError`.
- **BR-D17 unclassified Desktop errors.** An Lstat/Stat error on the config file other than not-exist → D6. A JSON encode error on the merged document may be marked `// unreachable:` only when every encoded value is a `json.RawMessage` from a successful decode or a value quarry built (strings, string slice), with that as the stated reason; any `any`/`float64` value forbids the mark.
- **BR-D18 SameFile stat failure.** If stat of either path fails, treat as not the same file → write `os.Executable()`. W1 fires only when the stat of the PATH quarry succeeded.
- **BR-D20 install writes only what it recognises.** Install never writes an entry BR-D4 would not call ours: when the BR-D5 chosen path's last element is not exactly `quarry`, install refuses with D9b before reading the config (mid-feature ruling, SCENARIO-13 checkpoint). Uninstall is unaffected.
- **BR-D19 uninstall failure flow.** BR-D13 continue-on-failure, interrupt-stops and D13 apply to uninstall identically under the uninstall verb.
- **BR-D15 test safety.** Once the real Desktop writer is wired, `cmd/quarry` `testEnv` gives `Home` a `t.TempDir()`-based value (or a fake Desktop locator) and a fake `Executable`, so no `cmd/quarry` test can rewrite the developer's real Desktop config. Lands in the same scenario that wires `defaultEnv`.

---

## Triage Brief

Request: `quarry claude install` / `uninstall` act on every Claude target present. No existing spec covers it; mcp-install excludes Desktop (`docs/specifications/mcp-install/specification.md:9`) — this spec withdraws that exclusion.

### Current surface
- `internal/cli/claude.go:31` `newClaudeCommand(runTool, lookPath, home, jsonOut)`; Short `:34`; Long `:35-36`; group runnable on purpose (`:37-38`) — do not "fix". Helpers `claudeNoArgs` (`:9`), `claudeRefuseJSON` (`:19`).
- `internal/cli/claude_install.go:13` (Short `:17`, Long `:18-28`); builds Server inline (`:33`), `srv.Install` (`:34`), hints `installTurnedOffHint`, `installQuarryNotOnPathWarning` on stderr via `writeClaudeLine`.
- `internal/cli/claude_uninstall.go:13` (Short `:17`, Long `:18-26`), `srv.Uninstall` (`:32`), `uninstallRemainingHints`.
- `internal/cli/render_claude.go`: install lines `:17-21`, refusals/hints `:23-28`, `installAddedLead :30`; uninstall `:32-37`, `:39-42`, `:44`; `claudeRefusalCopy :54`; `reportClaudeFailure :175` (ReportedError for ExitError/InterruptedError/ListUnreadableError/ErrForeignMarketplace/ErrClaudeNotFound/toolrun.StartError, else runtimeError); prefix `quarry: claude <verb>: <text>` (`:170`); `claudePath :155`.
- Exit codes 0/1/2 (`cmd/quarry/run.go` runProcess).
- `internal/claudeplugin`: `Runner` (`claudeplugin.go:8`), `LookPath` (`:12`), `Server` (`:27`), `WithRunner`/`WithLookPath`/`NewServer`, `Result` (`:55`), `Install` (`:70`), `UninstallResult` (`:104`), `Uninstall` (`:119`), `findClaude` (`:151`) → `ErrClaudeNotFound`, `quarryNotOnPath` (`:164`). `state.go`: `ErrClaudeNotFound` (`:32`), `ErrForeignMarketplace` (`:36`), `readState` (`:88`). Server has no filesystem/home concept.
- Wiring: `cli.Env{RunTool, LookPath, Home}` (`internal/cli/run.go:55-57`) — no Executable. Prod: `Home` = `os.UserHomeDir` (`cmd/quarry/run.go:191`), `toolrun.Run` + `exec.LookPath` (`:203-204`). Single call site `internal/cli/root.go:45`. mcp-install BR-8: no store/snapshots/config into the claude group. Open NIT (mcp-install STATE.md): build Server once, pass `srv, home, jsonOut`.
- Tests: `internal/cli/claude_install_test.go`, `claude_uninstall_test.go`, `claude_test.go` (fakes `toolCalls`, `runClaude`, `runClaudeAt(home, marker)`, `findsAt`, `cannotStart`); `internal/claudeplugin` `fakeClaude`/`fakePath`; `cmd/quarry/run_claude_wiring_test.go`. Pins: `Test_claude_*_help_prints_the_ruled_text`, `Test_claude_uninstall_reports_each_outcome`, group-help Contains pins, `cmd/quarry/run_claude_drift_test.go:18`, `run_plugin_readme_test.go:20,29`, `run_plugin_manifest_test.go:31`, `run_skill_drift_test.go:42-48`, `run_status_test.go:145-162`, `mcp_test.go:28,55`.

### Callers
| Symbol | Callers | Via |
|---|---|---|
| `Server.Install` | `internal/cli/claude_install.go:34`; claudeplugin `install_test.go`, `state_test.go` | LSP |
| `Server.Uninstall` | `internal/cli/claude_uninstall.go:32`; claudeplugin tests | grep |
| `NewServer`/`WithRunner`/`WithLookPath` | `claude_install.go:33`, `claude_uninstall.go:31` | grep |
| `Result`/`UninstallResult`/`Copy` | `render_claude.go:61,78,86,95,114,125,139,147`; both cmd files | grep |
| error types | `render_claude.go:181-203,212` | grep |
| `newClaudeCommand` | `internal/cli/root.go:45` | grep |
| `newClaudeInstallCommand` / `newClaudeUninstallCommand` | `internal/cli/claude.go:40` / `:41` | grep |
| `Env.RunTool`/`LookPath`/`Home` | `internal/cli/run.go:55-57`; `cmd/quarry/run.go:191,203-204`; `root.go:45` | grep |

### Prior art
- No JSON read-merge-write or atomic replace exists. `internal/platform/atomicfile` is exclusive-create + no-clobber (never overwrites); exports `SyncDir` (`atomicfile.go:34`). A replace helper is new.
- `os.Executable` unused anywhere; `exec.LookPath` only.
- macOS paths hard-coded, no GOOS/build tags (`cmd/quarry/run.go:162`, `internal/snapshot/discover.go:56`).
- Twin shape: `claudeplugin` Server + typed errors; all copy in `internal/cli/render_claude.go` (mcp-install STATE.md binding); `homepath.Abbreviate`.

### Already exists — do not re-plan
`Runner`/`LookPath`/`toolrun.Run`/`exec.LookPath` wiring; `Env.Home` injection and `claudePath`; `reportClaudeFailure`/`writeClaudeLine`/`ReportedError`/`runtimeError` and the `quarry: claude <verb>:` prefix; `quarry mcp`; `--json` and no-arg guards; Code plugin install/uninstall idempotence and foreign-marketplace guard; `atomicfile.SyncDir`.

### Must be built
Desktop config read-merge-write in a feature package sibling to `claudeplugin`, with typed errors; atomic replace helper; absolute binary path resolution plumbed through `Env.Executable`; target detection; per-target results and copy; claude-missing becomes skip; updated help, README, `mcp.go` Long, PRD lines, pins.

### Becomes dead
`installClaudeNotFoundRefusal` (`render_claude.go:28`), `uninstallClaudeNotFoundRefusal` (`:42`), `claudeRefusals.notFound` (`:50`), the `ErrClaudeNotFound` arm of `reportClaudeFailure` (`:199-202`). `ErrClaudeNotFound` itself stays as the S1 skip signal.

## Product Verdict

**SHIP WITH CHANGES** (Phase 1). User decisions: absent target skipped and named, neither present = refusal; foreign Desktop entry refused, stale ours updated; unparseable refused, valid gets backup + atomic replace + re-indent; no target-selection flag. Accepted changes, all folded into rules and copy here:
1. Test safety BR-D15 (binding, same scenario that wires `defaultEnv`).
2. Tell the user to quit Desktop first — Desktop is reported to rewrite `claude_desktop_config.json` itself while running (anthropics/claude-code#32345, #34359). Trailing line "Quit and reopen Claude Desktop to load it."
3. Other values preserved (BR-D8).
4. Symlinked config refused, never followed or replaced; file modes BR-D10/BR-D11.
5. Uninstall with a foreign `quarry` entry exits 1 (mirrors Code R1).
6. New warning W1 when PATH quarry differs from the path Desktop starts.
7. Delete the dead not-found copy in this feature.
8. Uninstall with neither present exits 1 (user decision 1; user did not reverse it).

MCP counterpart: none (MCP server never changes data).

## Surface & Copy

### A. Commands and help (verbatim)

**`quarry claude`** — group behaviour unchanged (bare → help exit 0, unknown name exit 2).
- Short: `Install quarry in Claude Code and Claude Desktop, or remove it`
- Long:
```
Install quarry in Claude Code and Claude Desktop, or remove it. Each
command acts on whichever of the two is on this Mac and skips the other.
```
- Root help row (`cmd/quarry/run_status_test.go:145-162`): `"  claude      Install quarry in Claude Code and Claude Desktop, or remove it\n"+`

**`quarry claude install`** — no flags, no args, no Example.
- Short: `Install quarry in Claude Code and Claude Desktop`
- Long:
```
Install quarry in Claude Code and in Claude Desktop, whichever of the two
is on this Mac; install skips the other and says so.

Claude Code counts as present when the claude command is on your PATH.
install adds quarry's plugin for all your projects: the quarry skill,
which teaches Claude to answer from quarry, and quarry's MCP server. It
runs:

  claude plugin marketplace add --scope user koblas/quarry
  claude plugin install --scope user quarry@quarry

Claude Code downloads the plugin from github.com/koblas/quarry; quarry
itself sends nothing and opens none of Claude Code's files. The plugin
starts "quarry" from your PATH.

Claude Desktop counts as present when ~/Library/Application Support/Claude
exists. install adds quarry's MCP server, not the skill, as the "quarry"
entry under mcpServers in claude_desktop_config.json in that folder,
with the full path to quarry. Every other setting stays as it was, though
the file's layout may change, and the file it replaces is saved as
claude_desktop_config.json.before-quarry. Quit Claude Desktop first: it
can rewrite the file while it runs.

Each step already done is skipped, so running install again is safe.
Restart Claude Code, or quit and reopen Claude Desktop, to load quarry.
```

**`quarry claude uninstall`** — no flags, no args, no Example.
- Short: `Uninstall quarry from Claude Code and Claude Desktop`
- Long:
```
Uninstall quarry from Claude Code and from Claude Desktop, whichever of
the two is on this Mac; uninstall skips the other and says so.

In Claude Code, uninstall removes quarry's plugin from your user scope
and the quarry marketplace. It runs:

  claude plugin uninstall --scope user quarry@quarry
  claude plugin marketplace remove --scope user quarry

A copy of the plugin installed for a single project stays, and uninstall
names each one.

In Claude Desktop, uninstall removes the "quarry" entry under mcpServers
in ~/Library/Application Support/Claude/claude_desktop_config.json when
that entry runs quarry mcp. Every other setting stays as it was, though
the file's layout may change, and the file it replaces is saved as
claude_desktop_config.json.before-quarry. Quit Claude Desktop first.

Each step with nothing to remove is skipped. Your quarry store and
snapshots are not touched.
```

Unchanged: `--json` refusals (exit 2), no-args refusals (exit 2), group unknown-command line (exit 2), every child argv string, every existing Code stdout line, hint and failure line (`render_claude.go:17-44`, `:127-134`, `:163-224`).

### C. stdout lines (one per outcome, target order)

Code lines unchanged (`render_claude.go:17-21`, `:32-37`). The Code plugin lines that do not name Claude Code stay byte-identical. No per-target headers.

| ID | Verb | Line |
|---|---|---|
| S1 | both | `Skipped Claude Code: no claude command on your PATH.` |
| S2 | both | `Skipped Claude Desktop: "~/Library/Application Support/Claude" does not exist.` |
| S2b | both | `Skipped Claude Desktop: "~/Library/Application Support/Claude" is not a folder.` |
| DA | install | `Added the quarry MCP server to Claude Desktop; it starts "<path>".` |
| DU | install | `Updated the quarry MCP server in Claude Desktop to start "<new path>" instead of "<old command>".` |
| DK | install | `The quarry MCP server is already in Claude Desktop; it starts "<path>".` |
| DR | uninstall | `Removed the quarry MCP server from Claude Desktop.` |
| DN | uninstall | `The quarry MCP server is not in Claude Desktop.` |
| DQ | install | `Quit and reopen Claude Desktop to load it.` (BR-D12) |
| DQu | uninstall | `Quit and reopen Claude Desktop to unload it.` (BR-D12) |

`<old command>` is the entry's `command` as stored (bare `quarry` prints `"quarry"`), with BR-D14 applied.

Examples — both present, fresh install (exit 0):
```
Added the quarry marketplace to Claude Code.
Installed the quarry plugin (skill and MCP server) for all your projects.
Restart Claude Code to load it.
Added the quarry MCP server to Claude Desktop; it starts "/opt/homebrew/bin/quarry".
Quit and reopen Claude Desktop to load it.
```
Code missing, Desktop entry already there (exit 0, no quit line):
```
Skipped Claude Code: no claude command on your PATH.
The quarry MCP server is already in Claude Desktop; it starts "/opt/homebrew/bin/quarry".
```
Uninstall, Desktop absent (exit 0):
```
Uninstalled the quarry plugin from Claude Code.
Removed the quarry marketplace from Claude Code.
Restart Claude Code to unload it.
Skipped Claude Desktop: "~/Library/Application Support/Claude" does not exist.
```

### D. stderr lines (prefix `quarry: claude <verb>: `, `writeClaudeLine` at `render_claude.go:169`)

`<P>` = `"~/Library/Application Support/Claude/claude_desktop_config.json"`, `<B>` = `"~/Library/Application Support/Claude/claude_desktop_config.json.before-quarry"`, `<D>` = `"~/Library/Application Support/Claude"`. Uninstall swaps the verb unless a separate line is shown.

| ID | Cause | Line | Exit |
|---|---|---|---|
| N1 | neither present, install | `quarry: claude install: found neither Claude Code (no claude command on your PATH) nor Claude Desktop ("~/Library/Application Support/Claude" does not exist); install either one, open it once, then run quarry claude install again` | 1 |
| N1u | neither present, uninstall | `quarry: claude uninstall: found neither Claude Code (no claude command on your PATH) nor Claude Desktop ("~/Library/Application Support/Claude" does not exist), so there is nothing to uninstall; if claude is installed, add it to your PATH, then run quarry claude uninstall again` | 1 |
| N1b | Code absent, `Claude` is a file, install | `quarry: claude install: found neither Claude Code (no claude command on your PATH) nor Claude Desktop ("~/Library/Application Support/Claude" is not a folder); install either one, open it once, then run quarry claude install again` | 1 |
| N1bu | Code absent, `Claude` is a file, uninstall | `quarry: claude uninstall: found neither Claude Code (no claude command on your PATH) nor Claude Desktop ("~/Library/Application Support/Claude" is not a folder), so there is nothing to uninstall; if claude is installed, add it to your PATH, then run quarry claude uninstall again` | 1 |
| C1 | unclassified Code error (BR-D16) | `quarry: claude <verb>: <lead><err>` — e.g. `quarry: claude install: added the quarry marketplace, but <err>` | 1 |
| D1 | not valid JSON | `quarry: claude install: cannot read <P>: it is not valid JSON (<json error>, at byte <offset>); fix it so Claude Desktop can read it too, then run quarry claude install again` — drop `, at byte <offset>` when not a `*json.SyntaxError` | 1 |
| D2 | install: top level not an object | `quarry: claude install: cannot add quarry to <P>: it holds a JSON <kind>, not an object; fix it so Claude Desktop can read it too, then run quarry claude install again` — `<kind>` ∈ null, array, string, number, boolean. Uninstall → DN | 1 |
| D3 | install: `mcpServers` neither object nor null | `quarry: claude install: cannot add quarry to <P>: its mcpServers is a JSON <kind>, not an object; fix it so Claude Desktop can read it too, then run quarry claude install again` — no `null` (null = absent). Uninstall → DN | 1 |
| D4 | install: foreign entry | `quarry: claude install: Claude Desktop has an MCP server named "quarry" that does not run quarry mcp, so quarry leaves it alone; rename or remove it in <P>, then run quarry claude install again` | 1 |
| D4u | uninstall: foreign entry | `quarry: claude uninstall: Claude Desktop has an MCP server named "quarry" that does not run quarry mcp, so quarry leaves it alone; to remove it, delete it from <P> yourself` | 1 |
| D5s | install: config is a symlink | `quarry: claude install: <P> is a symbolic link, so quarry leaves it alone; add <E> under mcpServers in the file it links to yourself, then quit and reopen Claude Desktop` — `<E>` e.g. `"quarry": {"command": "/opt/homebrew/bin/quarry", "args": ["mcp"]}` | 1 |
| D5su | uninstall: config is a symlink | `quarry: claude uninstall: <P> is a symbolic link, so quarry leaves it alone; if it has a "quarry" entry under mcpServers, remove it yourself` | 1 |
| D5o | install: config is a folder or non-regular file | `quarry: claude install: <P> is not a file, so quarry leaves it alone; move it aside, then run quarry claude install again` — uninstall → DN | 1 |
| D6 | cannot open/read | `quarry: claude install: cannot read <P> (<os reason>); check its permissions, then run quarry claude install again` — `osreason.Reason` (`render_claude.go:165`) | 1 |
| D7 | backup write fails | `quarry: claude install: cannot save <B> (<os reason>), so <P> is unchanged; check the permissions of <D>, then run quarry claude install again` | 1 |
| D8 | temp create/write/fsync/rename fails | `quarry: claude install: cannot write <P> (<os reason>), so it is unchanged; check the permissions of <D>, then run quarry claude install again` | 1 |
| D9 | install: temporary binary | `quarry: claude install: this quarry runs from a temporary build ("<exe>"), which will be gone when Claude Desktop starts it; run quarry claude install from an installed quarry, not go run` | 1 |
| D9b | install: chosen path's last element is not exactly `quarry` (checked after D9, before the config is read; D9 wins when both apply) | `quarry: claude install: this quarry binary is named "<base>", and quarry recognises its Claude Desktop entry only when the binary is named quarry; rename it to quarry, or put a link named quarry to it on your PATH, then run quarry claude install again` — `<base>` = chosen path's last element via `%q`; no `<E>` offered | 1 |
| D10 | install: `os.Executable` fails | `quarry: claude install: cannot tell where this quarry binary is (<os reason>), so quarry cannot add it to Claude Desktop; add "quarry": {"command": "<the path command -v quarry prints>", "args": ["mcp"]} under mcpServers in <P> yourself` — expected unreachable on darwin; `// unreachable:` only with a reason | 1 |
| D11 | `$HOME` unset | `quarry: claude install: cannot find your home directory ($HOME is not set), so quarry cannot look for Claude Desktop; set HOME, then run quarry claude install again` (reuses `errNoHome` wording, `cmd/quarry/run.go:177`) | 1 |
| D12 | Desktop folder stat error other than not-exist | `quarry: claude install: cannot check for Claude Desktop at <D> (<os reason>); check its permissions, then run quarry claude install again` | 1 |
| D13 | ctx cancelled after Code finished, before Desktop written | `quarry: claude install: stopped before quarry changed Claude Desktop; run quarry claude install again` (uninstall: `quarry: claude uninstall: stopped before quarry changed Claude Desktop; run quarry claude uninstall again`) — once the rename has started it completes and success lines print | 1 |
| W1 | install warning after DA/DU/DK: PATH has an absolute `quarry`, its stat succeeded, and it is not the same file as the written path (BR-D18) | `quarry: claude install: warning: Claude Desktop starts "<written>", but the quarry on your PATH is "<path quarry>"; run quarry claude install with the quarry you want Claude Desktop to start` | 0 |

Every Desktop refusal leaves config and backup untouched. Each failing target prints exactly one line at its place in target order. When N1/N1u fires, stdout is empty.

### E. Edge-case rows

**Desktop, install** (Code present and successful):

| Input | stdout | stderr | Exit | File |
|---|---|---|---|---|
| Desktop folder missing | S2 | — | 0 | none |
| `Claude` is a regular file | S2b | — | 0 | none |
| folder stat EACCES | — | D12 | 1 | none |
| HOME unset | — | D11 | 1 | none |
| config missing, folder present | DA, DQ | — | 0 | created 0600, no backup |
| empty file | DA, DQ | — | 0 | backup (empty), replaced |
| whitespace-only | DA, DQ | — | 0 | backup, replaced |
| unparseable / truncated / BOM | — | D1 | 1 | untouched |
| top level null/array/string/number/bool | — | D2 | 1 | untouched |
| `mcpServers` absent or null | DA, DQ | — | 0 | backup, replaced |
| `mcpServers` array/string/number/bool | — | D3 | 1 | untouched |
| ours, same path | DK | W1 if applicable | 0 | untouched, no backup |
| ours, other path (bare `"quarry"` incl.) | DU, DQ | W1 if applicable | 0 | backup, only `command` changed |
| foreign entry | — | D4 | 1 | untouched |
| symlink | — | D5s | 1 | untouched |
| folder or FIFO | — | D5o | 1 | untouched |
| unreadable | — | D6 | 1 | untouched |
| backup cannot be written | — | D7 | 1 | untouched |
| temp/rename fails | — | D8 | 1 | untouched, temp removed |
| `go run` / `$TMPDIR` binary | — | D9 | 1 | untouched (checked before reading config) |
| binary not named quarry (no SameFile `quarry` on PATH) | — | D9b | 1 | untouched (checked before reading config) |
| another key already runs quarry mcp | per `quarry` key | — | — | no row: only key `quarry` judged |

**Desktop, uninstall:**

| Input | stdout | stderr | Exit | File |
|---|---|---|---|---|
| folder missing / not a folder / stat error / HOME unset | S2 / S2b / — / — | — / — / D12 / D11 | 0 / 0 / 1 / 1 | none |
| config missing, empty, whitespace-only | DN | — | 0 | none created/written |
| non-object top level; `mcpServers` absent/null/non-object | DN | — | 0 | untouched |
| folder at config path | DN | — | 0 | untouched |
| unparseable | — | D1 | 1 | untouched |
| ours (any path) | DR, DQu | — | 0 | backup; entry removed; `mcpServers` kept |
| foreign | — | D4u | 1 | untouched |
| symlink | — | D5su | 1 | untouched |
| unreadable / backup fails / write fails | — | D6 / D7 / D8 | 1 | untouched |

**Code × Desktop:**

| Code outcome | Code output | Desktop runs? | Exit |
|---|---|---|---|
| `claude` not on PATH | S1 | yes | per Desktop |
| success / skips / R2 / PATH warning / R6 / Kept | unchanged | yes | per Desktop |
| R1 foreign marketplace | unchanged R1 | yes | 1 |
| R4 unreadable list, exit≠0, signal, cannot-run | unchanged replay + line | yes | 1 |
| R5 interrupted | unchanged interrupt line | no | 1 |
| Code absent and Desktop absent | N1 / N1u only | — | 1 |
| Code absent, HOME unset | S1, then D11 | — | 1 |
| Code absent, Desktop folder stat error | S1, then D12 | — | 1 |
| Code absent, `Claude` is a file | N1b / N1bu only | — | 1 |
| unclassified Code error | C1 in place | yes | 1 |

### Changes to existing surfaces
1. `internal/cli/claude.go:34-36`, `claude_install.go:17-28`, `claude_uninstall.go:17-26` → section A.
2. Root help row `cmd/quarry/run_status_test.go:145-162` → section A. Group-help child column width unchanged.
3. `internal/cli/mcp.go:28-32` paragraph becomes:
```
In Claude Code or Claude Desktop, run quarry claude install: in Claude
Code it installs quarry's plugin, which adds this server and the quarry
skill; in Claude Desktop it adds this server with quarry's full path. In
other MCP clients, add a server with the command "quarry" and the
argument "mcp". An app started outside a terminal may not find quarry on
your PATH; give it the full path that "command -v quarry" prints.
```
4. README.md:
   - L5 heading → `## Use quarry with Claude Code or Claude Desktop`
   - L14 → `Then add quarry to Claude Code and Claude Desktop, whichever you have:`
   - L20 → `In Claude Code this runs claude plugin marketplace add koblas/quarry and claude plugin install quarry@quarry for you; you can run those two yourself instead. In Claude Desktop it adds quarry's MCP server; see below.` (keep the two commands in backticks exactly as now — drift pin reads them)
   - L28 heading → `## Install or remove quarry`
   - L30 → ``In Claude Code, `quarry claude install` runs two commands for you, skipping each one that is already done, so running it again is safe:``
   - L44 → ``A copy of the plugin installed only for a project, or by your organization, stays; `quarry claude uninstall` names each one, and does not remove the quarry marketplace while any remains.`` (closes mcp-install STATE.md debt)
   - New paragraph after L44: ``In Claude Desktop, `quarry claude install` adds quarry's MCP server, not the skill, as the `quarry` entry under `mcpServers` in `~/Library/Application Support/Claude/claude_desktop_config.json`, with the full path to quarry, and saves the file it replaces as `claude_desktop_config.json.before-quarry` beside it. Every other setting stays as it was. Quit Claude Desktop before you run it, since Desktop can rewrite that file while it runs, then reopen Desktop to load quarry. `quarry claude uninstall` removes that entry and nothing else. If Claude Code or Claude Desktop is not on this Mac, both commands skip it and say so.``
   - L46 → `To add only the MCP server to Claude Code, without the skill, run:`
5. `docs/initial-prd.md:333` → `- Claude install: quarry claude install / uninstall act on Claude Code and Claude Desktop, whichever is on the Mac. Claude Code gets the plugin by running the claude command (user scope); quarry never edits Claude Code's files, and the download is Claude Code's, not quarry's. Claude Desktop gets only the MCP server, as the mcpServers.quarry entry in its config file with quarry's full path; quarry changes only that entry, keeps a backup of the file it replaces, and refuses a file it cannot parse or an entry it does not own.`
6. PRD Security and privacy, new bullet after `:266`: `- **Claude Desktop config.** quarry claude install and uninstall change only the quarry entry in Claude Desktop's config file, never another key, and save the file they replace beside it.`
7. PRD `:67`, `:99`, `:195`: no change.
8. SKILL and references: no change. Architect checks `run_skill_drift_test.go:42-48` still parses the README Claude Code section.
9. Pins to re-assert: help pins, group-help pins, `run_status_test.go:145-162`, `Test_claude_uninstall_reports_each_outcome` (S2 suffix or Desktop-present fake home — pick deliberately), mcp-install SCENARIO-05/05b not-found tests re-pointed to S1/N1/N1u, `mcp_test.go:28,55`, `run_plugin_readme_test.go:20,29,41-62`, `run_claude_drift_test.go:18`.

### Seams (binding for the architect)
- `cli.Env` gains `Executable func() (string, error)`. Desktop folder derived from `Env.Home`.
- Desktop config read/merge + atomic replace live in a feature package sibling to `claudeplugin`, with typed errors. All copy stays in `internal/cli/render_claude.go`.
- Replace helper is new (`atomicfile` is exclusive-create); BR-D9 is its contract.
- BR-D15 test safety.

### User-verified, outside the pipeline
1. Quit Desktop, run `quarry claude install`, reopen: quarry tools appear and other MCP servers still load.
2. Quit/reopen Desktop again, re-run install → DK (Desktop did not drop the entry). Once with Desktop left running during install.
3. After `brew upgrade`, install prints DK, not DU.
4. `cp claude_desktop_config.json.before-quarry claude_desktop_config.json` restores the earlier state.
5. `jq -S` diff before/after install and uninstall shows only the quarry entry changed (env secrets, numbers, `<>&` intact).
6. Desktop installed but never opened → S2; after opening once, install adds the entry.

---

## Scenarios (Gherkin)

```gherkin
Scenario: SCENARIO-01 — Install reaches both Claude Code and Claude Desktop
  Given claude is on PATH, the Claude Desktop folder exists and holds no config file
  When the user runs quarry claude install
  Then the Claude Code lines print, then DA and DQ
  And the config is created with mode 0600 holding mcpServers.quarry = {"command": <absolute path>, "args": ["mcp"]}, and the exit code is 0

Scenario Outline: SCENARIO-02 — Install merges into an existing Desktop config and keeps a backup
  Given the Desktop config holds <existing>
  When the user runs quarry claude install
  Then the quarry entry is added, every other value parses unchanged, the prior file is saved as claude_desktop_config.json.before-quarry with mode 0600, the config keeps its original mode, and the exit code is 0
  Examples:
    | existing |
    | an empty file |
    | whitespace only |
    | an object with no mcpServers |
    | mcpServers null |
    | other servers with env secrets, large numbers and <>& characters |

Scenario: SCENARIO-03 — Re-running install when Desktop already starts this quarry changes nothing
  Given mcpServers.quarry already runs the same path with args ["mcp"]
  When the user runs quarry claude install
  Then DK prints with no quit line, config and backup are untouched, and the exit code is 0

Scenario: SCENARIO-04 — Install repoints a stale quarry entry
  Given mcpServers.quarry runs quarry mcp from another path (bare "quarry" included) and carries env
  When the user runs quarry claude install
  Then only command changes, env is kept, DU and DQ print, a backup is written, and the exit code is 0

Scenario: SCENARIO-05 — Install refuses a foreign quarry entry
  Given mcpServers.quarry does not run quarry mcp
  When the user runs quarry claude install
  Then D4 prints on stderr, the file is untouched, and the exit code is 1

Scenario Outline: SCENARIO-06 — Install refuses a config it cannot safely change
  Given the config is <shape>
  When the user runs quarry claude install
  Then <line> prints on stderr, config and backup are untouched, and the exit code is 1
  Examples:
    | shape | line |
    | not valid JSON | D1 |
    | a JSON value that is not an object | D2 |
    | an object whose mcpServers is not an object | D3 |
    | a symbolic link | D5s |
    | a folder or FIFO | D5o |
    | unreadable | D6 |

Scenario Outline: SCENARIO-07 — A failed write leaves the Desktop config as it was
  Given <failure>
  When the user runs quarry claude install
  Then <line> prints on stderr, the config is unchanged, no temp file is left, and the exit code is 1
  Examples:
    | failure | line |
    | the backup cannot be saved | D7 |
    | the temp write or rename fails | D8 |

Scenario Outline: SCENARIO-08 — Install picks the quarry path Claude Desktop will start
  Given the running binary is <exe> and PATH resolves quarry to <path quarry>
  When the user runs quarry claude install
  Then <outcome>
  Examples:
    | exe | path quarry | outcome |
    | a Homebrew Cellar file | a symlink to that file | the symlink path is written, no warning, exit 0 |
    | an installed file | a different file | the running binary's path is written, W1 warns, exit 0 |
    | an installed file | none | the running binary's path is written, no warning, exit 0 |
    | a go-build or $TMPDIR file | any | D9, config untouched, exit 1 |

Scenario Outline: SCENARIO-09 — An absent target is skipped and named
  Given <absent> and the other target is present
  When the user runs quarry claude <verb>
  Then <skip line> prints on stdout in target order, the present target is handled as usual, and the exit code is 0
  Examples:
    | absent | verb | skip line |
    | no claude on PATH | install | S1 |
    | no claude on PATH | uninstall | S1 |
    | the Desktop folder is missing | install | S2 |
    | the Desktop folder is missing | uninstall | S2 |
    | Claude is a file, not a folder | install | S2b |

Scenario Outline: SCENARIO-10 — Neither target present is a refusal
  Given no claude on PATH and no Claude Desktop folder
  When the user runs quarry claude <verb>
  Then <line> prints on stderr, stdout is empty, and the exit code is 1
  Examples:
    | verb | line |
    | install | N1 |
    | uninstall | N1u |
    | install, Claude is a file | N1b |
    | uninstall, Claude is a file | N1bu |

Scenario Outline: SCENARIO-11 — A Claude Code failure does not stop Claude Desktop, an interrupt does
  Given Claude Code ends in <code outcome> and Claude Desktop is present
  When the user runs quarry claude install
  Then the unchanged Code failure line prints, Claude Desktop <desktop>, and the exit code is 1
  Examples:
    | code outcome | desktop |
    | a foreign quarry marketplace | is installed as usual |
    | a step exiting non-zero | is installed as usual |
    | an unreadable list | is installed as usual |
    | an interrupt during a Code step | is not attempted |
    | an interrupt after Code finished, before the Desktop write | is not changed and D13 prints |

Scenario Outline: SCENARIO-12 — A Desktop lookup that cannot be checked fails rather than skips
  Given <cause>
  When the user runs quarry claude <verb>
  Then <line> prints on stderr and the exit code is 1
  Examples:
    | cause | verb | line |
    | HOME is unset | install | D11 |
    | the Desktop folder cannot be stat'ed | uninstall | D12 |

Scenario: SCENARIO-13 — Uninstall removes quarry's Desktop entry
  Given mcpServers.quarry runs quarry mcp beside other servers
  When the user runs quarry claude uninstall
  Then DR and DQu print, a backup is written, only the quarry entry is gone, mcpServers is kept (even when empty), and the exit code is 0

Scenario Outline: SCENARIO-14 — Uninstall with no quarry entry changes nothing
  Given the config is <shape>
  When the user runs quarry claude uninstall
  Then DN prints, nothing is created or written, and the exit code is 0
  Examples:
    | shape |
    | missing |
    | empty |
    | whitespace only |
    | a JSON value that is not an object |
    | an object whose mcpServers is absent, null or not an object |
    | a folder |

Scenario Outline: SCENARIO-15 — Uninstall refuses what it cannot judge
  Given the config is <shape>
  When the user runs quarry claude uninstall
  Then <line> prints on stderr, the config is untouched, and the exit code is 1
  Examples:
    | shape | line |
    | holding a foreign quarry entry | D4u |
    | a symbolic link | D5su |
    | not valid JSON | D1 |
    | unreadable | D6 |
    | ours, but the backup cannot be saved | D7 |
    | ours, but the write fails | D8 |

Scenario Outline: SCENARIO-16 — Help and docs describe both targets
  When the user runs quarry <command> --help
  Then the ruled Short and Long print verbatim, and the README and PRD lines match Changes to existing surfaces
  Examples:
    | command |
    | claude |
    | claude install |
    | claude uninstall |
    | mcp |
```

---

## Sizing

Sizing pass (architect, 2026-10-08): 16 scenarios → 9 units, 5 FOLDs, no SPLIT, no LIGHT. Orchestration lives in `internal/cli` (forced: two sibling feature packages). New packages: `internal/claudedesktop` (Desktop config decisions, typed errors) and a platform replace helper (suggested `internal/platform/replacefile`, temp → chmod → fsync → rename → `atomicfile.SyncDir`). Fallback SPLIT seam for SCENARIO-01 if its run overruns: 01a = replace helper + `claudedesktop` create path; 01b = cli orchestration + `Env.Executable` + `defaultEnv` + BR-D15 + pin re-point (tick on 01b's `cli.Run` test).

Binding for every plan: test-fixture `Executable` values are fixed absolute strings outside `$TMPDIR` (a temp-dir exe flips to D9 once SCENARIO-08 lands); temp root is a Server option, never a direct `os.TempDir()` read; cli test helpers `runClaude`/`runClaudeFinding` move from `home ""` to a `t.TempDir()` home with no Desktop folder in SCENARIO-01. Interim safety: SCENARIO-02 refuses any present `quarry` key (never overwrites) until SCENARIO-04 lands; SCENARIO-13's removal carries the ours-only guard from its first commit.

| Scenario | Verdict — numbers |
| --- | --- |
| SCENARIO-01 | OWNS A RUN — 4 batches, `internal/claudedesktop` + replace helper (test-first). Adds `Env.Executable`, `defaultEnv` wiring, BR-D15; Desktop after Code in install with DA/DQ; carries S2 (install) and the D11 guard (empty home would make the Desktop path relative); re-points install Code-success pins to gain the S2 suffix. |
| SCENARIO-02 | OWNS A RUN — 3 batches: merge preserving other values, backup, modes, D7/D8 copy; test-first backup-before-config ordering. Folds SCENARIO-07 (faults must fire after the temp exists: non-empty folder at backup name for D7; user-immutable config via `syscall.Chflags` for D8). |
| SCENARIO-07 | FOLD → SCENARIO-02 |
| SCENARIO-04 | OWNS A RUN — 3 batches: BR-D4 "ours" check with D4 refusal (test-first, clobber guard), DU, DK no-op, BR-D12 quit-line suppression. Folds SCENARIO-03, 05. |
| SCENARIO-03, 05 | FOLD → SCENARIO-04 |
| SCENARIO-06 | OWNS A RUN — 3 batches: lstat-before-open refusing symlink/non-regular (D5s/D5o, test-first), D6 (incl. BR-D17 stat mapping), D1 offset, D2/D3 kinds, `<E>` single-line form. |
| SCENARIO-08 | OWNS A RUN — 2 batches: BR-D5/BR-D18 path choice, W1/D9/D10. Acceptance test at the `claudedesktop` Server boundary (real files + `WithTempDir`); cli pins W1/D9/D10 with fixed-string paths. |
| SCENARIO-13 | OWNS A RUN — 3 batches: BR-D7 uninstall decision with ours-only removal guard (test-first, destructive); Desktop wired into uninstall with S2/DR/DQu/DN; D4u/D5su; D1/D6/D7/D8 under uninstall verb; re-points uninstall success pins. Folds SCENARIO-14, 15. |
| SCENARIO-14, 15 | FOLD → SCENARIO-13 |
| SCENARIO-09 | OWNS A RUN — 3 batches: S2b, D12, `ErrClaudeNotFound` → S1, N1/N1u/N1b/N1bu; deletes dead not-found copy (`render_claude.go:28,42,50,199-202`) and re-points mcp-install not-found tests (`claude_install_test.go:224-239`, `claude_uninstall_test.go:240-247`). Folds SCENARIO-10, 12 (D11 install row green on arrival from 01). |
| SCENARIO-10, 12 | FOLD → SCENARIO-09 |
| SCENARIO-11 | OWNS A RUN — 2 batches: continue-on-failure, interrupt-stops, D13, C1 in-place printing (BR-D16) — for **both verbs** (BR-D19; plan adds uninstall pins beyond the install-only Gherkin). Re-points failure-path pins, raw-path tests gaining D11 (`claude_install_test.go:280`), `run_claude_wiring_test.go` `Empty(stdout)` tests gaining S2. |
| SCENARIO-16 | OWNS A RUN — 2 batches (sonnet): help Short/Long ×3, `mcp.go` Long, root help row, README, PRD, drift/readme pins. |

## BDD Acceptance Progress
- [x] SCENARIO-01: Install reaches both Claude Code and Claude Desktop — `internal/cli/claude_install_desktop_test.go` `Test_claude_install_adds_quarry_to_claude_desktop_after_claude_code`
- [x] SCENARIO-02: Install merges into an existing Desktop config and keeps a backup (folds 07, whose acceptance test is `internal/cli/claude_install_desktop_test.go` `Test_claude_install_reports_a_failed_backup_or_write_and_leaves_the_config_as_it_was`) — `internal/cli/claude_install_desktop_test.go` `Test_claude_install_merges_into_an_existing_desktop_config_and_keeps_a_backup`
- [x] SCENARIO-04: Install repoints a stale quarry entry (folds 03, whose acceptance test is `internal/cli/claude_install_desktop_test.go` `Test_claude_install_leaves_the_desktop_config_alone_when_it_already_starts_this_quarry`, and 05, whose acceptance test is `internal/cli/claude_install_desktop_test.go` `Test_claude_install_refuses_a_quarry_entry_that_does_not_run_quarry_mcp`) — `internal/cli/claude_install_desktop_test.go` `Test_claude_install_repoints_a_stale_quarry_entry_and_keeps_its_env`
- [x] SCENARIO-06: Install refuses a config it cannot safely change — `internal/cli/claude_install_desktop_test.go` `Test_claude_install_refuses_a_desktop_config_it_cannot_safely_change`
- [x] SCENARIO-08: Install picks the quarry path Claude Desktop will start — `internal/claudedesktop/path_test.go` `Test_install_writes_the_quarry_path_that_claude_desktop_will_start`
- [x] SCENARIO-13: Uninstall removes quarry's Desktop entry (folds 14, whose acceptance test is `internal/cli/claude_uninstall_desktop_test.go` `Test_claude_uninstall_leaves_claude_desktop_alone_when_its_config_has_no_quarry_entry`, and 15, whose acceptance test is `internal/cli/claude_uninstall_desktop_test.go` `Test_claude_uninstall_refuses_a_desktop_config_it_cannot_judge`) — `internal/cli/claude_uninstall_desktop_test.go` `Test_claude_uninstall_removes_quarry_from_claude_desktop_after_claude_code`
- [x] SCENARIO-09: An absent target is skipped and named (folds 10, whose acceptance test is `internal/cli/claude_absent_target_test.go` `Test_claude_refuses_when_neither_claude_code_nor_claude_desktop_is_present`, and 12, whose acceptance test is `internal/cli/claude_absent_target_test.go` `Test_claude_refuses_a_claude_desktop_it_cannot_look_for`) — `internal/cli/claude_absent_target_test.go` `Test_claude_skips_the_absent_target_and_handles_the_present_one`
- [ ] SCENARIO-11: A Claude Code failure does not stop Claude Desktop, an interrupt does
- [ ] SCENARIO-16: Help and docs describe both targets
