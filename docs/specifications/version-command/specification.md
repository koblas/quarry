# Specification: quarry version

<!-- spec-check: v1 -->

## Intent & Goal

**Primary Goal**: A user (or a script, or Claude through the skill) can ask a `quarry` binary what version it is — the release tag for a `go install …@vX.Y.Z` install, the commit (and whether the tree was dirty) for a build from a checkout — with `quarry version` or `quarry --version`, and the MCP server and the store record that same string. Paired with release-please (CI config, outside this pipeline) so releases get real tags starting at v1.0.0.

**Out of Scope**: release binaries, Homebrew tap, ldflags stamping; an MCP tool for the version (MCP clients get it in `initialize` serverInfo); `-v` shorthand; any change to `status` text output; Go-version/platform in the text line; deleting the now-unreached `(devel)` fallbacks in `internal/mcp/server.go:18` and `internal/store/duckstore/duckstore.go:28` (behaviour-neutral cleanup done outside the pipeline — touching those packages would split the feature).

**Business Rules**: see below.

## Business Rules & Invariants
- **BR-V1 One owner.** `internal/version.Resolve(*debug.BuildInfo)` is the only place the version rule lives. `quarry version`, `quarry --version`, MCP `Implementation.Version` and `store_info.quarry_version` all take its string.
- **BR-V2 The rule.** Version = `Main.Version` when non-empty and not `(devel)`; else `(devel) <rev>` where `<rev>` is `vcs.revision` cut to 12 characters (whole revision if shorter), plus `+dirty` only when `vcs.modified` is literally `true`; else `(devel)`. Intent: show every fact the build recorded and never claim one it didn't.
- **BR-V3 JSON facts.** `commit` = `vcs.revision` verbatim (untruncated) or null when absent; `modified` = true/false only for literal `"true"`/`"false"`, otherwise null (absent or malformed is unknown, not false); `go` = `runtime.Version()` always, nil build info included. A tagged build carrying vcs info fills `commit`/`modified` while text stays the tag.
- **BR-V4 Self-contained.** `version` reads only the binary: no store, no config, no HOME, no Quicken.
- **BR-V5 Root-only flag.** `--version` is a local root flag; it does not exist on subcommands. Root keeps `Args` nil so cobra's legacy args check still refuses unknown commands.

## Decisions
- First release 1.0.0; plugin version bumped in step (release-please `extra-files`). Tags `vX.Y.Z` with no component prefix.
- **OPEN (user):** a breaking-change commit makes release-please cut 2.0.0, and `go install …@v2.0.0` needs a `/v2` module path. Not needed to build this feature.
- Go 1.24+ stamps checkout builds with a pseudo-version (`v0.0.0-<time>-<rev12>[+dirty]`), so printing `Main.Version` verbatim already meets "commit for checkouts"; the `(devel) <rev>` arm covers builds that still report `(devel)`.

---

## Triage Brief

- Registry `internal/cli/root.go:12-47` (doc :7-11, "No Args or Run field" comment :13-14, persistent `--json` :27, `AddCommand` :29-45). `Env` seams `internal/cli/run.go:44-58`; `Execute` :68-92 (unknown flag → `UsageError` :91). Stdout write failure helper `internal/cli/output.go:55-58`.
- `cmd/quarry/run.go:88-93` `buildVersion(info)` returns `Main.Version` or "" for nil; used at :71 (`newServerFactory` → `duckstore.WithQuarryVersion`, reads `debug.ReadBuildInfo()` itself at :69) and :150 (`mcp.WithVersion`, `newMCPServe(info, …)` :144); `defaultEnv` :191-209 reads build info at :193. Tested `cmd/quarry/main_process_test.go:164-179` (nil/empty rows pin "").
- Fallback `(devel)` constants: `internal/mcp/server.go:18` (`WithVersion("")` keeps default :49-53, `Implementation` :94), `internal/store/duckstore/duckstore.go:28`.
- Store version surfaces: `store_info.quarry_version` (`internal/store/duckstore/schema.go:171`, read `status.go:17`) → `status --json` `.store.quarry_version` (`internal/report/document/status.go:13,29,112`); status text never renders it.
- Siblings: `internal/cli/mcp.go:15-50` (`noArgs`, Env seam; tests `cmd/quarry/run_mcp_test.go:190-219`, `startMCP`/`c.info` cases :126); `internal/cli/status.go:12-50` + `json_status.go` (`renderResult` + `emit`).
- Pins: root help exact block `cmd/quarry/run_status_test.go:113-139`; skill drift parser `cmd/quarry/run_skill_drift_test.go:78-95`; pins that see `(devel)` under `go test` (stay valid): `run_mcp_test.go:33`, `run_shared_documents_test.go:133`, `run_status_test.go:492`.
- `newServerFactory` callers (grep, confirm with LSP): `cmd/quarry/main_test.go:40`, `run_store_test.go:631`, `run_sync_lock_test.go:163`, `run_sync_rates_test.go:67,845`.
- Commit history is Conventional Commits (40/40 non-merge sampled); PRs merged with merge commits.

**Callers**
| Symbol | Caller | Via |
| --- | --- | --- |
| `newRootCommand` | `internal/cli/run.go:70` | grep |
| `cli.Execute` / `cli.Env` | `cmd/quarry/run.go` `defaultEnv` :191-209, `runProcess` | grep |
| `buildVersion` | `cmd/quarry/run.go:71,150`; `main_process_test.go:177` | grep |
| root Available Commands block | `cmd/quarry/run_status_test.go:126-143` | grep |
| `mcp.WithVersion` | `cmd/quarry/run.go:150`; `internal/mcp/server_test.go`, `cmd/quarry/run_mcp_test.go` | grep |

## Product Verdict

**SHIP WITH CHANGES** (product-vision, 2026-11-21): one version string with one owner; `quarry version` + root `--version` (no `-v`); no MCP tool; release tags unprefixed `vX.Y.Z`. Sizing-pass copy ruling (same day) settled revision/modified edge cases, `--version` combinations, regression pins and the 06/07 split.

## Surface & Copy

**`version` command** — `Use: version`, `Args: noArgs`. Short: `Show quarry's version`. No Example (deliberate; siblings `status`, `mcp` have none). Long, verbatim:
```
Print the version of this quarry binary: the release tag, such as v1.0.0,
for a binary installed with go install
github.com/koblas/quarry/cmd/quarry@<version>, or Go's pseudo-version for
one built from a checkout, which names the commit and ends in +dirty when
the checkout had uncommitted changes.

version reads only the binary itself: it needs no store, reads no config
and never looks at Quicken.

--json adds the full commit, whether the checkout had uncommitted changes,
and the Go version quarry was built with.
```
- Root flag help line: `--version   print quarry's version` (no shorthand).
- Root Available Commands gains, after `sync`: `  version     Show quarry's version`.
- Text output: `quarry <version>\n` on stdout, nothing on stderr, exit 0. Identical bytes when stdout is not a TTY.
- `--json` (key order fixed): `{"version": "<same string as text>", "commit": "<vcs.revision>" | null, "modified": true | false | null, "go": "<runtime.Version()>"}`. `quarry --version --json` and `quarry --json --version` print the same document.
- Stdout write failure, every arm (text, `--json`, `--version`): existing `quarry: cannot write the result to stdout: <write error>`, exit 1 (`internal/cli/output.go:55-58`).

**Edge rows**
| Input | Text | JSON version / commit / modified |
| --- | --- | --- |
| `go install …@v1.0.0` (no vcs) | `quarry v1.0.0` | `v1.0.0` / null / null |
| checkout clean | `quarry v0.0.0-20261121151146-260d0d29f940` | same / 40-char / false |
| checkout dirty | `quarry v0.0.0-20261121151146-260d0d29f940+dirty` | same / 40-char / true |
| `(devel)` + revision, modified=true | `quarry (devel) 260d0d29f940+dirty` | same / 40-char / true |
| `(devel)` + revision shorter than 12 | `quarry (devel) <whole rev>` | same / rev / per modified |
| `(devel)` + revision, modified absent or malformed | `quarry (devel) 260d0d29f940` | same / 40-char / null |
| empty `Main.Version` | as `(devel)` | as `(devel)` |
| `(devel)`, no vcs; or nil build info | `quarry (devel)` | `(devel)` / null / null |
| HOME unset, no store, no Quicken | as above, exit 0 | — |
| `(devel)` + `vcs.revision` = "" (any modified) | `quarry (devel)` | `(devel)` / null / per modified |
| `(devel)`, revision unknown, modified=true | `quarry (devel)` | `(devel)` / null / true |
| `(devel)` + duplicate `vcs.revision`, values differ | `quarry (devel)` | `(devel)` / null / per modified |
| `(devel)` + duplicate `vcs.revision`, values equal | as single revision | as single revision |
| duplicate `vcs.modified`, values differ | text as modified absent (no `+dirty`) | per revision / per revision / null |
| duplicate `vcs.modified`, values equal | as single value (equal `"TRUE"` is still malformed) | as single value |
| tagged `Main.Version` + duplicate/empty vcs keys | `quarry <tag>` | `<tag>` / per rules above / per rules above |

Duplicate keys are compared as raw strings; agreement counts as one value, any disagreement makes that key unknown. None of these is an error: `quarry version` exits 0. (Mid-feature ruling, product-vision 2026-11-21: for `vcs.revision`, "absent" in BR-V2/BR-V3 also covers an empty value and conflicting duplicates; for `vcs.modified`, "absent or malformed" also covers conflicting duplicates; `+dirty` is appended only to the `<rev>` arm. Pins affected: SCENARIO-01 `Resolve` table test gains these rows plus a first-wins/last-wins mutation arm; SCENARIO-02 JSON pins gain a `commit: null` empty-revision row.)

**Refusals (existing copy, stderr, exit 2)**
- `quarry version foo` → existing `noArgs` copy as siblings render it.
- `quarry --version sync`, `quarry sync --version` → `quarry: unknown flag: --version; Run 'quarry sync --help' for usage.`
- `quarry --version extra` → `quarry: unknown command "extra" for "quarry"; Run 'quarry --help' for usage.`
- `quarry version --version` → `quarry: unknown flag: --version; Run 'quarry version --help' for usage.`
- `quarry --version --help` → help, exit 0.

**Regression pins (SCENARIO-03):** bare `quarry` → stdout byte-identical to `quarry --help`, empty stderr, exit 0; `quarry --json` → same help bytes, exit 0 (not the version document); `quarry bogus` → `quarry: unknown command "bogus" for "quarry"; Run 'quarry --help' for usage.`, exit 2.

**Store/MCP:** `store_info.quarry_version`, `status --json` `.store.quarry_version`, MCP `sync_status` and MCP `initialize` `serverInfo.version` carry the BR-V2 string. Under `go test` (no vcs settings) they stay `(devel)`.

### Changes to existing surfaces
- `internal/cli/root.go:7-11` doc comment: add `version` and the root `--version` flag.
- `internal/cli/root.go:13-14` comment ("No Args or Run field"): now false — say root has a `RunE` for `--version` and keeps `Args` nil so unknown commands still refuse.
- `cmd/quarry/run_status_test.go:113-139`: insert `"  version     Show quarry's version\n"` after the `sync` line; assert the `--version` line in Flags.
- `cmd/quarry/main_process_test.go:164-179`: nil/empty-info rows change from "" to `(devel)`.
- `README.md:7-12` (docs, outside pipeline): add `go install github.com/koblas/quarry/cmd/quarry@latest` (needs Xcode Command Line Tools: quarry links DuckDB via cgo) above `command -v quarry`, and `quarry version` as the works-check.
- `plugin/skills/quarry/SKILL.md`: none; confirm skill drift test stays green.

---

## Scenarios (Gherkin)

```gherkin
Scenario Outline: SCENARIO-01 — quarry version prints the binary's version
  Given a quarry binary whose build info has version <main> and VCS info <vcs>
  When the user runs `quarry version`
  Then stdout is exactly "quarry <shown>" and exit is 0
  Examples:
    | main                                     | vcs                              | shown                                    |
    | v1.0.0                                   | none                             | v1.0.0                                   |
    | v0.0.0-20261121151146-260d0d29f940       | revision 260d0d29f940…, clean    | v0.0.0-20261121151146-260d0d29f940       |
    | v0.0.0-20261121151146-260d0d29f940+dirty | revision 260d0d29f940…, modified | v0.0.0-20261121151146-260d0d29f940+dirty |
    | (devel)                                  | revision 260d0d29f940…, modified | (devel) 260d0d29f940+dirty               |
    | (devel)                                  | none                             | (devel)                                  |
    | no build info                            | none                             | (devel)                                  |

Scenario: SCENARIO-02 — --json reports version, commit, modified and Go version
  Given a quarry binary built from a checkout with uncommitted changes
  When the user runs `quarry version --json`
  Then stdout is {"version": …, "commit": "<40-char revision>", "modified": true, "go": "<runtime.Version()>"} in that key order

Scenario: SCENARIO-03 — the root --version flag prints the same line
  Given any quarry binary
  When the user runs `quarry --version`
  Then stdout is the same line `quarry version` prints and exit is 0

Scenario: SCENARIO-04 — version needs nothing else to work
  Given HOME is unset, no store exists and Quicken is not installed
  When the user runs `quarry version`
  Then it prints the version and exits 0

Scenario: SCENARIO-05 — version refuses arguments
  Given any quarry binary
  When the user runs `quarry version foo`
  Then stderr shows the existing "takes no arguments" usage error and exit is 2

Scenario: SCENARIO-06 — the store records the same version string
  Given a quarry built from a checkout whose build records (devel), revision 260d0d29f940… and uncommitted changes
  When the user runs `quarry sync`
  Then store_info.quarry_version and `quarry status --json` .store.quarry_version are "(devel) 260d0d29f940+dirty"

Scenario: SCENARIO-07 — the MCP server reports the same version string
  Given a quarry built from a checkout whose build records (devel), revision 260d0d29f940… and uncommitted changes
  When an MCP client sends initialize to `quarry mcp`
  Then serverInfo.version is "(devel) 260d0d29f940+dirty"
```

---

## Sizing
| Scenario | Verdict — numbers |
| --- | --- |
| SCENARIO-01 | OWNS A RUN — ~3 batches, 1 feature package (`internal/version` Resolve + table test; `internal/cli/version.go` + `Env.BuildInfo` + root registration/doc/help pin; `cmd/quarry` `buildVersion` delegate + `defaultEnv` wiring pin). Sonnet architect. |
| SCENARIO-04 | FOLD into 01 — coverage of 01's code; 01 carries its acceptance test (nil-factory Env, HOME=""). |
| SCENARIO-05 | FOLD into 01 — existing noArgs copy, one assertion. |
| SCENARIO-02 | LIGHT — `cli`: `json_version.go`, `--json` branch, pins (key order, nulls, go, write-failure). |
| SCENARIO-03 | LIGHT — `cli`: local root `--version` flag + root `RunE` sharing 02's emit; help-pin Flags line; refusal rows; regression pins. Stops `PARTIAL: needs architect` if cobra dispatch changes. |
| SCENARIO-06 | LIGHT — `cmd/quarry`: `newServerFactory(info, …)` instead of reading build info itself; mechanical callers; sync acceptance test. `PARTIAL: needs architect` if callers run past a handful. |
| SCENARIO-07 | LIGHT — `cmd/quarry`: `(devel) 260d0d29f940+dirty` row in existing `startMCP`/`c.info` cases (`run_mcp_test.go:126`). |

## BDD Acceptance Progress
- [ ] SCENARIO-01: quarry version prints the binary's version
- [ ] SCENARIO-04: Version needs nothing else to work
- [ ] SCENARIO-05: Version refuses arguments
- [ ] SCENARIO-02: --json reports version, commit, modified and Go version
- [ ] SCENARIO-03: The root --version flag prints the same line
- [ ] SCENARIO-06: The store records the same version string
- [ ] SCENARIO-07: The MCP server reports the same version string
