# Specification: Phase 2c — snapshots, config and import history

<!-- spec-check: v1 -->

## Intent & Goal

**Primary Goal**: Stop snapshots piling up. `quarry sync` keeps the newest `snapshots.keep` snapshots (default 12) and never deletes the one the store was built from; `quarry snapshots` lists them and `quarry snapshots prune` applies the cap by hand. A config file (`~/Library/Application Support/quarry/config.toml`) holds `snapshots.keep` and `quicken.path`. `import_runs` keeps one row per successful build across rebuilds, so the hash of every snapshot a store was built from outlives the snapshot. The spend/cashflow report pipeline is de-duplicated first (PRE-01).

**Out of Scope**: config keys other than `snapshots.keep` and `quicken.path` (`sync.busy_timeout`, `reports.since`, spill cap → MCP phase, `--currency` → 2f); a `quarry config` command (DON'T BUILD — the file is the interface); snapshot pinning, size or age caps; per-source caps (one global cap); a lock between `prune` and a concurrent `sync` (recorded as an open debt); Quicken app version (quarry has no source for it); chmod of snapshots to read-only.

**Business Rules**: see below.

## Business Rules & Invariants
- P2c-1 (config file): TOML at `~/Library/Application Support/quarry/config.toml`. No env override, no `--config` flag. quarry never writes it in 2c; permissions not checked on read. Missing or empty file → defaults, silently. Known keys exactly `snapshots.keep` and `quicken.path`; `[snapshots] keep = 24` and `snapshots.keep = 24` are equivalent.
- P2c-2 (who reads config): `sync` (incl. `--from`), `snapshots`, `snapshots prune` load and validate the whole file; a malformed file or bad value refuses them (C1/C2/C2q/C2r/C4, exit 1) even when a flag overrides the key. `status`, `accounts`, `spend`, `cashflow`, `sql` never load it. `sync` validates config before it touches Quicken: a config refusal takes no snapshot.
- P2c-3 (precedence): flag > config > built-in default (`snapshots.keep` 12; Quicken path: `--quicken` > `quicken.path` > discovery). With `--quicken`, `quicken.path` is type-checked but never stat'ed. `sync --from` ignores `quicken.path`'s value (never checks existence).
- P2c-4 (`quicken.path` value): leading `~/` expanded with `homepath.Expand`; only absolute or `~/` paths valid; relative or empty refused (C2r); not resolved against the config folder. Trailing slash and symlinks accepted as for the flag.
- P2c-5 (snapshot identity and order): a snapshot is a regular file in the snapshots folder named `^\d{8}T\d{6}Z(_\d+)?\.sqlite$`; its manifest is `<id>.json`. Partials, strays, orphan `.json`, subdirectories are not snapshots. Newest = ID timestamp, then numeric `_N` suffix (none = 0; `_10` newer than `_2`); never `taken_at`. List and prune share this order.
- P2c-6 (store's snapshot): the latest `import_runs.snapshot_path` (highest id), compared after `filepath.EvalSymlinks` on both sides. If no folder snapshot matches by path, the one whose ID equals the recorded path's base name without extension is the store's own (U6). After sync, the snapshot just built (known to sync without a store read).
- P2c-7 (retention): keep the newest N; the store's snapshot is never deleted — when outside the newest N, N+1 are kept. Delete `<id>.sqlite` then `<id>.json`. An orphan `<id>.json` whose `.sqlite` is gone is removed silently (not counted, not listed). One global cap across Quicken files.
- P2c-8 (auto-prune): runs only after a successful store swap (`sync` or `--from`). Not after schema mismatch, validation failure or any refusal: a failed sync deletes nothing. Rejected snapshots (integrity fail, no accounts) are still discarded at once. Snapshots kept after a schema mismatch or validation failure count toward the cap. Prune failures after a successful sync warn; exit stays 0.
- P2c-9 (import history): `import_runs` keeps one row per successful build, carried from the previous store into the new one. New row id = previous max id + 1; carried rows keep ids and values; columns an older store lacks are NULL. No previous store → history starts silently (CF1). Previous store unreadable → CF2 warning, history restarts, build proceeds.
- P2c-10 (latest-row reads): `duckstore/status.go` (`store_info CROSS JOIN import_runs`) and `duckstore.go` `snapshotPathQuery` (R2's `--from` hint) read the latest row (`ORDER BY id DESC LIMIT 1`). Every `status` line and `status --json` field stays byte-identical. `quarry sql "SELECT * FROM import_runs"` returns one row per build (intended).
- P2c-11 (format): `duckstore.FormatVersion` bumps 3→4 only if the DDL changes; if bumped, read commands show the existing R2 line until the first 2c sync (accepted).
- P2c-12 (PRE-01 neutrality): the report-pipeline refactor changes no behaviour: no hunk in any `package *_test` black-box test file under `internal/cli/spend*_test.go`, `internal/cli/cashflow_test.go`, `internal/cli/report_help_test.go`, `internal/store/duckstore/spending*_test.go`, `internal/store/duckstore/cashflow_test.go`, `cmd/quarry/`; ±0 top-level tests per package (except tests of deleted dead code, listed); full suite, lint, coverage gate clean; no `--json` field, help string, golden or `testdata` change. Needing to change a black-box assertion = behaviour change → stop and report.
- P2c-13 (PRD amendments): drop "Quicken version" from the `snapshots` row (:163) and the `import_runs` row (:127). :146 paragraph → "A rejected snapshot (no accounts, failed integrity check) is deleted at once and never counts toward the cap. `import_runs` keeps one row per successful build, carried across rebuilds, so the hash of every snapshot a store was built from outlives the snapshot. A snapshot that was never built into a store keeps its hash only in its manifest, which is deleted with it." :127 → "Snapshot hash, row counts, validation results | One row per successful build, kept across rebuilds; audit trail". A snapshot never built into a store keeps its hash only in its manifest.

---

## Triage Brief
- No `snapshots` command; root registers sync, status, accounts, spend, cashflow, sql (`internal/cli/root.go:29-34`). Prior art: `newStatusCommand(env.NewReport, jsonOut)`, `newSyncCommand(env.NewServer, jsonOut)`, `noArgs`, `UsageError`.
- Snapshot package `internal/snapshot`: `Sync` (`snapshot.go:114-188`) backs up to a partial, `buildManifest` (`:218-240`), commits manifest then snapshot (`commit` `:192`). Rejection already immediate (`snapshot.go:158-161` `destination.Discard`; `content_refusal.go:31`). Schema mismatch commits and keeps (`snapshot.go:184-186`, `MismatchError`). `SyncAndImport` (`import.go:80`) → `importVerified` (`:91`); no post-import hook yet.
- Wiring `cmd/quarry/run.go:46-70` `newServerFactory`: `snapshot.NewServer(WithSnapshotDir(<storeDir>/snapshots), WithReference, WithHome, WithImporter, WithStoreProbe)`. No retention option. `report.Server` (`run.go:84-94`) has no snapshot-dir access; `report.SnapshotID` (`report.go:60`) duplicates `snapshot.snapshotID` (`import.go:69`).
- Reusable: `readManifest` (`from.go:135`), `hashFile` (`snapshot.go:285-298`), `resolveFrom`/`ImportFrom` (`from.go`), `sweepLeftovers` (`destination.go:50-64`, name+age delete), `Destination` port (`ports.go`, no List/Remove).
- Manifest (`manifest.go:11-26`): Path, Manifest, Source, TakenAt (RFC3339), Bytes, SHA256, Accounts; Schema.Fingerprint, Verified. No Quicken version anywhere.
- `import_runs` (`duckstore/schema.go:78-104`): snapshot_path, snapshot_sha256, schema_fingerprint, snapshot_taken_at, source_path, …; importer hardcodes `importRunID = 1` (`importer.go:128,139`); `Status` refuses ≠1 run (`status.go:15-16,~60`).
- Permissions: snapshot dir 0700, files 0600 (`destination.go:40,76,113`); store dir 0700, duckdb 0600.
- Home/path seams: `resolveHome` (`run.go:107`), `storeDirUnder` (`run.go:97`), `homepath.Expand/Abbreviate` (~45 call sites), `DiscoverBundle`, `ResolveBundlePath`, bundle refusals `bundle.go:39,45,104-119`, discovery refusals `discover.go:135-168`.
- No config loader, no TOML dependency.
- **Already exists — do not re-plan:** exit-code plumbing, `--json` root flag, `formatMB`, `takenLayout`, `humanize.Count`, `marshalDocument`, `noArgs`, `homepath`, bundle/discovery refusals, `readRefusal` R1–R3, manifest read/hash, snapshot ID layout (`20060102T150405Z`, `_N`).
- Callers (all `grep`; gopls findReferences failed its control): `WithSnapshotDir`/`NewServer` `run.go:61-62`; `SyncAndImport`/`ImportFrom` `cli/sync.go:~86-92`; `newDirDestination` `snapshot.go:125`; `readManifest` `from.go:35`; `hashFile` `snapshot.go:224`, `from.go:40`; `snapshotID` `import.go:127,163,203`, `outcome.go:39`, `from.go:100`; `report.SnapshotID` `cli/json_status.go:88`, `cli/render_status.go:36`, `report/refusal.go:82`; `importRunID` `importer.go:128,139,143`; `ImportRuns` `duckstore.go:425,544`, `status.go:15-16,49`; `DefaultWindow` `report/window.go:38`.

## Product Verdict
**SHIP WITH CHANGES** (scoping 2026-09-30), user decisions: TOML; one global cap; keys `snapshots.keep` and `quicken.path`; report-pipeline refactor inside 2c as PRE-01.
1. Scope: snapshots list/prune, auto-prune, config (two keys), `import_runs` history ("carry-forward" = history carried across rebuilds, 2a spec:12). 2d's open→fixed findings reuse the carry-forward seam.
2. `import_runs` becomes carried history; `status` and R2's hint read the latest row.
3. PRD amended (P2c-13).
4. Bad config refused (exit 1) only in the three commands that read it, before sync touches Quicken; unknown key warns.
5. `sync --json` `pruned` key always present, `null` when no store was built.
6. `quicken.path` with the precedence and copy below; discovery refusals name the key.
7. PRE-01 before SCENARIO-01 under P2c-12.

## Pre-step
**PRE-01 — report-pipeline de-duplication** (from `docs/specifications/phase2b-spending/STATE.md` open debt "Parallel report pipeline"; behaviour-neutral, no scenario, no `When`). Run tag `run: build feature: phase2c-snapshots-config unit: PRE-01`; one architect plan with line-anchored inventories, developer runs, test-reviewer checkpoint; arch-reviewer sees the range at the final gate. Proof: P2c-12. Checklist:
- extract `renderTable` (rename `spendingTotalLabel`/`spendingPartialStatus`/`spendingAccountsCaption`);
- move `accountFilter`/`readArgs`/`marks`/`transactionRangeQuery` to duckstore `filter.go` and extract `transactionRange`;
- one `report` fill helper for `fillMonths`/`fillPeriods`;
- one report flag struct (`--since/--until/--account`) and a shared `renderResult` + `emit` tail;
- `spendAccountDocument` → `accountDocument`; `cashFlowCells` args → `CashFlowFigures`;
- `allLeftOut` for `appendEmptyWindowWarning`;
- `time.DateOnly` for the duplicated date layouts;
- `DefaultWindow` into `window.go`; remove `period.First` if unused;
- `"spend"`/`"cashflow"` command-name consts;
- `ErrUnsupportedPeriod`/`ErrUnsupportedGrouping` reachability note.
- move the per-`store.OpenFault` reason phrase out of `report/refusal.go:50-61` `storeRefusal` down to `internal/store`, `report` calling it — so `snapshot` renders the R3 reasons in CF2, the `snapshots` warning and the prune refusal without importing `report` (added by the sizing pass; neutral under P2c-12). No new tests: `report/refusal_test.go:47-66` already drives every moved branch (NotDuckDB, Permission, Locked, Other), so `internal/store` stays ±0.
If `spec-check.py --run` does not accept a non-scenario unit, record PRE-01 in `METRICS.md` only.

## Surface & Copy

Paths in copy are `~`-abbreviated in human output and absolute in `--json` (2a rule). `<config>` = `~/Library/Application Support/quarry/config.toml`; `<snapshots>` = `~/Library/Application Support/quarry/snapshots`.

### Config refusals and warnings
| # | Condition | stderr | Exit |
|---|---|---|---|
| C1 | malformed TOML | `quarry: cannot read ~/Library/Application Support/quarry/config.toml: line 3: <first line of the parser's message>; fix the file and run the command again` | 1 |
| C2 | `snapshots.keep` not an integer ≥ 1 (0, -1, 2.5, "twelve") | `quarry: ~/Library/Application Support/quarry/config.toml: snapshots.keep must be a whole number of 1 or more, got 0; fix the file and run the command again` (value as TOML wrote it: `got "twelve"`, `got 2.5`) | 1 |
| C2q | `quicken.path` not a string (`12`, `true`, array, table) | `quarry: ~/Library/Application Support/quarry/config.toml: quicken.path must be a path in quotes, got 12; fix the file and run the command again` | 1 |
| C2r | `quicken.path` relative or empty | `quarry: ~/Library/Application Support/quarry/config.toml: quicken.path must be a full path or start with ~/, got "Home.quicken"; fix the file and run the command again` | 1 |
| C2a | a value for `snapshots.keep` or `quicken.path` written as a table (`[quicken.path]` header) | C2 / C2q line ending `got a table`, e.g. `quarry: ~/Library/Application Support/quarry/config.toml: quicken.path must be a path in quotes, got a table; fix the file and run the command again` | 1 |
| C2m | multi-line value (array) for either key | C2 / C2q line with the raw value collapsed to one line (every whitespace run → one space, trimmed), e.g. `… snapshots.keep must be a whole number of 1 or more, got [ 1, 2 ]; fix the file and run the command again` | 1 |
| C2t | a plain value where a table is expected | `snapshots = 3` → `quarry: ~/Library/Application Support/quarry/config.toml: snapshots must be a table, such as snapshots.keep = 12, got 3; fix the file and run the command again`; `quicken = "x"` → `quarry: ~/Library/Application Support/quarry/config.toml: quicken must be a table, such as quicken.path = "~/Documents/Home.quicken", got "x"; fix the file and run the command again`; arrays/inline values follow the C2m collapse rule | 1 |
| C2l | a key introduced by an array-of-tables header (`[[snapshots]]`, `[[quicken]]`, `[[snapshots.keep]]`, `[[quicken.path]]`) | the C2t / C2 / C2q line ending `got a list of tables` (never an empty `got`, never the body text), e.g. `quarry: ~/Library/Application Support/quarry/config.toml: snapshots must be a table, such as snapshots.keep = 12, got a list of tables; fix the file and run the command again`. Rule: `got` is the collapsed raw text only for a value written right of `=`; a header-introduced value is named by kind — `[x]` → `a table`, `[[x]]` → `a list of tables`; an inline array of inline tables after `=` stays raw and collapsed | 1 |
| C3 | unknown key at any level (one line per key, file order; never `snapshots` or `quicken` themselves — see C2t) | `quarry: warning: ~/Library/Application Support/quarry/config.toml: unknown key snapshot.keep; quarry ignores it`; same text without `quarry: warning: ` in `warnings[]`; command proceeds | 0 |
| C4 | path exists but unreadable, or is a directory | `quarry: cannot read ~/Library/Application Support/quarry/config.toml: <OS reason per G1>; fix the file and run the command again` | 1 |

### `quicken.path` outcomes (sync)
| # | Condition | stderr | Exit |
|---|---|---|---|
| K1 | configured path does not exist | `quarry: ~/Documents/Home.quicken does not exist; check quicken.path in ~/Library/Application Support/quarry/config.toml, or pass the file with --quicken <path>` | 1 |
| K2 | configured path not a bundle, or bundle with no `data` | `quarry: ~/Documents/Home.quicken is not a Quicken for Mac file (expected a .quicken bundle containing a data file); set quicken.path in ~/Library/Application Support/quarry/config.toml to the .quicken bundle` | 1 |
| K3 | configured path is `.qdf` | reused verbatim from `bundle.go:45` | 1 |
| K4 | configured bundle not open in Quicken | reused verbatim from `notOpenInQuickenRefusal` (`bundle.go:111-112`) | 1 |
| K5 | configured path unreadable | reused verbatim from `unreadableRefusal` (`bundle.go:117-119`) | 1 |

Flag-origin copy (`bundle.go:39`, `notABundleRefusal` `bundle.go:104-105`) stays byte-identical for `--quicken`. K1/K2 need to know the path's origin.

| Input | Outcome |
|---|---|
| `quicken.path` set, no `--quicken`, bundle open | syncs it; output identical to `--quicken` (Source line shows the path; nothing says where it came from) |
| `--quicken X` and `quicken.path Y` | syncs X; Y type-checked only |
| `--quicken X`, `quicken.path` relative | C2r, exit 1 |
| `sync --from <id>`, `quicken.path` missing on disk | succeeds; key ignored |
| `sync --from <id>`, `quicken.path = 12` | C2q, exit 1 |
| `quicken.path` unset | discovery (amended lines below) |
| trailing slash / symlink to bundle | accepted |
| `snapshots` / `prune`, `quicken.path` missing on disk | no effect (never stat'ed) |

### `quarry snapshots`
- Use `snapshots`; Short `List the snapshots quarry has taken and which one the store was built from`
- Long:
```
List the snapshots quarry sync has taken, newest first: when each was
taken, its size, the Quicken file it came from, and which one the store was
built from. Snapshots live in ~/Library/Application Support/quarry/snapshots.

After each successful sync, quarry deletes the oldest snapshots beyond the
newest 12, never the one the store was built from. Snapshots of every
Quicken file count toward the same 12. To keep a different number, set
snapshots.keep in ~/Library/Application Support/quarry/config.toml:

  [snapshots]
  keep = 24

Status is "store" for the snapshot the store was built from, "schema
differs" for one quarry cannot import, and "no manifest" for one that
cannot be used with --from. Rebuild the store from a listed snapshot with
quarry sync --from <ID>.
```
- Example:
```
  quarry snapshots
  quarry snapshots prune --dry-run
```
- stdout (exit 0): columns two spaces apart; Size right-aligned via `formatMB`; Taken via `takenLayout`, local time; Source = base name of the manifest's source; no trailing spaces; Total row per spend precedent.
```
ID                Taken                     Size  Source        Status
20260930T141502Z  2026-09-30 10:15 EDT   55.2 MB  Home.quicken  store
20260929T090011Z  2026-09-29 05:00 EDT   55.2 MB  Home.quicken  schema differs
20260927T143005Z  unknown                55.1 MB  unknown       no manifest
Total                                   165.5 MB
```
- Size = `.sqlite` size from `stat`. Status precedence: `store` > `no manifest` (missing or unreadable manifest) > `schema differs` (`schema.verified` false) > blank.
- `--json`:
```json
{"directory":"/Users/…/snapshots","keep":12,
 "store_snapshot":{"id":"20260930T141502Z","path":"/Users/…/20260930T141502Z.sqlite"},
 "snapshots":[{"id":"…","path":"…","manifest":"…","taken_at":"2026-09-30T14:15:02Z","bytes":57881234,
   "source":"/Users/…/Home.quicken","sha256":"…","schema_verified":true,"store":true}],
 "total_bytes":173643702,"warnings":[]}
```
  No manifest → `manifest`, `taken_at`, `source`, `sha256`, `schema_verified` are `null` (keys always present). `store_snapshot` `null` when no store or store unreadable; `store_snapshot.id` = path base name without extension.

| Input | stdout | stderr | Exit |
|---|---|---|---|
| folder missing or empty | header only, no Total; JSON `snapshots:[]`, `total_bytes:0` | `quarry: no snapshots in ~/Library/Application Support/quarry/snapshots yet; run quarry sync to take one` (in `warnings[]` without `quarry: `) | 0 |
| no store | table, nothing `store`; `store_snapshot:null` | nothing | 0 |
| store unreadable (R3 cases; R2 still yields `snapshot_path` and counts as readable) | table, nothing marked | `quarry: warning: cannot tell which snapshot the store was built from: <R3 reason>` | 0 |
| store built with `--from <path>` outside the folder | nothing marked; `store_snapshot` = that path | nothing | 0 |
| store's snapshot deleted by hand | nothing marked; `store_snapshot` still names it | nothing | 0 |
| folder unreadable | empty | `quarry: cannot read ~/Library/Application Support/quarry/snapshots: <OS reason per G1>` | 1 |
| positional argument (`quarry snapshots list`) | empty | `quarry: snapshots takes no arguments; to delete old snapshots run quarry snapshots prune; Run 'quarry snapshots --help' for usage.` | 2 |
| C1/C2/C2q/C2r/C4 | empty | as ruled | 1 |

Further `snapshots` outcomes (copy ruling, SCENARIO-16):

| Outcome | stdout | stderr | Exit |
|---|---|---|---|
| a listed `.sqlite` whose stat fails (folder readable, not searchable) | empty | `quarry: cannot read ~/Library/Application Support/quarry/snapshots: <OS reason per G1>` | 1 |
| no snapshots and store unreadable | header only; JSON `snapshots:[]`, `store_snapshot:null` | the no-snapshots line, then `quarry: warning: cannot tell which snapshot the store was built from: <reason>`; `warnings[]` carries both in that order without prefixes | 0 |
| manifest decodes but `taken_at` does not parse, or `source` is empty | that cell reads `unknown`; Status still follows `schema.verified` (never `no manifest`); `--json` only the affected field is `null` | nothing | 0 |
| store of this format with zero import runs | table, nothing marked; `store_snapshot:null` | `quarry: warning: cannot tell which snapshot the store was built from: the store has no import history` | 0 |
| store of another format (R2) that names no snapshot path | table, nothing marked; `store_snapshot:null` | `quarry: warning: cannot tell which snapshot the store was built from: the store was built by another version of quarry` | 0 |
| interrupted (SIGINT/SIGTERM); `ctx.Err()` is checked before any store-fault classification | empty | `quarry: snapshots interrupted` | 1 |

An R2 store that does name a snapshot path stays readable: that snapshot is marked (and protected by prune).

### `quarry snapshots prune`
- Use `prune`; Short `Delete all but the newest snapshots`
- Long:
```
Delete all but the newest snapshots now, as sync does after each successful
sync. prune keeps --keep snapshots, or snapshots.keep from
~/Library/Application Support/quarry/config.toml (12 unless set). The
snapshot the store was built from is never deleted, even when it is older.

With --dry-run, prune lists what it would delete and deletes nothing.
```
- Example:
```
  quarry snapshots prune --dry-run
  quarry snapshots prune --keep 3
```
- Flags: `--keep` `IntVar` default 0 read via `Changed()` (no default printed), help ``keep the newest `n` snapshots (default: snapshots.keep in the config file, 12 unless set)`` → `--keep n`; `--dry-run` help `list the snapshots prune would delete without deleting them`.
- Keep phrase: N=1 `the newest one`; N≥2 `the newest 12`; protected snapshot outside → append ` and 20260801T120000Z, the store's snapshot`. Counts via `humanize.Count` (`1 snapshot` / `3 snapshots`). Row: `  <ID>  <Taken or unknown>  <Size right-aligned>`, newest first.

| Outcome | stdout | stderr | Exit |
|---|---|---|---|
| deleted | `Deleted 3 snapshots (165.4 MB), keeping the newest 12:` + one row per deleted snapshot | nothing | 0 |
| dry run | `Would delete 3 snapshots (165.4 MB), keeping the newest 12:` + rows | nothing | 0 |
| store's snapshot outside kept set | `Deleted 3 snapshots (165.4 MB), keeping the newest 12 and 20260801T120000Z, the store's snapshot:` | nothing | 0 |
| nothing to delete (incl. count ≤ N+1 when the extra is protected) | `Nothing to delete: 5 snapshots, within the newest 12` (singular `1 snapshot`) | nothing | 0 |
| no snapshots / folder missing | `Nothing to delete: no snapshots in ~/Library/Application Support/quarry/snapshots` | nothing | 0 |
| no store | nothing protected; keeps newest N; normal lines | nothing | 0 |
| store exists, R3-unreadable (incl. dry run) | empty | `quarry: cannot tell which snapshot the store at ~/Library/Application Support/quarry/quarry.duckdb was built from (<R3 reason>), so no snapshot was deleted; run quarry sync to rebuild the store, then prune again` | 1 |
| some deletes fail | `Deleted …` block lists only successes (empty stdout if none succeeded) | per failure `quarry: cannot delete snapshot 20260601T090000Z: <OS reason per G1>` | 1 |
| interrupted (SIGINT/SIGTERM) | `Deleted …` block for what was deleted | `quarry: snapshots prune interrupted; 2 snapshots were not deleted` | 1 |
| `--keep 0` or negative | empty | `quarry: --keep must be 1 or more; the snapshot the store was built from is always kept; Run 'quarry snapshots prune --help' for usage.` | 2 |
| `--keep abc` | empty | existing cobra parse-error line with the same suffix | 2 |
| positional argument | empty | `quarry: prune takes no arguments; Run 'quarry snapshots prune --help' for usage.` | 2 |
| folder unreadable | empty | as `snapshots` | 1 |
| C1/C2/C2q/C2r/C4 | empty | as ruled | 1 |

Further `prune` outcomes (copy ruling, SCENARIO-16; all apply to `--dry-run` too):

| Outcome | stdout | stderr | Exit |
|---|---|---|---|
| store of this format with zero import runs | empty | `quarry: cannot tell which snapshot the store at ~/Library/Application Support/quarry/quarry.duckdb was built from (the store has no import history), so no snapshot was deleted; run quarry sync to rebuild the store, then prune again` | 1 |
| store of another format (R2) naming no snapshot path | empty | `quarry: cannot tell which snapshot the store at ~/Library/Application Support/quarry/quarry.duckdb was built from (the store was built by another version of quarry), so no snapshot was deleted; run quarry sync to rebuild the store, then prune again` | 1 |
| interrupted before any delete | empty | `quarry: snapshots prune interrupted` | 1 |

Prune rulings U1–U7 (copy ruling, SCENARIO-21; all apply to `--dry-run` too):

| # | Outcome | Ruling | Exit |
|---|---|---|---|
| U6 | store's recorded snapshot path does not resolve, but a snapshot with the same ID is in the folder | **Match by path first, then by ID** (P2c-6 amended): a folder snapshot is the store's own if its path equals the recorded `import_runs.snapshot_path` after `EvalSymlinks`; if none matches by path, the one whose ID equals the recorded path's base name without extension is the store's own. One rule for `snapshots` (Status `store`, `"store":true`) and `prune` (protected; keep phrase names it as usual). `store_snapshot.path` in `--json` stays the recorded path. No refusal, no new line | 0 |
| U6b | store names a path outside the folder and no same-ID file exists there | nothing marked, nothing protected; normal lines | 0 |
| U7 | the cannot-tell refusals (store unreadable / no import history / built by another version) fire **only when at least one snapshot lies beyond the newest N** (a deletion depends on knowing the store's snapshot). With count ≤ N the store is not consulted: `Nothing to delete: 5 snapshots, within the newest 12`. No snapshots or no folder: `Nothing to delete: no snapshots in ~/Library/Application Support/quarry/snapshots`, stderr empty | 0 (refusal stays 1) |
| U1 | `.sqlite` removed, then removing its `.json` fails | counts as deleted; nothing printed; the next prune removes the leftover `.json` | 0 |
| U5 | `.sqlite` already gone at delete time | not a failure and not counted: dropped from the `Deleted` block, count, size and `deleted[]`; not in `failed[]`; its `.json` removed as an orphan. If it was the only candidate: `Nothing to delete: <count now> snapshots, within the newest N`; no stderr | 0 |
| U2 | removing an orphan `.json` fails | silent; exit unaffected | — |
| U3 | singular mid-delete interrupt | `quarry: snapshots prune interrupted; 1 snapshot was not deleted` | 1 |
| U4 | a failed delete and an interrupt in one run | stdout the `Deleted …` block for the successes; stderr each `quarry: cannot delete snapshot <id>: <OS reason>`, then `quarry: snapshots prune interrupted; N snapshots were not deleted`. N counts only selected snapshots never attempted (failures not counted). Bare `quarry: snapshots prune interrupted` only when nothing was attempted. `--json`: `deleted[]`/`failed[]` as they stand at the interrupt | 1 |

Prune `--json` rulings (copy ruling, SCENARIO-28; refines U7 — "not consulted" means "cannot block", not "must not be opened"):
- When count ≤ N (incl. zero snapshots) prune reads the store's snapshot **best-effort, in text and `--json` mode, one code path**: success → `store_snapshot` `{"id","path"}` exactly as `quarry snapshots --json` gives (U6 path-then-ID, R2 naming a path); no store / unreadable / no import history / other version naming no path → `store_snapshot: null`, stderr empty, `warnings: []`, exit 0. Never refuses, never warns. Text mode prints the same `Nothing to delete: …` line either way. An interrupt during that read is still `quarry: snapshots prune interrupted`, exit 1.
- `store_snapshot: null` in `prune --json` means "no store, or the store could not be read and nothing depended on it"; when count > N it can only mean "no store" (every other case refuses).
- Refusals under `--json`, with or without `--dry-run` (cannot-tell, C1/C2/C2q/C2r/C2t/C4, folder unreadable, usage): stdout empty, stderr the refusal line, exit 1 (2 for usage). No JSON document.
- Partial failure or interrupt: JSON printed with `deleted[]`/`failed[]` as they stand, stderr lines as ruled, exit 1.
- `would_delete[]`/`deleted[]` entries `{"id","path","bytes"}` newest first (ID timestamp, then numeric suffix), `path` absolute, `bytes` the `.sqlite` size; `failed[]` entries `{"id","path","reason"}` same order; `keep` the effective N (`--keep`, else `snapshots.keep`, else 12); `dry_run` reflects the flag; every list `[]`, never `null`.

- `--json` (one shape): `{"dry_run":false,"keep":12,"store_snapshot":{…}|null,"deleted":[{"id","path","bytes"}],"would_delete":[{"id","path","bytes"}],"failed":[{"id","path","reason"}],"warnings":[]}`. `would_delete` `[]` in a real run; `deleted`/`failed` `[]` in a dry run; partial failure still prints JSON, exit 1.

### Sync auto-prune
- Human: one line after the Transfers block and its `?` rows, only when ≥ 1 snapshot was deleted (`%-10s` label): `Pruned    3 snapshots beyond the newest 12 (165.4 MB)`; singular `Pruned    1 snapshot beyond the newest 12 (55.1 MB)`; N=1 `… beyond the newest one (…)`; after `--from <old>` outside the kept set `Pruned    3 snapshots beyond the newest 12 and the store's own (165.4 MB)`.
- `--json`: top-level `"pruned"` after `store`, always present: `null` when no store was built (mismatch, validation failed); else `{"keep":12,"deleted":[{"id","path","bytes"}],"failed":[{"id","path","reason"}]}`, `deleted:[]` when nothing to delete.
- Failures warn, exit 0 (each also in `warnings[]` without prefix):
  - `quarry: warning: cannot delete snapshot 20260601T090000Z: <OS reason>; run quarry snapshots prune to try again`
  - `quarry: warning: cannot list ~/Library/Application Support/quarry/snapshots to delete old snapshots: <OS reason>; run quarry snapshots prune to try again`
- Interrupted during prune (store already swapped): `quarry: sync interrupted while deleting old snapshots; the store was rebuilt; run quarry snapshots prune to finish` — exit 1.
- CF1: no previous store → history starts silently. CF2: previous store's `import_runs` unreadable → `quarry: warning: cannot carry import history forward from the previous store (<reason>); import_runs starts again with this sync` (exit 0; build proceeds).
- CF2 `<reason>` phrases ("it"/"its" = the previous store; warning, build proceeds, exit 0; never driver text or Go type names; `warnings[]` carries the line without `quarry: warning: `):

| Case | `<reason>` |
|---|---|
| open or read fault | `store.OpenError.UnreadableReason`, verbatim (e.g. `the file is not a DuckDB database`) |
| ids in `import_runs` not unique | `its import_runs table repeats an id` |
| no `import_runs` table | `it has no import_runs table` |
| a required column (`id`, `snapshot_path`, `snapshot_sha256`) missing or NULL, or any other row-read fault on a readable file | `its import_runs table is incomplete` |

  Columns an older store format lacks are carried as NULL and are not a fault.

### Changes to existing surfaces
1. `internal/cli/sync.go` sync Long — insert after the "if a check fails…" paragraph:
```
After it rebuilds the store, sync deletes the oldest snapshots beyond the
newest 12 (snapshots.keep in ~/Library/Application Support/quarry/config.toml),
never the one the store was built from; a failed sync deletes nothing. Run
quarry snapshots to list them.
```
   and replace `sync.go:55-57` (discovery sentence) with:
```
Without --quicken, quarry uses quicken.path from
~/Library/Application Support/quarry/config.toml if it is set. Otherwise it
looks for .quicken files in ~/Documents and in
~/Library/Application Support/Quicken/Documents, and uses the one it finds
if there is exactly one.
```
   The `--from` paragraph gains nothing.
2. `--quicken` flag help → ``"`path` to the .quicken file to snapshot (default: quicken.path in the config file, else the only one in ~/Documents or Quicken's Documents folder)"`` (renders `--quicken path`).
3. `internal/snapshot/from.go:117-119` `idNotFoundRefusal` → `quarry: no snapshot 20260601T090000Z in ~/Library/Application Support/quarry/snapshots; run quarry snapshots to list the ones kept` (exit 1); update pinned tests.
4. `internal/snapshot/discover.go:135-137` `noBundleFoundRefusal` → `quarry: no .quicken file found in ~/Documents or ~/Library/Application Support/Quicken/Documents; pass one with --quicken <path> or set quicken.path in ~/Library/Application Support/quarry/config.toml`
5. `internal/snapshot/discover.go:142-150` `multipleQuickenBundlesRefusal` → `quarry: found 2 .quicken files (~/Documents/A.quicken, ~/Documents/B.quicken); choose one with --quicken <path> or set quicken.path in ~/Library/Application Support/quarry/config.toml`
6. Unchanged (still true): `documentsUnreadableRefusal`, `quickenDocumentsUnreadableRefusal` (`discover.go:154-168`); `outcome.go:26`, `import.go:198` "the snapshot is kept at …".
7. `duckstore/status.go:27`, `duckstore.go:175` → latest `import_runs` row (P2c-10).
8. PRD `docs/initial-prd.md` :127, :146, :163 per P2c-13.
10. R3 reason for a store with no import run (`duckstore/status.go` `errImportRunCount`; wording `expected exactly one import run, found 0` is false now that history holds several) → `quarry: cannot read the store at ~/Library/Application Support/quarry/quarry.duckdb: the store has no import history; run quarry sync to rebuild it` (exit 1); update pinned tests. Owner: SCENARIO-16.
9. Close debts when 2c ships: 2a spec:15, 2a STATE:72, 2b STATE "Snapshots accumulate … until 2c"; 2b STATE "Parallel report pipeline" (PRE-01).

---

## Scenarios (Gherkin)

### Config file

```gherkin
Scenario: SCENARIO-01 — A malformed config file is refused before Quicken is touched
  Given Quicken has Home.quicken open
  And config.toml contains "[snapshots" on line 1
  When I run quarry sync
  Then stderr is C1 for line 1, the exit code is 1, no new snapshot exists in the snapshots folder, and the store is unchanged
```

```gherkin
Scenario: SCENARIO-02 — A snapshots.keep below 1 is refused
  Given config.toml sets snapshots.keep = 0
  When I run quarry snapshots prune
  Then stderr is C2 with "got 0", the exit code is 1, and no snapshot is deleted
```

```gherkin
Scenario: SCENARIO-03 — An unknown config key warns and the command proceeds
  Given config.toml sets snapshot.keep = 3 and the snapshots folder holds 2 snapshots
  When I run quarry snapshots
  Then stderr is C3 naming snapshot.keep, stdout lists 2 snapshots, and the exit code is 0
```

```gherkin
Scenario: SCENARIO-04 — Read commands ignore a broken config file
  Given a built store and config.toml containing "[snapshots" on line 1
  When I run quarry status
  Then stdout is the usual status block, stderr is empty, and the exit code is 0
```

### `quicken.path`

```gherkin
Scenario: SCENARIO-05 — sync snapshots the file named by quicken.path
  Given Quicken has ~/Books/Home.quicken open, ~/Documents holds A.quicken and B.quicken, and config.toml sets quicken.path = "~/Books/Home.quicken"
  When I run quarry sync
  Then stdout's Source line is "Source    ~/Books/Home.quicken" and the exit code is 0
```

```gherkin
Scenario: SCENARIO-06 — --quicken overrides quicken.path
  Given Quicken has ~/Documents/A.quicken open and config.toml sets quicken.path = "~/Books/Missing.quicken"
  When I run quarry sync --quicken ~/Documents/A.quicken
  Then stdout's Source line is "Source    ~/Documents/A.quicken" and the exit code is 0
```

```gherkin
Scenario: SCENARIO-07 — A configured path that does not exist is refused naming the config key
  Given config.toml sets quicken.path = "~/Books/Missing.quicken"
  When I run quarry sync
  Then stderr is K1 for ~/Books/Missing.quicken and the exit code is 1
```

```gherkin
Scenario: SCENARIO-08 — A configured path that is not a .quicken bundle is refused naming the config key
  Given config.toml sets quicken.path = "~/Books/notes.txt", a regular file
  When I run quarry sync
  Then stderr is K2 for ~/Books/notes.txt and the exit code is 1
```

```gherkin
Scenario: SCENARIO-09 — A relative quicken.path is refused
  Given config.toml sets quicken.path = "Home.quicken"
  When I run quarry sync
  Then stderr is C2r with got "Home.quicken" and the exit code is 1
```

```gherkin
Scenario: SCENARIO-10 — A quicken.path that is not a string is refused
  Given config.toml sets quicken.path = 12
  When I run quarry sync
  Then stderr is C2q with got 12 and the exit code is 1
```

```gherkin
Scenario: SCENARIO-11 — sync --from ignores quicken.path
  Given snapshot 20260927T143005Z in the snapshots folder and config.toml sets quicken.path = "~/Books/Missing.quicken"
  When I run quarry sync --from 20260927T143005Z
  Then the store is rebuilt from 20260927T143005Z and the exit code is 0
```

```gherkin
Scenario: SCENARIO-12 — Finding several .quicken files points at quicken.path
  Given ~/Documents holds A.quicken and B.quicken and no quicken.path is set
  When I run quarry sync
  Then stderr is the amended several-files line naming both files and quicken.path, and the exit code is 1
```

### Import history

```gherkin
Scenario: SCENARIO-13 — sync keeps earlier builds in import_runs
  Given a store built from snapshot 20260927T143005Z
  When I run quarry sync --from 20260928T090000Z
  Then import_runs holds 2 rows: row 1 carries 20260927T143005Z's hash unchanged and row 2 carries 20260928T090000Z's hash
```

```gherkin
Scenario: SCENARIO-14 — status reports the latest build when import_runs holds several
  Given a store whose import_runs holds builds from 20260927T143005Z and 20260928T090000Z
  When I run quarry status
  Then stdout's Snapshot line names 20260928T090000Z and every other status line is unchanged from the single-build form
```

```gherkin
Scenario: SCENARIO-15 — sync warns and restarts history when the previous store's history cannot be read
  Given a file at the store path that is not a DuckDB database
  When I run quarry sync --from 20260928T090000Z
  Then stderr is CF2 with reason "the file is not a DuckDB database", import_runs holds 1 row, and the exit code is 0
```

### `quarry snapshots`

```gherkin
Scenario: SCENARIO-16 — snapshots lists newest first, marks the store's snapshot, and totals the size
  Given the snapshots folder holds 20260927T143005Z, 20260930T141502Z and 20260930T141502Z_2, and the store was built from 20260930T141502Z
  When I run quarry snapshots
  Then the rows are 20260930T141502Z_2, 20260930T141502Z, 20260927T143005Z in that order, only 20260930T141502Z has Status "store", the last row is "Total" with the summed size, and the exit code is 0
```

```gherkin
Scenario: SCENARIO-17 — snapshots marks a schema-mismatch snapshot and one with no manifest
  Given one snapshot whose manifest has schema.verified false and one whose .json manifest is missing
  When I run quarry snapshots
  Then the first has Status "schema differs" and the second has Taken "unknown", Source "unknown" and Status "no manifest"
```

```gherkin
Scenario: SCENARIO-18 — snapshots --json lists every snapshot with its store flag
  Given 2 snapshots and the store built from the newer one
  When I run quarry snapshots --json
  Then stdout has keys directory, keep, store_snapshot, snapshots, total_bytes and warnings, and each snapshot has id, path, manifest, taken_at, bytes, source, sha256, schema_verified and store
```

```gherkin
Scenario: SCENARIO-19 — snapshots with none taken yet says how to take one
  Given the snapshots folder does not exist
  When I run quarry snapshots
  Then stdout is the header row only, stderr is the no-snapshots-yet line, and the exit code is 0
```

```gherkin
Scenario: SCENARIO-20 — snapshots warns when the store cannot be read
  Given 2 snapshots and a store file that is not a DuckDB database
  When I run quarry snapshots
  Then no row has Status "store", stderr is "quarry: warning: cannot tell which snapshot the store was built from: the file is not a DuckDB database", and the exit code is 0
```

### `quarry snapshots prune`

```gherkin
Scenario: SCENARIO-21 — prune deletes all but the newest N snapshots
  Given 5 snapshots and the store built from the newest
  When I run quarry snapshots prune --keep 3
  Then stdout starts "Deleted 2 snapshots (<size>), keeping the newest 3:" followed by the 2 oldest IDs newest first, their .sqlite and .json files are gone, and the exit code is 0
```

```gherkin
Scenario: SCENARIO-22 — prune --dry-run lists what it would delete and deletes nothing
  Given 5 snapshots
  When I run quarry snapshots prune --keep 3 --dry-run
  Then stdout starts "Would delete 2 snapshots (<size>), keeping the newest 3:" and all 5 snapshots still exist
```

```gherkin
Scenario: SCENARIO-23 — prune keeps the store's snapshot when it is older than the newest N
  Given 5 snapshots and the store built from the oldest, 20260801T120000Z
  When I run quarry snapshots prune --keep 3
  Then stdout starts "Deleted 1 snapshot (<size>), keeping the newest 3 and 20260801T120000Z, the store's snapshot:" and 20260801T120000Z still exists
```

```gherkin
Scenario: SCENARIO-24 — prune with nothing beyond the cap deletes nothing
  Given 5 snapshots and no snapshots.keep set
  When I run quarry snapshots prune
  Then stdout is "Nothing to delete: 5 snapshots, within the newest 12" and the exit code is 0
```

```gherkin
Scenario: SCENARIO-25 — prune refuses when the store cannot be read
  Given 5 snapshots and a store file that is not a DuckDB database
  When I run quarry snapshots prune --keep 1
  Then stderr is the cannot-tell refusal with reason "the file is not a DuckDB database", all 5 snapshots still exist, and the exit code is 1
```

```gherkin
Scenario: SCENARIO-26 — prune --keep 0 is a usage error
  Given any snapshots folder
  When I run quarry snapshots prune --keep 0
  Then stderr is "quarry: --keep must be 1 or more; the snapshot the store was built from is always kept; Run 'quarry snapshots prune --help' for usage." and the exit code is 2
```

```gherkin
Scenario: SCENARIO-27 — prune reports each snapshot it could not delete and exits 1
  Given 3 snapshots, and the oldest cannot be deleted
  When I run quarry snapshots prune --keep 1
  Then stdout starts "Deleted 1 snapshot (<size>), keeping the newest one:", stderr is "quarry: cannot delete snapshot <oldest>: permission denied", and the exit code is 1
```

```gherkin
Scenario: SCENARIO-28 — prune --json reports deleted, would_delete and failed in one shape
  Given 5 snapshots
  When I run quarry snapshots prune --keep 3 --json
  Then stdout has keys dry_run, keep, store_snapshot, deleted, would_delete, failed and warnings, deleted has 2 entries and would_delete is []
```

### Sync auto-prune

```gherkin
Scenario: SCENARIO-29 — sync deletes snapshots beyond the newest 12 and prints a Pruned line
  Given 12 snapshots, no config file, and Quicken has Home.quicken open
  When I run quarry sync
  Then the oldest snapshot is deleted, stdout ends with "Pruned    1 snapshot beyond the newest 12 (<size>)", and the exit code is 0
```

```gherkin
Scenario: SCENARIO-30 — sync honours snapshots.keep from config
  Given 4 snapshots and config.toml sets snapshots.keep = 2
  When I run quarry sync --from <newest>
  Then only the 2 newest snapshots remain
```

```gherkin
Scenario: SCENARIO-31 — sync --json always carries the pruned key
  Given a snapshot whose schema differs from the reference
  When I run quarry sync --from <it> --json
  Then stdout's "pruned" is null and the exit code is 1
```

```gherkin
Scenario: SCENARIO-32 — a failed sync deletes nothing
  Given 12 snapshots and a Quicken file whose balances do not reconcile
  When I run quarry sync
  Then all 13 snapshots still exist, stdout has no Pruned line, and the exit code is 1
```

```gherkin
Scenario: SCENARIO-33 — sync warns when an old snapshot cannot be deleted, and still succeeds
  Given 12 snapshots, and the oldest cannot be deleted
  When I run quarry sync
  Then stderr is "quarry: warning: cannot delete snapshot <oldest>: permission denied; run quarry snapshots prune to try again", stdout has no Pruned line, and the exit code is 0
```

### `--from` copy

```gherkin
Scenario: SCENARIO-34 — --from with an unknown ID points at quarry snapshots
  Given no snapshot 20260601T090000Z in the snapshots folder
  When I run quarry sync --from 20260601T090000Z
  Then stderr is "quarry: no snapshot 20260601T090000Z in ~/Library/Application Support/quarry/snapshots; run quarry snapshots to list the ones kept" and the exit code is 1
```

Edge rows owned by scenario plans as unit tests (not scenarios): C4; prune interrupted; sync interrupted mid-prune; prune with unreadable folder; sync auto-prune when the folder cannot be listed; store built with `--from <path>` outside the folder; orphan `.json` removed silently; `_10` after `_2`; `--keep abc`; `prune` positional args; `snapshots list`; no-bundle-found discovery line.

---

## Sizing
| Scenario | Verdict — numbers |
| --- | --- |
| PRE-01 | OWNS A RUN (run 1) — 4 batches (duckstore `filter.go` move + `transactionRange` / report fill helper + `DefaultWindow` into `window.go` + `period.First` + reachability note + R3 reason phrase down to `internal/store` / cli `renderTable` + renames + `time.DateOnly` / report flag struct + shared `renderResult`+`emit` tail + `accountDocument` + `CashFlowFigures` + `allLeftOut` + command-name consts), 1 feature package (report) + duckstore + cli; code-first, no run `A` (no acceptance test; proof is P2c-12) → `Runs: B1 \| B2 \| V`; traced in METRICS.md only |
| SCENARIO-01 | OWNS A RUN (run 2) — 3 batches (TOML dependency + loader: missing/empty → defaults, C1 with line, C4 / value rules C2, C2q, C2r with `homepath.Expand`, C3 unknown keys in file order / `sync` loads config before the Server factory touches Quicken, C3 on stderr + `warnings[]`, cmd wiring), 1 new config package + cli + cmd; absorbs 04, 09, 10; owns edge row C4 |
| SCENARIO-02 | FOLD into SCENARIO-24 — test only: C2 is 01's loader; prune starts loading config in 24 |
| SCENARIO-03 | FOLD into SCENARIO-16 — test only: C3 is 01's loader; `snapshots` starts loading config in 16 |
| SCENARIO-04 | FOLD into SCENARIO-01 — test only: `status` never loads config |
| SCENARIO-05 | OWNS A RUN (run 3) — 3 batches (precedence `--quicken` > `quicken.path` > discovery carrying the path's origin; `--from` never resolves it / K1 + K2 origin copy, K3–K5 reused, amended several-files and no-bundle-found discovery refusals / cli passes the configured path, sync Long discovery sentence + `--quicken` help), 1 feature package (snapshot) + cli; absorbs 06, 07, 08, 11, 12; owns surfaces #1 (discovery sentence), #2, #4, #5 |
| SCENARIO-06 | FOLD into SCENARIO-05 — the flag branch of 05's precedence; test only |
| SCENARIO-07 | FOLD into SCENARIO-05 — K1 is one origin branch of the existing not-found refusal |
| SCENARIO-08 | FOLD into SCENARIO-05 — K2, same origin branch as K1 |
| SCENARIO-09 | FOLD into SCENARIO-01 — test only: C2r is 01's loader, reached through `sync` |
| SCENARIO-10 | FOLD into SCENARIO-01 — test only: C2q is 01's loader, reached through `sync` |
| SCENARIO-11 | FOLD into SCENARIO-05 — the `--from` branch of 05's precedence; test only |
| SCENARIO-12 | FOLD into SCENARIO-05 — two refusal strings plus their pinned tests (no-bundle-found line included) |
| SCENARIO-13 | OWNS A RUN (run 4) — 3 batches (duckstore reads the previous store's `import_runs` before the swap, older columns NULL, unreadable → reason on the build result / carry rows, new id = max+1, `importRunID` goes / latest-row reads in `status.go` + `snapshotPathQuery`), 1 feature package (importer) + store/duckstore; absorbs 14; owns PRD amendments (surface #8); no FormatVersion bump |
| SCENARIO-14 | FOLD into SCENARIO-13 — two `ORDER BY id DESC LIMIT 1` reads; without them 13's second row breaks `status` and the R2 hint |
| SCENARIO-15 | LIGHT (run 5) — 2 steps (CF2 line in `Outcome.Warnings` from 13's carry-forward failure, reason via PRE-01's shared phrase / stderr + `warnings[]` pin), snapshot only |
| SCENARIO-16 | OWNS A RUN (run 6) — 4 batches (listing: ID pattern, order by timestamp then `_N`, `stat` size, manifest status precedence, strays and orphan `.json` excluded, missing vs unreadable folder / store's-snapshot read port on duckstore: latest row, `EvalSymlinks`, R3 → warning / cli `snapshots`: table + Total + help + no-args usage + config load / cmd wiring + `idNotFoundRefusal` copy), 1 feature package (snapshot) + duckstore + cli + cmd; absorbs 03, 17, 19, 20, 34; owns edge rows `_10` after `_2`, orphan `.json` not listed, `snapshots list`, `--from <path>` outside the folder |
| SCENARIO-17 | FOLD into SCENARIO-16 — the Status precedence is part of 16's Status column |
| SCENARIO-18 | LIGHT (run 7) — 2 steps (snapshots JSON document, manifest fields `null` with keys present / `renderResult` branch), cli only |
| SCENARIO-19 | FOLD into SCENARIO-16 — the missing-folder branch of 16's listing plus one warning |
| SCENARIO-20 | FOLD into SCENARIO-16 — test of the fault branch 16's store read must cover anyway |
| SCENARIO-21 | OWNS A RUN (run 8) — test-first (store's snapshot never deleted; R3 → nothing deleted) — 4 batches (retention selection in list order: newest N, store's snapshot protected → N+1 / prune on the Server: R3 refusal before any delete, `.sqlite` then `.json`, orphan `.json` silent, per-failure results, interrupt / cli `snapshots prune` + `--keep` ≥ 1 bound + parse error + no-args / Deleted block + keep phrase + failure lines + help), 1 feature package (snapshot) + cli + cmd; absorbs 23, 25, 26, 27; owns edge rows orphan `.json` removed silently, prune interrupted, unreadable folder, `--keep abc`, positional args |
| SCENARIO-22 | FOLD into SCENARIO-24 — dry-run is a no-delete guard (test-first), so not LIGHT; one branch beside 24's |
| SCENARIO-23 | FOLD into SCENARIO-21 — the protection guard 21 builds test-first, plus its keep phrase |
| SCENARIO-24 | OWNS A RUN (run 9) — test-first (`--dry-run` deletes nothing) — 3 batches (prune loads and validates config, `snapshots.keep` default / `--dry-run` + Would-delete block; both Nothing-to-delete lines are already rendered by SCENARIO-21), 1 feature package (snapshot) + cli + cmd; absorbs 02, 22 |
| SCENARIO-25 | FOLD into SCENARIO-21 — the same guard as 23: an unknown store's snapshot deletes nothing |
| SCENARIO-26 | FOLD into SCENARIO-21 — the out-of-bound test 21's `--keep` needs anyway |
| SCENARIO-27 | FOLD into SCENARIO-21 — the fault test 21's delete needs anyway |
| SCENARIO-28 | LIGHT (run 10) — 2 steps (prune JSON document, one shape for real, dry and partial-failure runs / `renderResult` branch, JSON on exit 1), cli only |
| SCENARIO-29 | OWNS A RUN (run 11) — test-first (prune only after a successful swap; the built snapshot protected) — 3 batches (post-swap prune in `SyncAndImport` and `ImportFrom` with a keep option, nothing after mismatch or validation failure / delete failures and cannot-list → warnings, interrupt → exit 1 / cli Pruned line + `pruned` JSON key + sync Long paragraph + cmd wiring of `snapshots.keep`), 1 feature package (snapshot) + cli + cmd; absorbs 30, 31, 32, 33; owns edge rows sync interrupted mid-prune, cannot-list warning; surface #1 (auto-prune paragraph) |
| SCENARIO-30 | FOLD into SCENARIO-29 — test only: the keep option 29 wires from config |
| SCENARIO-31 | FOLD into SCENARIO-29 — the `null` arm of 29's `pruned` key |
| SCENARIO-32 | FOLD into SCENARIO-29 — test of 29's only-after-swap guard |
| SCENARIO-33 | FOLD into SCENARIO-29 — the fault test 29's delete needs anyway |
| SCENARIO-34 | FOLD into SCENARIO-16 — one refusal string plus pinned tests; it points at the command 16 adds |

Sizing notes (binding on per-scenario architects):
- Order follows dependencies, not IDs: PRE-01 first (P2c-12); config loader (01) before every reader; 13 before 16 because the store mark reads the latest `import_runs` row; list (16) before prune (21) — they share one order; prune core (21) before config/dry-run (24), before prune JSON (28) and before sync auto-prune (29).
- 14 folds into 13, not after it: today `Status` refuses `found != 1` (`status.go:60`) and `snapshotPath` returns "" on `rows != 1` (`duckstore.go:257`), so a second row breaks `status` and the R2 hint at once.
- 13 puts the carry-forward failure (an `*store.OpenError`) on the build result, so 15 stays snapshot-only; an `importer.Store` port change in 15 would make it two feature packages.
- R3 reason phrases (`storeRefusal`, `report/refusal.go:50-61`) move down to `internal/store` in PRE-01; 15, 16 and 21 render them from `snapshot`, which may not import `report`. Fallback if PRE-01 cannot take it: 15 does it and becomes OWNS A RUN (2 batches).
- No FormatVersion bump: `import_runs` already has `id BIGINT PRIMARY KEY` and every column; history is rows, not DDL (P2c-11 does not fire). If a DDL change surfaces, 13 owns the 3→4 bump.
- 01 adds the first TOML dependency: its developer needs a network module fetch (sandbox escape) before building.
- Surface #9 (closing 2a/2b debts) is the orchestrator's at SHIP.
- Past units: 16 (< 20), so the p90-twin SPLIT rule does not apply; 2b's heaviest twin (S20, 3 folds) cost 2.4M IE — 16 and 21 are its twins.

## BDD Acceptance Progress
- [x] SCENARIO-01: A malformed config file is refused before Quicken is touched — `cmd/quarry/run_config_test.go` `Test_run_sync_refuses_a_malformed_config_before_taking_a_snapshot`
- [x] SCENARIO-04: Read commands ignore a broken config file — delivered by SCENARIO-01 `cmd/quarry/run_config_test.go` `Test_run_read_commands_ignore_a_malformed_config`
- [x] SCENARIO-09: A relative quicken.path is refused — delivered by SCENARIO-01 `cmd/quarry/run_config_test.go` `Test_run_sync_refuses_a_relative_quicken_path`
- [x] SCENARIO-10: A quicken.path that is not a string is refused — delivered by SCENARIO-01 `cmd/quarry/run_config_test.go` `Test_run_sync_refuses_a_quicken_path_that_is_not_a_string`
- [x] SCENARIO-05: sync snapshots the file named by quicken.path — `cmd/quarry/run_quicken_path_test.go` `Test_run_sync_snapshots_the_file_named_by_quicken_path`
- [x] SCENARIO-06: --quicken overrides quicken.path — delivered by SCENARIO-05 `cmd/quarry/run_quicken_path_test.go` `Test_run_sync_prefers_the_quicken_flag_over_quicken_path`
- [x] SCENARIO-07: A configured path that does not exist is refused naming the config key — delivered by SCENARIO-05 `cmd/quarry/run_quicken_path_test.go` `Test_run_sync_refuses_a_quicken_path_that_does_not_exist`
- [x] SCENARIO-08: A configured path that is not a .quicken bundle is refused naming the config key — delivered by SCENARIO-05 `cmd/quarry/run_quicken_path_test.go` `Test_run_sync_refuses_a_quicken_path_that_is_not_a_bundle`
- [x] SCENARIO-11: sync --from ignores quicken.path — delivered by SCENARIO-05 `cmd/quarry/run_quicken_path_test.go` `Test_run_sync_from_ignores_quicken_path`
- [x] SCENARIO-12: Finding several .quicken files points at quicken.path — delivered by SCENARIO-05 `cmd/quarry/run_discovery_test.go` `Test_run_pools_bundles_across_both_documents_folders`
- [x] SCENARIO-13: sync keeps earlier builds in import_runs — `cmd/quarry/run_import_runs_test.go` `Test_run_sync_from_keeps_the_earlier_build_in_import_runs`
- [x] SCENARIO-14: status reports the latest build when import_runs holds several — delivered by SCENARIO-13 `cmd/quarry/run_status_test.go` `Test_run_status_reports_the_latest_build_when_import_runs_holds_several`
- [x] SCENARIO-15: sync warns and restarts history when the previous store's history cannot be read — `cmd/quarry/run_import_runs_test.go` `Test_run_sync_from_warns_and_restarts_history_when_the_previous_store_is_not_a_duckdb_database`
- [x] SCENARIO-16: snapshots lists newest first, marks the store's snapshot, and totals the size — `cmd/quarry/run_snapshots_test.go` `Test_run_snapshots_lists_newest_first_marks_the_stores_snapshot_and_totals_the_size`
- [x] SCENARIO-03: An unknown config key warns and the command proceeds — delivered by SCENARIO-16 — `cmd/quarry/run_snapshots_test.go` `Test_run_snapshots_warns_about_an_unknown_config_key_and_still_lists`
- [x] SCENARIO-17: snapshots marks a schema-mismatch snapshot and one with no manifest — delivered by SCENARIO-16 — `cmd/quarry/run_snapshots_test.go` `Test_run_snapshots_marks_a_schema_mismatch_and_a_missing_manifest`
- [x] SCENARIO-19: snapshots with none taken yet says how to take one — delivered by SCENARIO-16 — `cmd/quarry/run_snapshots_test.go` `Test_run_snapshots_with_none_taken_yet_says_how_to_take_one`
- [x] SCENARIO-20: snapshots warns when the store cannot be read — delivered by SCENARIO-16 — `cmd/quarry/run_snapshots_test.go` `Test_run_snapshots_warns_when_the_store_cannot_be_read`
- [x] SCENARIO-34: --from with an unknown ID points at quarry snapshots — delivered by SCENARIO-16 — `cmd/quarry/run_from_refusals_test.go` `Test_run_sync_from_an_unknown_id_points_at_quarry_snapshots`
- [x] SCENARIO-18: snapshots --json lists every snapshot with its store flag — `cmd/quarry/run_snapshots_json_test.go` `Test_run_snapshots_json_prints_the_ruled_document_for_two_snapshots_and_a_store`
- [x] SCENARIO-21: prune deletes all but the newest N snapshots — `cmd/quarry/run_prune_test.go` `Test_run_snapshots_prune_deletes_all_but_the_newest_n`
- [x] SCENARIO-23: prune keeps the store's snapshot when it is older than the newest N — delivered by SCENARIO-21 — `cmd/quarry/run_prune_test.go` `Test_run_snapshots_prune_keeps_the_stores_snapshot_when_it_is_older_than_the_newest_n`
- [x] SCENARIO-25: prune refuses when the store cannot be read — delivered by SCENARIO-21 — `cmd/quarry/run_prune_test.go` `Test_run_snapshots_prune_refuses_when_the_store_cannot_be_read`
- [x] SCENARIO-26: prune --keep 0 is a usage error — delivered by SCENARIO-21 — `cmd/quarry/run_prune_test.go` `Test_run_snapshots_prune_refuses_keep_0_as_a_usage_error`
- [x] SCENARIO-27: prune reports each snapshot it could not delete and exits 1 — delivered by SCENARIO-21 — `cmd/quarry/run_prune_test.go` `Test_run_snapshots_prune_reports_each_snapshot_it_could_not_delete_and_exits_1`
- [ ] SCENARIO-24: prune with nothing beyond the cap deletes nothing
- [ ] SCENARIO-02: A snapshots.keep below 1 is refused
- [ ] SCENARIO-22: prune --dry-run lists what it would delete and deletes nothing
- [ ] SCENARIO-28: prune --json reports deleted, would_delete and failed in one shape
- [ ] SCENARIO-29: sync deletes snapshots beyond the newest 12 and prints a Pruned line
- [ ] SCENARIO-30: sync honours snapshots.keep from config
- [ ] SCENARIO-31: sync --json always carries the pruned key
- [ ] SCENARIO-32: a failed sync deletes nothing
- [ ] SCENARIO-33: sync warns when an old snapshot cannot be deleted, and still succeeds
