# Specification: Claude Code plugin install (`quarry claude install` / `uninstall`)

<!-- spec-check: v1 -->

## Intent & Goal

**Primary Goal**: one command installs quarry into Claude Code (skill plus MCP server) and one removes it, so a user never has to remember the two `claude plugin` lines. Request (2026-10-07): "for the `mcp` command we should have a `--install` option and `--uninstall` that would add to the settings" — reshaped by the user to a separate command group ("maybe just `quarry ai install` ... or something similar"), ruled `quarry claude`. User also asked that README.md document it.

**Out of Scope**: Claude Desktop (`claude_desktop_config.json`; a reserved future `--desktop` flag, not built); MCP-only registration (`claude mcp add` stays a documented manual route); any `--scope` flag (user scope only); `--json` output for these commands; enabling a disabled plugin; project/local-scope removal (hints only); any read or write of `~/.claude.json`, `~/.claude/**` or `.mcp.json` by quarry.

**Business Rules**: quarry never edits Claude Code's files — it only runs the `claude` command; every re-run is safe because quarry reads `claude`'s JSON lists before each step and skips steps already done; quarry acts only on its own marketplace (`koblas/quarry` on GitHub) and refuses when a foreign marketplace named `quarry` exists; children never prompt (stdin `/dev/null`, no `-y`), child output is shown only on failure; no test runs the real `claude`.

## Business Rules & Invariants
- **BR-1 (shell-out only).** quarry never opens, reads or writes `~/.claude.json`, `~/.claude/**` or `.mcp.json`. Every change goes through `claude plugin ...` child processes. Only the owner of a format writes it.
- **BR-2 (idempotent).** Before any step, quarry runs `claude plugin marketplace list --json` then `claude plugin list --json` and decides each step from those lists. Marketplace is *ours and present* when an entry has `name == "quarry" && source == "github" && repo == "koblas/quarry"`. Plugin is *installed at user scope* when an entry has `id == "quarry@quarry" && scope == "user"` (`enabled` ignored for presence; missing `enabled` counts as enabled). A step already done is skipped and reported with its "already"/"not" line.
- **BR-3 (foreign marketplace).** An entry named `quarry` that fails the BR-2 test (other source, other repo, missing fields, directory source, fork) is foreign. Both commands refuse before running any step, change nothing, exit 1 (R1 copy).
- **BR-4 (children).** Each child runs with stdin `/dev/null`, no `-y`; stdout captured on its own (what `--json` lists are decoded from) and stdout+stderr captured together in arrival order (what is replayed); the combined buffer is replayed to quarry's stderr only when that child fails (trailing `\n` added if missing); on success dropped. A child is killed when quarry's context is cancelled (SIGINT/SIGTERM via existing `signal.NotifyContext`, `cmd/quarry/run.go:42`); `cmd.WaitDelay` set so a node launcher's grandchildren cannot hang `Wait`.
- **BR-5 (order).** install: LookPath(`claude`) → both lists → BR-3 check → marketplace step → plugin step → stdout restart line → stderr R2 hint → stderr quarry-PATH warning. uninstall: LookPath(`claude`) → both lists → BR-3 check → plugin step → marketplace step (or Kept) → stdout restart line → stderr R6 hints. Within each subcommand cobra `Args` (no-args refusal) runs before `--json` refusal, before LookPath, before any child.
- **BR-6 (keep marketplace).** uninstall does not run `marketplace remove` while any `quarry@quarry` entry with scope other than `user` remains; prints Kept line and one R6 hint per such entry in list order.
- **BR-7 (restart line).** `Restart Claude Code to load it.` / `... to unload it.` printed only when at least one `claude` add/install/uninstall/remove step ran. Skips and Kept do not count.
- **BR-8 (no store).** `uninstall`/`install` take no store, snapshots or config factory; "Your quarry store and snapshots are not touched" is true by construction (architect cites the wiring line).
- **BR-9 (plugin unchanged).** `plugin/.claude-plugin/plugin.json` keeps bare `quarry` + `["mcp"]` so a user-scope `claude mcp add` entry dedupes with the plugin's by endpoint.
- **BR-10 (tests).** Every test uses a fake runner / fake LookPath; the real exec adapter is tested with a helper-process pattern re-running the test binary. `testEnv` (`cmd/quarry/main_test.go:27-31`) must get fake `RunTool`/`LookPath` defaults the moment the real adapter is wired.

---

## Triage Brief

- **`mcp` command**: internal/cli/mcp.go:19-56 `newMCPCommand(serve MCPServeFunc, isTerminal TerminalProbe, jsonOut *bool)`; no local flags; `Args: noArgs` (errors.go:16, exit 2); refuses `--json` via `UsageError{mcpJSONRefusal}` (:11, :40-42). Long :25-40 says "Add it to your client's MCP config with the command "quarry" and the argument "mcp"." — the manual step this feature replaces in Claude Code.
- **Wiring**: internal/cli/root.go:43 `root.AddCommand(newMCPCommand(env.ServeMCP, env.IsTerminal, jsonOut))`; persistent `--json` root.go:24; root doc comment :6-10 lists commands. `cli.Env` internal/cli/run.go:46-57 (Stdin/Stdout/Stderr, factories, LoadConfig, ServeMCP, IsTerminal, Now) — **no seam for exec, LookPath or home**. cmd/quarry/run.go:139-155 `newMCPServe`, :174-184 `resolveHome`, :186-200 `defaultEnv`, :202-231 exit mapping (nil→0, `cli.UsageError`→2, `ReportedError`→1 silent, else 1 printed `quarry: <err>`). `signal.NotifyContext` at run.go:42.
- **Plugin route (static)**: plugin/.claude-plugin/plugin.json:6 `"mcpServers": {"quarry": {"command":"quarry","args":["mcp"]}}`; `.claude-plugin/marketplace.json` at repo root names marketplace `quarry`; README.md:14-19 tells users `claude plugin marketplace add koblas/quarry` + `claude plugin install quarry@quarry`; README:23 says quarry's only network request is the exchange-rate fetch. phase3d-skill spec:26 "no new CLI command" — superseded by this spec.
- **Settings writer / exec / install precedent**: none. No code reads/writes JSON settings, `~/.claude/**`, or runs a child process. No install/setup/init command. Nearest precedents: `snapshots prune` subcommand shape and its no-args refusal (internal/cli/snapshots_prune.go:44); `UsageError` exact-text refusals (internal/cli/errors.go:5-25); `homepath.Abbreviate` (internal/platform/homepath) for `~`; `%q` for machine paths (internal/report/document/holdings.go warnings).
- **`claude` 2.1.285** at `/opt/homebrew/bin/claude`: `plugin install|i [-s scope] <plugin>` (default user), `plugin uninstall|remove [-s scope] <plugin>`, `plugin marketplace add [--scope] <source>`, `plugin marketplace remove|rm [--scope] <name>` (omitted scope removes from every scope — quarry always passes `--scope user`), `plugin list --json`, `plugin marketplace list --json`, `plugin enable <plugin>`.
- **Observed JSON shapes** (captured 2026-10-07; test-fixture truth):
  - marketplace list: `[{"name":"caveman","source":"github","repo":"JuliusBrussee/caveman","installLocation":"/Users/x/.claude/plugins/marketplaces/caveman"}]`
  - plugin list: `[{"id":"gopls-lsp@claude-plugins-official","version":"1.0.0","scope":"user","enabled":true,"installPath":"...","installedAt":"...","lastUpdated":"...","projectEnabled":true}, {"id":"...","scope":"project","enabled":false,"projectPath":"/Users/x/repos/foo",...}]` — same `id` may repeat, one entry per project.
- **Callers** (shape-changing symbols): `newMCPCommand` ← root.go:43 (LSP). `MCPServeFunc` ← run.go:53, cmd/quarry/run.go:141, mcp.go:19, mcp_test.go:96 (LSP). `Env.ServeMCP` ← cmd/quarry/run.go:199, root.go:43 (grep). `Env` struct ← `defaultEnv` cmd/quarry/run.go:186-200, `testEnv` cmd/quarry/main_test.go:27-35 (grep).
- **Pins that change**: root help byte-for-byte cmd/quarry/run_status_test.go:145-162; positional-arg rows run_usage_test.go:217-220, unknown-command form :225, unknown-flag rows :323, `--help` rows :391; run_read_usage_test.go:53-54; `mcp` Long pin internal/cli/mcp_test.go:28,55; README section const cmd/quarry/run_plugin_readme_test.go:41-62; manifest pins run_plugin_manifest_test.go:31-67; skill-drift command tree run_skill_drift_test.go:242 (recognises new commands automatically).

**Already exists — do not re-plan:** `UsageError`/`noArgs` (errors.go), exit mapping (run.go:202-231), `signal.NotifyContext` ctx cancel (run.go:42), `homepath.Abbreviate`, `snapshots prune` subcommand/no-args precedent, plugin manifest + its tests, `mcp` `--json` refusal pattern.

**Becomes dead:** only the old `mcp` Long sentence. Nothing loses a caller.

## Product Verdict

Pass 1 on `mcp --install/--uninstall`: **DON'T BUILD** (writing `~/.claude.json` races Claude Code; wrapper around one pasteable line). User overrode with reshape to a separate verb. Pass 2: **SHIP WITH CHANGES**, accepted: (1) group named `claude` (rejected `ai` vague, `setup` reads as first run, `client`/`integrate` abstract, `plugin` reads as into quarry, `mcp install` names lesser half); (2) `install` installs the plugin (skill + MCP), not a bare MCP entry; (3) idempotent via JSON list reads; (4) README network sentence + PRD Decisions line amended in-feature. Scoped copy ruling after sizing: **SHIP WITH CHANGES**, accepted: R1 foreign-marketplace identity and refusals; explicit unknown-subcommand refusal on the group (cobra default exits 0); R4 one "cannot read" form per list; R2/R6/R7 hints; root-help row.

Architect step 0 (before SCENARIO-01's plan), record only, do not rule: in a throwaway `HOME=$TMPDIR/h` observe `claude plugin list --json` and `claude plugin marketplace list --json` on an empty home; `claude plugin enable --help` default scope (R2 hint assumes user default; if not, R2 line becomes `claude plugin enable --scope user quarry@quarry`); shape of a local-scope plugin entry. (Orchestrator's attempt to read `enable --help` was blocked by a hook.)

## Surface & Copy

### Commands and help (render verbatim)

**`quarry claude`** — group; runnable only to print help / refuse unknown names.
- Short: `Install quarry's plugin in Claude Code, or remove it`
- Long:
```
Install quarry's plugin in Claude Code, or remove it. quarry does this by
running the claude command, so claude must be on your PATH.
```
- `quarry claude` alone, and `quarry claude --json` alone: group help on stdout, exit 0 (mirrors root).
- `quarry claude bogus`: stderr `quarry: unknown command "bogus" for "quarry claude"; Run 'quarry claude --help' for usage.`, exit 2. Built as an `Args` check on the group (same form as root's pinned line run_usage_test.go:225); cobra's default would print help and exit 0.
- Root help (`cmd/quarry/run_status_test.go:145-162`) gains, between `cashflow` and `findings`, exactly: `"  claude      Install quarry's plugin in Claude Code, or remove it\n"+` (name column 12 wide).
- `newRootCommand` doc comment (root.go:6-10) gains `claude (with its install and uninstall children)`.

**`quarry claude install`** — no flags, no args, no Example.
- Short: `Install quarry's plugin (skill and MCP server) in Claude Code`
- Long:
```
Install quarry's Claude Code plugin for all your projects. The plugin adds
the quarry skill, which teaches Claude to answer from quarry, and quarry's
MCP server. install runs:

  claude plugin marketplace add --scope user koblas/quarry
  claude plugin install --scope user quarry@quarry

skipping each step that is already done, so running it again is safe.
Claude Code downloads the plugin from github.com/koblas/quarry; quarry
itself sends nothing and opens none of Claude Code's files. The plugin
starts "quarry" from your PATH. Restart Claude Code to load it.
```

**`quarry claude uninstall`** — no flags, no args, no Example.
- Short: `Uninstall quarry's plugin from Claude Code`
- Long:
```
Uninstall quarry's Claude Code plugin from your user scope and remove the
quarry marketplace. uninstall runs:

  claude plugin uninstall --scope user quarry@quarry
  claude plugin marketplace remove --scope user quarry

skipping each step with nothing to remove. A copy of the plugin installed
for a single project stays, and uninstall names each one. Your quarry
store and snapshots are not touched.
```

### Child argv (literal; also the `<command>` strings in R5)
- `claude plugin marketplace list --json`
- `claude plugin list --json`
- `claude plugin marketplace add --scope user koblas/quarry`
- `claude plugin install --scope user quarry@quarry`
- `claude plugin uninstall --scope user quarry@quarry`
- `claude plugin marketplace remove --scope user quarry`

### stdout lines (one per step, step order)

| Step | Did it | Skipped |
|---|---|---|
| install, marketplace | `Added the quarry marketplace to Claude Code.` | `The quarry marketplace is already in Claude Code.` |
| install, plugin | `Installed the quarry plugin (skill and MCP server) for all your projects.` | `The quarry plugin is already installed for all your projects.` |
| uninstall, plugin | `Uninstalled the quarry plugin from Claude Code.` | `The quarry plugin is not installed for all your projects.` |
| uninstall, marketplace | `Removed the quarry marketplace from Claude Code.` | `The quarry marketplace is not in Claude Code.` |
| uninstall, marketplace kept (BR-6) | `Kept the quarry marketplace: the quarry plugin is still installed elsewhere and needs it.` | — |

Trailing line (BR-7): install `Restart Claude Code to load it.`; uninstall `Restart Claude Code to unload it.`; none when no step ran.

Composition examples (R8): plugin absent + marketplace ours → `not installed` / `Removed` / restart. Plugin present + marketplace absent → `Uninstalled` / `not in Claude Code` / restart. Project copies only → `not installed` / `Kept` / R6 hints on stderr / no restart / exit 0. Install with user copy present, marketplace absent → `Added` / `already installed` / restart. Marketplace absent + project copies only → `not installed` / `not in Claude Code` (no Kept line: nothing to keep) / R6 hints / no restart (orchestrator, derived from R8 per-step composition, 2026-10-07).

### stderr hints and warnings (exit stays 0; printed after stdout lines)

- **R2 disabled user copy** (install; user-scope `quarry@quarry` with `enabled:false`; "already" line still printed; no restart line): `quarry: claude install: the quarry plugin is installed but turned off; to turn it on, run claude plugin enable quarry@quarry`
- **quarry-PATH warning** (install; `LookPath("quarry")` returns any error; after R2): `quarry: claude install: warning: the plugin starts "quarry" from your PATH, and your PATH has none; add the directory holding quarry to your PATH`
- **R6 remaining copies** (uninstall; one line per `quarry@quarry` entry with scope ≠ `user`, in list order; `<scope>` verbatim from entry):
  - with `projectPath`: `quarry: claude uninstall: the quarry plugin is still installed for project %q; to remove it, run claude plugin uninstall --scope <scope> quarry@quarry in that directory`
  - without: `quarry: claude uninstall: the quarry plugin is still installed for a project Claude Code did not name; to remove it, run claude plugin uninstall --scope <scope> quarry@quarry in that project's directory`
  - scope other than `project` or `local` (e.g. `managed`; ignores `projectPath`; `%q` wraps the scope verbatim): `quarry: claude uninstall: the quarry plugin is still installed at scope %q, which claude plugin uninstall cannot remove from; it stays until whoever manages that scope removes it`
- **R7 path form** for `%q` here and in the cannot-run line: `$HOME` set and path == `$HOME` → `~`; path starts with `$HOME/` → `~/...`; otherwise (incl. `$HOME` unset/empty) raw as `claude` printed. `%q` wraps the abbreviated text: `"~/repos/foo"`.
- **R3**: only project/local entries on install → install user copy normally, no hint.

### Refusals and failures (stdout holds only step lines already printed; exit as shown)

| Outcome | stderr | exit |
|---|---|---|
| no `claude` on PATH, install (any `LookPath` error: not found, `ErrDot`, non-executable) | `quarry: claude install: cannot find the claude command on your PATH; install Claude Code, then run quarry claude install again` | 1 |
| no `claude` on PATH, uninstall | `quarry: claude uninstall: cannot find the claude command on your PATH; add it to your PATH, then run quarry claude uninstall again` | 1 |
| R1 foreign `quarry` marketplace, install (before any step) | `quarry: claude install: Claude Code has a marketplace named "quarry" that is not koblas/quarry on GitHub; remove it with claude plugin marketplace remove quarry, then run quarry claude install again` | 1 |
| R1 foreign, uninstall | `quarry: claude uninstall: Claude Code has a marketplace named "quarry" that is not koblas/quarry on GitHub, so quarry leaves it and its plugin alone; to remove them, run claude plugin uninstall quarry@quarry, then claude plugin marketplace remove quarry` | 1 |
| R4 unreadable marketplace list (not JSON / not array / entry without string `name`) | `quarry: claude install: cannot read what claude plugin marketplace list --json printed; update Claude Code, or run the two commands in quarry claude install --help yourself` (uninstall: `quarry: claude uninstall: ... quarry claude uninstall --help yourself`) | 1 |
| R4 unreadable plugin list (entry without string `id` or `scope`) | same with `claude plugin list --json` | 1 |
| list exits non-zero | captured buffer replayed, then `quarry: claude install: claude plugin marketplace list --json exited with status N; see its message above` (or `claude plugin list --json`; uninstall prefix likewise) | 1 |
| first step fails | captured buffer, then `quarry: claude install: claude plugin marketplace add --scope user koblas/quarry exited with status N; see its message above` (uninstall: `... claude plugin uninstall --scope user quarry@quarry exited ...`) | 1 |
| second step fails after first ran this time (partial) | captured buffer, then `quarry: claude install: added the quarry marketplace, but claude plugin install --scope user quarry@quarry exited with status N; see its message above, then run quarry claude install again` (uninstall: `quarry: claude uninstall: uninstalled the quarry plugin, but claude plugin marketplace remove --scope user quarry exited with status N; see its message above, then run quarry claude uninstall again`) | 1 |
| second step fails, first was skipped | first-step form naming the second command | 1 |
| empty captured buffer | drop `; see its message above` (partial: `... exited with status 1, then run quarry claude install again`) | 1 |
| child stopped by a signal quarry did not send (ctx not done) | `exited with status N` → `was stopped by signal <name>` (`<name>` = signal `String()`, e.g. `killed`); same partial/empty variants | 1 |
| R5 interrupted (ctx cancelled: during a list, between children, before first child, or mid-step; running/next-due child killed) | `quarry: claude install: stopped before <command> finished; run quarry claude install again` (`<command>` = literal argv string of the running or next-due command; uninstall form likewise) | 1 |
| `claude` found but exec fails to start (e.g. permission denied) | `quarry: claude install: cannot run claude at %q (<os error>); check that it is Claude Code and that you can run it, then run quarry claude install again` (`%q` = LookPath result with R7; `<os error>` innermost text; partial prefix `added the quarry marketplace, but cannot run claude at ...` / `uninstalled the quarry plugin, but cannot run claude at ...` when a step already ran) | 1 |
| `--json` | `quarry: claude install prints no JSON; drop --json` / `quarry: claude uninstall prints no JSON; drop --json` | 2 |
| any argument | `quarry: install takes no arguments; Run 'quarry claude install --help' for usage.` / `quarry: uninstall takes no arguments; Run 'quarry claude uninstall --help' for usage.` | 2 |

Deliberately no row: `$HOME` unset (quarry reads nothing under home; `claude`'s own failure lands in a step-failure row); user-scope MCP-only `quarry` entry from `claude mcp add` (not ours; Claude Code dedupes by endpoint); disabled plugin on uninstall (`enabled` ignored).

### Changes to existing surfaces

- **`internal/cli/mcp.go:25-28` `mcp` Long**, first paragraph becomes (rest unchanged):
```
Run quarry as a local MCP server for Claude and other MCP clients. The
client starts it and talks to it over stdin and stdout; quarry opens no
network port.

In Claude Code, run quarry claude install: it installs quarry's plugin,
which adds this server and the quarry skill. In other MCP clients, add a
server with the command "quarry" and the argument "mcp". An app started
outside a terminal may not find quarry on your PATH; give it the full
path that "command -v quarry" prints.
```
- **README.md:14-19** install block becomes a fenced `quarry claude install` followed by: `This runs claude plugin marketplace add koblas/quarry and claude plugin install quarry@quarry for you; you can run those two yourself instead.`
- **README.md:23** first sentence becomes: `quarry's only network request is the exchange-rate fetch during quarry sync, which carries nothing but dates, back to the date of your earliest transaction; quarry claude install has Claude Code download the plugin from GitHub, and quarry itself sends nothing.`
- **README.md new section** `## Install or remove the plugin` (after "Use quarry with Claude Code" content, before "Run a monthly summary") documenting `quarry claude install` (what it runs, re-run safe, restart Claude Code), `quarry claude uninstall` (what it runs, project copies stay and are named, store untouched), and the MCP-only alternative `claude mcp add --scope user quarry -- quarry mcp`. Exact prose is the developer's, pinned byte-for-byte by the README const in `cmd/quarry/run_plugin_readme_test.go`; command lines in it must equal the Long's.
- **`docs/initial-prd.md` Decisions** (after :326) new line: `quarry claude install / uninstall install and remove the Claude Code plugin by running the claude command (user scope); quarry never edits Claude Code's files, and the download is Claude Code's, not quarry's.`
- **Drift pin**: `install` Long's `koblas/quarry` and `quarry@quarry` must equal `.claude-plugin/marketplace.json` name/owner and `plugin/.claude-plugin/plugin.json` name, and README's lines (precedent run_plugin_manifest_test.go:31-67).
- Pins to re-assert: run_status_test.go:145-162; run_usage_test.go:217-220,225,323,391; run_read_usage_test.go:53-54; mcp_test.go:28,55; run_plugin_readme_test.go:41-62.

### Seams (binding for architect)
- `internal/claudeplugin` feature package: `Server` with `Install(ctx, out io.Writer, errOut io.Writer)`-shaped API (architect fixes signature), options `WithRunner`, `WithLookPath`, `WithHome`. `Runner func(ctx context.Context, name string, args ...string) (stdout, combined []byte, exitCode int, err error)` (gate R1 ruling: lists decode `stdout`; replay and `ExitError.Output` use `combined`) — `err` only when the child failed to start or ctx cancelled; non-zero exit ⇒ `err == nil`. `LookPath func(file string) (string, error)`. One shared `readState(ctx)` (marketplace list then plugin list); one shared step runner owning failure classification. Never imports `internal/cli`.
- `internal/platform/toolrun`: real adapter (`os/exec`, stdin `/dev/null`, combined buffer, `CommandContext` + `WaitDelay`, signal detection), helper-process tests only.
- `internal/cli`: `claude.go` (group + unknown-subcommand `Args`), `claude_install.go`, `claude_uninstall.go`, `render_claude.go`; `Env` gains `RunTool`, `LookPath`, and a home value for R7. `cmd/quarry/run.go` `defaultEnv` wires `toolrun.Run`, `exec.LookPath`, `os.UserHomeDir`; `testEnv` gets fakes.
- Until SCENARIO-04 wires the real adapter, `defaultEnv.RunTool` is nil: SCENARIO-01 lists it under Left unbuilt and its cmd/quarry tests reach only refusals that fire before the runner.

---

## Scenarios (Gherkin)

```gherkin
Scenario: SCENARIO-01 — Fresh install runs both steps
  Given claude is on PATH and its lists show no quarry marketplace and no quarry plugin
  When the user runs `quarry claude install`
  Then quarry runs `claude plugin marketplace add --scope user koblas/quarry` then `claude plugin install --scope user quarry@quarry`, stdout holds the two "did" lines and `Restart Claude Code to load it.`, stderr is empty, and the process exits 0

Scenario: SCENARIO-02 — Install when both are already present
  Given the lists show our marketplace and a user-scope quarry@quarry
  When the user runs `quarry claude install`
  Then no add or install runs, stdout holds both "already" lines and no restart line, and the process exits 0

Scenario: SCENARIO-03 — Install when only the marketplace is present
  Given the lists show our marketplace and no user-scope quarry@quarry
  When the user runs `quarry claude install`
  Then only the plugin install runs, stdout holds the "already in" then "Installed" lines then the restart line, and the process exits 0

Scenario: SCENARIO-04 — Install warns when quarry is not on PATH
  Given install succeeds and LookPath("quarry") fails
  When the user runs `quarry claude install`
  Then the quarry-PATH warning is on stderr after the stdout lines and the process exits 0

Scenario: SCENARIO-05 — Install refuses when claude is not on PATH
  Given LookPath("claude") fails
  When the user runs `quarry claude install`
  Then the install not-found refusal is on stderr, no child runs, stdout is empty, and the process exits 1

Scenario: SCENARIO-05b — Uninstall refuses when claude is not on PATH
  Given LookPath("claude") fails
  When the user runs `quarry claude uninstall`
  Then the uninstall not-found refusal is on stderr and the process exits 1

Scenario: SCENARIO-06 — First install step fails
  Given the marketplace add exits 1 with a message
  When the user runs `quarry claude install`
  Then stderr holds the captured child output then the first-step line, stdout is empty, and the process exits 1

Scenario: SCENARIO-07 — Second install step fails after the first ran
  Given the marketplace add succeeds and the plugin install exits 1
  When the user runs `quarry claude install`
  Then stdout holds the "Added" line, stderr holds the captured output then the partial line, and the process exits 1

Scenario: SCENARIO-08 — Re-run after a partial install finishes the job
  Given the state SCENARIO-07 left (marketplace present, plugin absent) and a working claude
  When the user runs `quarry claude install` again
  Then the marketplace step is skipped, the plugin installs, and the process exits 0

Scenario: SCENARIO-09 — Unreadable plugin list
  Given `claude plugin list --json` prints something that is not JSON
  When the user runs `quarry claude install`
  Then the plugin-list cannot-read refusal is on stderr, nothing is replayed, and the process exits 1

Scenario: SCENARIO-09b — List command exits non-zero
  Given `claude plugin marketplace list --json` exits 1 with a message
  When the user runs `quarry claude install`
  Then stderr holds the captured output then the list exited-with-status line, and the process exits 1

Scenario: SCENARIO-10 — Uninstall runs both steps
  Given the lists show our marketplace and only a user-scope quarry@quarry
  When the user runs `quarry claude uninstall`
  Then quarry runs `claude plugin uninstall --scope user quarry@quarry` then `claude plugin marketplace remove --scope user quarry`, stdout holds both "did" lines and `Restart Claude Code to unload it.`, and the process exits 0

Scenario: SCENARIO-11 — Uninstall when nothing is installed
  Given the lists show no quarry marketplace and no quarry@quarry
  When the user runs `quarry claude uninstall`
  Then no remove runs, stdout holds both "not" lines and no restart line, and the process exits 0

Scenario: SCENARIO-12 — Uninstall keeps the marketplace for a project copy
  Given a user-scope quarry@quarry and a project-scope quarry@quarry with projectPath under $HOME
  When the user runs `quarry claude uninstall`
  Then the user copy is uninstalled, the marketplace remove does not run, stdout holds "Uninstalled" then the Kept line then `Restart Claude Code to unload it.`, stderr holds one R6 hint naming the project via %q with ~, and the process exits 0

Scenario: SCENARIO-13 — Second uninstall step fails
  Given the plugin uninstall succeeds and the marketplace remove exits 1
  When the user runs `quarry claude uninstall`
  Then stdout holds "Uninstalled", stderr holds the uninstall partial line, and the process exits 1

Scenario: SCENARIO-14 — Interrupted mid-step
  Given the plugin install child is running
  When quarry's context is cancelled during `quarry claude install`
  Then the child is killed, stderr holds the R5 stopped line naming `claude plugin install --scope user quarry@quarry`, and the process exits 1

Scenario: SCENARIO-15 — install refuses --json
  When the user runs `quarry claude install --json`
  Then the install JSON refusal is on stderr, no child runs, and the process exits 2

Scenario: SCENARIO-15b — uninstall refuses --json
  When the user runs `quarry claude uninstall --json`
  Then the uninstall JSON refusal is on stderr and the process exits 2

Scenario: SCENARIO-16 — install refuses arguments
  When the user runs `quarry claude install extra`
  Then the no-arguments refusal is on stderr and the process exits 2

Scenario: SCENARIO-17 — mcp help points at quarry claude install
  When the user runs `quarry mcp --help`
  Then stdout holds the ruled Long paragraph naming `quarry claude install`, and the drift test ties install's Long command lines to marketplace.json, plugin.json and README

Scenario: SCENARIO-18 — README documents install and uninstall
  Given README.md
  When the README pin test runs
  Then README holds the `quarry claude install` block, the amended network sentence and the new install/remove section byte-for-byte, with command lines equal to the Longs

Scenario: SCENARIO-19 — Install refuses a foreign quarry marketplace
  Given a marketplace named quarry with source "directory"
  When the user runs `quarry claude install`
  Then the R1 install refusal is on stderr, no step runs, and the process exits 1

Scenario: SCENARIO-20 — Uninstall refuses a foreign quarry marketplace
  Given a marketplace named quarry whose repo is not koblas/quarry
  When the user runs `quarry claude uninstall`
  Then the R1 uninstall refusal is on stderr, nothing is removed, and the process exits 1

Scenario: SCENARIO-21 — Install hints when the user copy is disabled
  Given our marketplace and a user-scope quarry@quarry with enabled false
  When the user runs `quarry claude install`
  Then stdout holds both "already" lines with no restart line, stderr holds the R2 hint, and the process exits 0

Scenario: SCENARIO-22 — Plugin list entry without id is unreadable
  Given a `claude plugin list --json` entry that lacks id
  When the user runs `quarry claude install`
  Then the plugin-list cannot-read refusal is on stderr and the process exits 1

Scenario: SCENARIO-23 — Failed step with empty output
  Given the marketplace add exits 1 printing nothing
  When the user runs `quarry claude install`
  Then the step line ends `exited with status 1` with no "see its message above", and the process exits 1

Scenario: SCENARIO-24 — Child killed by a signal quarry did not send
  Given the plugin install is killed by SIGKILL while quarry's context is live
  When the user runs `quarry claude install`
  Then the step line reads `was stopped by signal killed` and the process exits 1

Scenario: SCENARIO-25 — claude resolves but cannot start
  Given LookPath("claude") returns a path under $HOME and exec fails with permission denied
  When the user runs `quarry claude install`
  Then the cannot-run line with a ~ path and the OS reason is on stderr and the process exits 1

Scenario: SCENARIO-26 — Hint path outside home is not abbreviated
  Given HOME is unset and a project-scope quarry@quarry has projectPath "/srv/x"
  When the user runs `quarry claude uninstall`
  Then the R6 hint shows "/srv/x" unabbreviated

Scenario: SCENARIO-27 — Unknown claude subcommand
  When the user runs `quarry claude bogus`
  Then stderr holds `quarry: unknown command "bogus" for "quarry claude"; Run 'quarry claude --help' for usage.` and the process exits 2

Scenario: SCENARIO-27b — Bare claude group prints help
  When the user runs `quarry claude`
  Then the group help is on stdout and the process exits 0

Scenario: SCENARIO-28 — Root help lists claude
  When the user runs `quarry --help`
  Then the claude row appears between cashflow and findings exactly as ruled
```

---

## Sizing

Sizing pass (architect, 2026-10-07): no mandatory test-first code (quarry writes no files); every unit code-first. Twin check against phase3a SCENARIO-01 (1,585k IE, mid-range) — no SPLIT.

| Scenario | Verdict — numbers |
| --- | --- |
| SCENARIO-01 | OWNS A RUN — 3 batches, 1 feature package (`internal/claudeplugin`): (1) shared state reader + decode/exit fault rows + R1 identity; (2) `Install` with skip branches + R2 hint; (3) cli `claude` group + `install`, `Env.RunTool`, root.go wiring, usage/help pins. Runs A \| B1 \| V. Folds SCENARIO-02, 03, 09, 09b, 15, 16, 19, 21, 22. |
| SCENARIO-02, 03, 09, 09b, 15, 16, 19, 21, 22 | FOLD → SCENARIO-01 |
| SCENARIO-04 | OWNS A RUN — 3 batches: (1) `internal/platform/toolrun` exec adapter, helper-process tests (capture, exit mapping, fail-to-start, stdin /dev/null, kill on cancel, WaitDelay); (2) LookPath port in claudeplugin (claude before any child, quarry after success) + warning/refusal copy + cannot-run line; (3) `defaultEnv` wires `RunTool`/`LookPath`, `testEnv` fakes, wiring pin. Folds SCENARIO-05, 25. |
| SCENARIO-05, 25 | FOLD → SCENARIO-04 |
| SCENARIO-06 | OWNS A RUN — 2 batches: (1) step-failure error carrying steps done, captured output, classification (non-zero / signal / ctx cancelled naming running-or-next command); (2) cli failure renderer shared with uninstall. Folds SCENARIO-07, 08, 14, 23, 24. |
| SCENARIO-07, 08, 14, 23, 24 | FOLD → SCENARIO-06 |
| SCENARIO-10 | OWNS A RUN — 2 batches: (1) `Uninstall` on shared reader + step runner, not-present skips, step failures, R1 uninstall refusal; (2) cli `uninstall` subcommand, guards, usage rows, help pins. Folds SCENARIO-11, 13, 05b, 15b, 20. |
| SCENARIO-11, 13, 05b, 15b, 20 | FOLD → SCENARIO-10 |
| SCENARIO-12 | OWNS A RUN — 2 batches: (1) keep-marketplace rule over non-user entries, mutation check on the guard; (2) Kept line + R6 hints in list order. Folds SCENARIO-26. (Home batch dead: `Env.Home`/`claudePath` landed in 04.) |
| SCENARIO-26 | FOLD → SCENARIO-12 |
| SCENARIO-17 | OWNS A RUN — 3 batches, 0 feature packages (sonnet): (1) `mcp` Long + mcp_test pin; (2) README block, network sentence, new section, README const pin; (3) drift test (Long ↔ marketplace.json, plugin.json, README), PRD Decisions line, group unknown-subcommand refusal, root-help row + pin. Folds SCENARIO-18, 27, 27b, 28. |
| SCENARIO-18, 27, 27b, 28 | FOLD → SCENARIO-17 |

## BDD Acceptance Progress
- [x] SCENARIO-01: Fresh install runs both steps (folds 02 `Test_claude_install_skips_both_steps_when_both_are_present`, 03 `Test_claude_install_installs_only_the_plugin_when_the_marketplace_is_present`, 09 `Test_claude_install_refuses_a_plugin_list_that_is_not_json`, 09b `Test_claude_install_replays_a_failed_marketplace_list`, 15 `Test_claude_install_refuses_json`, 16 `Test_claude_install_refuses_arguments`, 19 `Test_claude_install_refuses_a_foreign_quarry_marketplace`, 21 `Test_claude_install_hints_when_the_user_copy_is_turned_off`, 22 `Test_claude_install_refuses_a_plugin_list_entry_without_an_id`) — `internal/cli/claude_install_test.go` `Test_claude_install_runs_both_steps_when_nothing_is_installed`
- [x] SCENARIO-04: Install warns when quarry is not on PATH (folds 05 `Test_claude_install_refuses_when_claude_is_not_on_the_path`, 25 `Test_claude_install_reports_a_claude_it_cannot_run`) — `internal/cli/claude_install_test.go` `Test_claude_install_warns_when_quarry_is_not_on_the_path`
- [x] SCENARIO-06: First install step fails (first-step row green on arrival `Test_claude_install_reports_a_failed_first_step`; folds 07 (acceptance test below), 08 `Test_claude_install_finishes_on_a_rerun_after_a_partial_install`, 14 `Test_claude_install_reports_an_interrupt_while_a_step_runs`, 23 `Test_claude_install_drops_see_above_when_the_failed_step_printed_nothing`, 24 `Test_claude_install_reports_a_step_stopped_by_a_signal`) — `internal/cli/claude_install_test.go` `Test_claude_install_reports_a_partial_install_when_the_plugin_step_fails`
- [x] SCENARIO-10: Uninstall runs both steps (folds 11 `Test_claude_uninstall_skips_both_steps_when_nothing_is_installed`, 13 `Test_claude_uninstall_reports_a_partial_uninstall_when_the_marketplace_step_fails`, 05b `Test_claude_uninstall_refuses_when_claude_is_not_on_the_path`, 15b `Test_claude_uninstall_refuses_json`, 20 `Test_claude_uninstall_refuses_a_foreign_quarry_marketplace`) — `internal/cli/claude_uninstall_test.go` `Test_claude_uninstall_runs_both_steps_when_both_are_present`
- [x] SCENARIO-12: Uninstall keeps the marketplace for a project copy (folds 26 `Test_claude_uninstall_shows_a_project_path_outside_home_unabbreviated`) — `internal/cli/claude_uninstall_test.go` `Test_claude_uninstall_keeps_the_marketplace_for_a_project_copy`
- [x] SCENARIO-17: mcp help points at quarry claude install (folds 18 `Test_readme_section_for_installing_and_removing_the_plugin_is_verbatim`, 27 `Test_claude_refuses_an_unknown_subcommand`, 27b `Test_claude_prints_its_group_help_on_stdout`, 28 `Test_run_help_prints_quarrys_description` (green on arrival)) — `internal/cli/mcp_test.go` `Test_mcp_help_prints_the_ruled_long_text`
