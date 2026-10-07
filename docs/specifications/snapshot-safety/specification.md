# Specification: Snapshot safety

<!-- spec-check: v1 -->

## Intent & Goal

**Primary Goal**: quarry never loses history or deletes a snapshot it should keep: at most one command that changes quarry's files runs at a time; a snapshot's file-extension letter case never changes how it is listed, pruned or used; and prune never deletes when it cannot tell which snapshot the store was built from. Closes phase2c-snapshots-config Open debts #6, #7, #9, #10 (debt sweep, 2026-10-06).

**Out of Scope**: waiting or retrying for the lock; naming the lock holder (no pid in the lock file); locking readers (status, reports, sql, MCP) or `prune --dry-run`; the DIRECTORY-at-`quarry.duckdb.wal` item (already refused by `Replace`, not concurrency — closed as handled); writing upper-case names (quarry still writes lowercase); the existing R3 `StoreWarning` abbreviated-path form in `warnings[]`.

**Business Rules**: one writer at a time via an OS advisory lock on `quarry.lock`, refused at once when held, taken before any write; extension case-insensitive snapshot and manifest names with one winner per ID; an unreadable (not missing) recorded store path means cannot-tell, protecting nothing by ID.

## Business Rules & Invariants
- One writer: at most one `quarry sync` (fresh, `--from`, its auto-prune) or `quarry snapshots prune` (not `--dry-run`) runs at a time (BR-L1).
- A refused writer changed nothing: the lock is taken after usage and config checks and before any read of Quicken or a snapshot and any file write (BR-L3).
- Letter case of a snapshot's or manifest's extension never matters; the on-disk name is what quarry reports and removes (BR-C1, BR-C3).
- Prune deletes only what it can rule out as the store's file (BR-U1; phase2c P2c-7 safety by construction).

User decisions (2026-10-06): lock covers writers only; held lock refuses now (exit 1, no wait); `.SQLITE` recognized everywhere; WAL-directory item dropped; defect 3 uses the phase2c ruled copy.

---

## Triage Brief

- **No lock exists** (grep `flock|Flock|\.lock` over `*.go`: none). Sync-vs-sync snapshot naming is already safe: `dirDestination.Backup` (internal/snapshot/destination.go:77-104) claims names by exclusive create and bumps `_N`.
- **History carry is last-writer-wins**: `Store.Replace` (internal/store/duckstore/duckstore.go:330) reads carried history at :336, builds a per-pid partial (:339, :42), renames over the store at :379. Overlapping syncs both carry from the same prior store.
- **Prune races**: `importVerified` → `autoPrune` (internal/snapshot/auto_prune.go:12) lists, marks the store from this run's manifest path, deletes; `Server.Prune` (prune.go:118) plans from `listFolder` + `markStore`, deletes with no recheck. `ImportFrom` (from.go:22-) reads manifest, hashes, inspects, imports — a prune between steps fails the import.
- **Readers**: open read-only (`openRead`, duckstore.go:205); the rename at :379 is atomic. No reader hazard constructible. MCP uses the same reads and never writes.
- **Case**: `snapshotFilePattern` (destination.go:24) is lowercase `\.sqlite$`; `scanFolder` (list.go:146-163) builds a case-sensitive `names` map, so `<id>.SQLITE`'s `<id>.json` is an orphan and `sweepOrphans`/`autoPrune` delete it. `ID()` (import.go:154) trims lowercase only. `resolveFrom` (from.go:77-89) mishandles `.SQLITE` in both forms. Already case-aware: `storeEntryIndex` (EqualFold), `entriesAt` file identity (list_store_identity_test.go:84,131).
- **Unreadable recorded path**: `entriesAt` (list.go:254-264) swallows the `os.Stat` error; `os.SameFile(nil, x)` is false; ID fallback then marks nothing, `selectPrune` (prune.go:37) treats the store's snapshot as prunable. Copy ruled, not built: phase2c specification.md:90.
- **Prior ruling reversed**: phase0-snapshot/specification.md:34 (BR-14) and :286 "concurrent syncs | exclusive create; no lock file".

**Already exists — do not re-plan:** `RefusalError`/`causedRefusalError` (internal/snapshot/refusal.go, import.go), `FailureOutcome` → exit 1, `Server.cannotTellRefusal` (prune.go), `Listing.StoreUnreadable`/`StoreWarning`, `markStore`, `storeSnapshot`, `osreason.Reason`, `homepath.Abbreviate`, age-gated leftover sweeps (duckstore.go:394, destination.go:56), exclusive-create naming, `WithRemove` (snapshot.go:101) test seam, `folderUnreadableRefusal` (list.go:178-184), `NoSnapshotsAbsolute` (list.go:94).

**Callers** (shape-changing symbols): `Server.Prune` ← internal/cli/snapshots_prune.go:63 (LSP); `Server.PlanPrune` ← snapshots_prune.go:65 (grep); `Server.List` ← internal/cli/snapshots.go:53 (LSP); `Server.Sync` ← internal/snapshot/import.go:163 (LSP); `Server.SyncAndImport` ← internal/cli/sync.go:122 (grep); `Server.ImportFrom` ← internal/cli/sync.go:115 (grep); `Store.Replace` ← internal/importer/importer.go:171 via `importer.Store` port (LSP); `entriesAt` ← list.go:217 `markStoreSnapshot` (← `markStore`, auto_prune.go:28) (LSP); `snapshot.ID` ← import.go:218,251,325, from.go:101, list.go:161, outcome.go:39, internal/cli/json_snapshots.go:59 (LSP); `snapshotFilePattern`/`manifestFilePattern` ← list.go:153/146 (grep); `resolveFrom` ← from.go:26 (grep); `report.SnapshotID` ← report/refusal.go:145, cli/render_status.go:83, report/document/summary.go:78, report/document/status.go:117 (grep).

**Tests pinning behaviour that changes**: list_store_identity_test.go:84-92,131-139; destination_test.go (~:94-141); prune_sweep_test.go; from_internal_test.go:17-57; import_from_test.go:383-396; auto_prune_test.go, auto_prune_faults_test.go; sync_cancellation_test.go; prune_store_test.go:206-312; list_test.go:471,484,510; cmd/quarry/run_import_test.go:187-189 and run_store_faults_test.go:80,163-165 (quarry folder now holds `quarry.lock`); run_case_variant_test.go:25-59; sync Long pin (cmd/quarry/run_usage_test.go), prune Long pin (internal/cli/snapshots_prune_test.go:15).

## Product Verdict

Scoping pass: **SHIP WITH CHANGES**, accepted — the changes are the rulings below (8-site case-folding list, D1 duplicate-ID warning, unreadable path marks nothing by ID, lock ordering BR-L3). Mid-scope copy ruling (after sizing): **SHIP WITH CHANGES**, accepted — BR-C6/BR-C7, F1–F6, non-regular entries never compete, `report.SnapshotID` any case, changes 8–9 to `--from`.

## Business rules (rulings as intent; supersede earlier specs, do not edit them)

**A. Lock**
- **BR-L1 (one writer).** At most one quarry command that changes quarry's files runs at a time: `quarry sync` (fresh and `--from`, including its auto-prune) and `quarry snapshots prune` without `--dry-run`. Everything else never opens the lock — `status`, `accounts`, reports, `sql`, `findings`, `snapshots`, `prune --dry-run`, `mcp`. A running MCP server never blocks a sync.
- **BR-L2 (no waiting).** A held lock refuses at once, exit 1. No wait, retry or timeout config.
- **BR-L3 (before any write).** The lock is taken after usage checks and config loading and before anything reads Quicken or a snapshot or writes a file: before bundle resolution and `--from` resolution, so before `Destination.Prepare` (internal/snapshot/destination.go:45-51, mkdir + leftover sweep) and `Replace` (duckstore.go:330-333); for prune before `planPrune` (prune.go:69) and `sweepOrphans` (prune.go:137). This ordering makes "changed nothing"/"deleted nothing" true; cite these lines beside those strings. Refusal order: usage (2) → config (1) → lock (1) → everything else.
- **BR-L4 (mechanism).** OS advisory `flock(LOCK_EX|LOCK_NB)` (not fcntl — flock is per open file description, so an in-process acceptance test can hold it through a second open). Kernel releases it on exit, so kill -9 leaves no stale lock. Taken once per command, held to exit; auto-prune runs under sync's lock and never re-takes it.
- **BR-L5 (the file).** `~/Library/Application Support/quarry/quarry.lock`, beside `quarry.duckdb`; created 0600, empty, never written, never deleted (leaving it is accepted). Opened for reading is enough, so a read-only lock file locks fine. Must be a regular file, checked with `Lstat` (symlink refused). sync creates the quarry folder 0700 if missing (as destination.go:46 / duckstore.go:331 do). **prune never creates the quarry folder**: folder missing → no lock, existing no-snapshots line.
- **BR-L6 (closes debts).** phase2c STATE.md:79-80 close at SHIP. phase2c spec :9 superseded. phase0 BR-14 (phase0-snapshot/specification.md:34) and row :286 superseded: a second sync now refuses on the lock. Exclusive-create naming and the 1-hour age-gated sweep stay as defence in depth.

**B. Extension letter case**
- **BR-C1 (intent).** The letter case of a snapshot's file extension never matters: `<id>.sqlite`, `<id>.SQLITE`, `<id>.Sqlite` are the same snapshot `<id>`; same for its manifest `<id>.json` in any case. quarry still writes only lowercase. The ID part stays strict (`\d{8}T\d{6}Z(_\d+)?`).
- **BR-C2 (one file per ID).** Exact lowercase `.sqlite` wins; otherwise the name sorting first in byte order (`.SQLITE` beats `.Sqlite`). Other variants are strays: not listed, not in `total_bytes`, never pruned or deleted, warning D1. Manifest chosen by the same rule.
- **BR-C3 (on-disk names out).** Every path quarry reports or removes is the on-disk name, never rebuilt as `id+".sqlite"`: `snapshots --json` `path`/`manifest`, `prune --json` `deleted[]`/`would_delete[]`/`failed[]` `path`, sync `pruned.deleted[].path`, and the remove calls.
- **BR-C4 (orphans).** A `.json` of any case is an orphan only when no snapshot file of that ID exists in any case and no `.<id>.sqlite.partial` exists. Swept as today; removes every orphan variant.
- **BR-C5 (new names).** A new snapshot never takes an ID any file in the folder already uses in any case; next `_N` instead (case-sensitive volume, same-second collision only).
- **Sites** (each a defect today): destination.go:24 `snapshotFilePattern`, :27 `manifestFilePattern`; list.go:84 entry path `id+".sqlite"`; list.go:148 exact-case orphan check; import.go:155 `ID()` (feeds `--from <id>` hints in V1/import-failure/stdout-write refusals and `store_snapshot.id`, json_snapshots.go:59); internal/report/report.go:12,61-64 `SnapshotID` (status Snapshot line, R2 hint); from.go:78-89 — path-form test matches `.sqlite` any case; ID form resolves through the BR-C2 winner (case-sensitive volume with only `X.SQLITE` resolves); manifest = strip extension (any case), find `.json` any case, lowercase first (`TrimSuffix(".sqlite")` gives `X.SQLITE.json`); json_snapshots.go:68 manifest path; destination.go:93 `fileExists` collision check per BR-C5.

**C. Unreadable recorded store path**
- **BR-U1 (intent).** Prune deletes only when it can rule out that a folder entry is the store's file. If the store's recorded `snapshot_path` cannot be stat'ed for any reason other than does-not-exist (`errors.Is(err, fs.ErrNotExist)`) — EACCES, ELOOP, ENOTDIR, ENAMETOOLONG — quarry cannot tell, and marks/protects nothing on that basis. Not-exist keeps today's U6 path-then-ID behaviour.
- **BR-U2.** In that case the U6 ID fallback (list.go:243) does not apply: listing marks nothing even when an ID matches. One rule for `snapshots` and `prune`.
- **BR-U3.** `store_snapshot` still names the recorded `{id, path}`, e.g. `"store_snapshot":{"id":"20260101T000000Z","path":"/Volumes/Backup/20260101T000000Z.sqlite"}`, every entry `"store": false`.
- **BR-U4 (auto-prune).** Auto-prune's recorded path is the snapshot sync just hashed (from.go:41) or wrote; a stat failure there is a race only. Auto-prune then deletes nothing: `pruned` = `{"keep":N,"deleted":[],"failed":[]}`, no `Pruned` line, **no new warning**, exit 0. Safety branch stays in code; its test may be declared unreachable or injected through a fake.

## Surface & Copy

Paths `~`-abbreviated on stderr, absolute in `--json` and `warnings[]` (2a rule). Paths print raw, not `%q` (from.go:114,120 precedent).

### Lock refusals (stdout empty, also under `--json` — no document; precedent sync.go:99-131, phase2c :244)

| # | Cause | Command | stderr (one line) | Exit |
|---|---|---|---|---|
| L1s | lock held by another sync or prune | `sync`, `sync --from` | `quarry: another quarry sync or quarry snapshots prune is running, so this sync changed nothing; run the command again once that one finishes` | 1 |
| L1p | lock held | `snapshots prune` | `quarry: another quarry sync or quarry snapshots prune is running, so this prune deleted nothing; run the command again once that one finishes` | 1 |
| L2 | quarry folder missing and cannot be created | `sync` only | `quarry: cannot create ~/Library/Application Support/quarry: <OS reason per G1>; make ~/Library/Application Support writable by your user, then run the command again` | 1 |
| L3 | `quarry.lock` exists and is not a regular file (directory, symlink, fifo, socket; Lstat) | both | `quarry: ~/Library/Application Support/quarry/quarry.lock is not a regular file; remove it, then run the command again` | 1 |
| L4a | `quarry.lock` missing and cannot be created | both | `quarry: cannot create ~/Library/Application Support/quarry/quarry.lock: <OS reason per G1>; make ~/Library/Application Support/quarry writable by your user, then run the command again` | 1 |
| L4b | `quarry.lock` exists and cannot be opened | both | `quarry: cannot open ~/Library/Application Support/quarry/quarry.lock: <OS reason per G1>; make it readable by your user, or remove it, then run the command again` | 1 |
| L5s | flock fails other than would-block (ENOTSUP, ENOLCK) | `sync` | `quarry: cannot lock ~/Library/Application Support/quarry/quarry.lock: <OS reason per G1>, so this sync changed nothing; ~/Library/Application Support/quarry must be on a disk that supports file locks` | 1 |
| L5p | same | `prune` | `quarry: cannot lock ~/Library/Application Support/quarry/quarry.lock: <OS reason per G1>, so this prune deleted nothing; ~/Library/Application Support/quarry must be on a disk that supports file locks` | 1 |

One shared lock-held line per command; it does not name the holder (no pid in the file).

### Help Long changes
- **sync Long** (internal/cli/sync.go:66-69): new paragraph right after the auto-prune paragraph:
```
Only one quarry sync or quarry snapshots prune runs at a time; while one
is running, another stops at once and changes nothing.
```
- **prune Long** (internal/cli/snapshots_prune.go:30-35): same two lines as a paragraph after the first paragraph; replace the `--dry-run` line with:
```
With --dry-run, prune lists what it would delete and deletes nothing; it
runs even while a sync is running.
```
- `--from` flag help unchanged. `snapshots` Long unchanged.

### `.SQLITE` copy
- Text listing: ID column `20260927T143005Z`, never the extension; row identical to a lowercase one.
- `--json`: `"path":"/Users/…/snapshots/20260927T143005Z.SQLITE"`, `"manifest":"/Users/…/snapshots/20260927T143005Z.json"` (on-disk names); `id` has no extension.
- `sync --from 20260927T143005Z` with only `.SQLITE` present: succeeds; output and `import_runs.snapshot_path` carry the on-disk path; `status` Snapshot line `20260927T143005Z`.
- `sync --from 20260927T143005Z.SQLITE` (no slash): a path relative to the current directory, as `.sqlite` is today; missing → existing `quarry: ~/…/20260927T143005Z.SQLITE does not exist; check the path passed to --from` (from.go:114), exit 1.
- Prune / Pruned lines show the ID; deletes the `.SQLITE` file, then its manifest (any case).
- **D1 duplicate-ID warning** (`snapshots` only; exit 0; one per affected ID, in listing order):
  - Two names: `quarry: warning: ~/Library/Application Support/quarry/snapshots holds both 20260927T143005Z.sqlite and 20260927T143005Z.SQLITE; quarry lists, prunes and uses only 20260927T143005Z.sqlite; rename or remove the other`
  - Three or more: `quarry: warning: ~/Library/Application Support/quarry/snapshots holds 20260927T143005Z.SQLITE, 20260927T143005Z.Sqlite and 20260927T143005Z.sqlite; quarry lists, prunes and uses only 20260927T143005Z.sqlite; rename or remove the others` (names in byte order; "only" name = BR-C2 winner).
  - Manifest variants: same line with `.json` names.
  - `warnings[]`: same text without `quarry: warning: `, folder absolute.
  - Order: config warnings, no-snapshots note, D1 lines, store warning. Prune and auto-prune stay silent about strays.

### Unreadable recorded path
- **prune** (count > N, real or `--dry-run`): existing cannot-tell refusal (prune.go:183-186) with the phase2c :90 reason:
  `quarry: cannot tell which snapshot the store at ~/Library/Application Support/quarry/quarry.duckdb was built from (cannot read /Volumes/Backup/20260101T000000Z.sqlite: permission denied), so no snapshot was deleted; run quarry sync to rebuild the store, then prune again` — exit 1, stdout empty, no JSON.
- **snapshots:** `quarry: warning: cannot tell which snapshot the store was built from: cannot read /Volumes/Backup/20260101T000000Z.sqlite: permission denied` — exit 0, nothing marked. `warnings[]` carries the recorded path absolute (NoSnapshotsAbsolute precedent, list.go:94); do NOT copy the existing R3 `StoreWarning` (json_snapshots.go:86) abbreviated form — out of scope.
- **prune count ≤ N:** `Nothing to delete: 5 snapshots, within the newest 12`, stderr empty, `store_snapshot` = recorded `{id,path}` (U7, :242 unchanged).
- Reasons via `osreason.Reason`: `permission denied`, `too many levels of symbolic links`, `not a directory`, `file name too long`.

### Edge-case rows

| Input | Outcome | Exit |
|---|---|---|
| sync while another sync holds the lock | L1s; no snapshot, no leftover sweep, store unchanged | 1 |
| sync while a prune holds it | L1s | 1 |
| `sync --from X` while locked | L1s, checked before `--from` resolution (even unknown X) | 1 |
| prune while a sync (or its auto-prune) holds it | L1p; nothing deleted, no orphan sweep | 1 |
| prune, count ≤ N, while locked | L1p (lock before planning) | 1 |
| `prune --dry-run` while locked | runs normally, unlocked; no note | 0 |
| `prune --keep 0` while locked | usage line (usage before lock) | 2 |
| malformed config while locked | C1 (config before lock) | 1 |
| holder killed with kill -9 | kernel releases; `quarry.lock` stays; next run proceeds silently | 0 |
| `quarry.lock` is a directory or symlink | L3 | 1 |
| `quarry.lock` mode 0400 | locks fine | 0 |
| `quarry.lock` mode 0000 | L4b | 1 |
| quarry folder read-only, no `quarry.lock` | L4a | 1 |
| prune, quarry folder missing | `Nothing to delete: no snapshots in ~/Library/Application Support/quarry/snapshots`; creates nothing | 0 |
| first-ever sync, folder missing | folder 0700, `quarry.lock` 0600, proceeds | 0 |
| status/sql/reports/MCP while a sync runs | unaffected | as today |
| only `X.SQLITE` (+ `X.json`) | listed as X, counted, prunable, `--from X` works, `X.json` not an orphan | 0 |
| `X.SQLITE` + `X.sqlite` (case-sensitive volume only; Linux tests) | `X.sqlite` listed; `X.SQLITE` stray, never deleted; D1 | 0 |
| `X.SQLITE` + `X.Sqlite`, no lowercase | `X.SQLITE` wins; D1 | 0 |
| `X.json` alone, no snapshot any case, no partial | orphan, swept silently (unchanged) | — |
| `X.JSON` beside `X.sqlite` | X's manifest | 0 |
| recorded path ENOENT | unchanged U6 path-then-ID | 0 |
| recorded path EACCES/ELOOP/ENOTDIR, count > N | cannot-tell refusal with reason | 1 |
| same, `snapshots` | warning, nothing marked, `store_snapshot` recorded | 0 |
| same, inside sync's auto-prune (race) | deletes nothing, empty `pruned` lists, no warning | 0 |

### Changes to existing surfaces
1. sync Long and prune Long: paragraphs above.
2. internal/snapshot/doc.go:9-11: add one sentence that sync's store build and Prune each run under quarry's lock file (developer's wording within budget).
3. `report.SnapshotID` doc (report.go:61-62) and `ID()` doc (import.go:152-153) → "its extension, in any letter case".
4. Superseded, not edited: phase0 BR-14 (:34) and row :286; phase2c P2c-5 (:18) and Out of Scope :9.
5. Debts closed at SHIP: phase2c STATE.md:79-80 (lock), plus #6 and #7 (case, unreadable path).
6. Still true, no change: SKILL.md:16; references/monthly-summary.md:44-46; duckstore.go:41; destination.go:30.
7. MCP and skill: no new strings, no tool changes.

### User-verified, outside the pipeline
- none. Case-sensitive-volume rows (two letter cases of one ID): macOS default APFS is case-insensitive and there is no Linux CI, so a test that skips there never runs. Pin them through the folder-scan/name-selection logic with a directory listing that is injected (or a pure function over names), not a real case-sensitive volume; on-disk tests cover single-variant `.SQLITE`.

### `--from` resolution (mid-scope ruling, 2026-10-06)

- **BR-C6 (ID-form `--from` resolves what `snapshots` lists).** `sync --from <id>` lists the snapshots folder once and resolves `<id>` to the BR-C2 winner among the **regular** entries for that ID — the same file `quarry snapshots` lists. From there, today's outcomes on the on-disk path (Stat, manifest, hash, content refusals, from.go:32-54). Folder cannot be read → quarry cannot tell which file is `<id>` and acts on nothing (BR-U1); no fallback to a lowercase `<id>.sqlite` lookup.
- **BR-C7 (path-form manifest).** `sync --from <dir>/<id>.<sqlite any case>` strips one `.sqlite` extension in any case and lists `<dir>` to find `<stem>.json` in any case, exact lowercase first, else byte order (BR-C2). Every entry type competes for the manifest (`readManifest` follows symlinks; a directory gives today's unreadable line). No match → existing `… is not a quarry snapshot (no .json manifest next to it); …` (from.go:153, 176).
- The lock is already held at every refusal below (BR-L3). Lines do **not** say "changed nothing" (a first-ever `sync --from` has already created the quarry folder and `quarry.lock`).

| # | Input | stderr (one line; stdout empty, also under `--json`) | Exit |
|---|---|---|---|
| F1 | `--from <id>`, snapshots folder cannot be listed (EACCES, ENOTDIR, ELOOP) | `quarry: cannot read ~/Library/Application Support/quarry/snapshots: <OS reason per G1>` — the listing's `folderUnreadableRefusal` (list.go:178-184) verbatim, no tail | 1 |
| F2 | `--from <id>`, snapshots folder missing | unchanged: `quarry: no snapshot <id> in ~/Library/Application Support/quarry/snapshots; run quarry snapshots to list the ones kept` (from.go:118-121) | 1 |
| F3 | `--from <id>`, folder listable, no regular entry for `<id>` in any case | F2 line | 1 |
| F4 | path form, snapshot's parent folder cannot be listed (searchable, not readable, e.g. 0300) | `quarry: cannot read <parent folder, ~-abbreviated>: <OS reason per G1>`, e.g. `quarry: cannot read ~/Backups: permission denied` — same helper as F1 | 1 |
| F5 | path form, parent listed, the manifest found cannot be read | unchanged: `quarry: cannot read <on-disk manifest path>: <reason>; check the file's permissions` (from.go:186-191) | 1 |
| F6 | snapshots folder 0300, `--from <id>` | F1 (`permission denied`); accepted change (quarry creates that folder 0700, destination.go:46) | 1 |
| F7 | path form with a relative value (e.g. `x.sqlite`, `snaps/x.sqlite`) and the current folder cannot be resolved (`filepath.Abs` → `os.Getwd` fails: darwin cwd without search permission; Linux also cwd removed) | `quarry: cannot resolve x.sqlite against the current folder: <OS reason per G1>; pass --from an absolute or ~/ path instead`, e.g. `quarry: cannot resolve x.sqlite against the current folder: permission denied; pass --from an absolute or ~/ path instead` — value shown as given; `causedRefusalError`; `osreason.Reason` also unwraps `*os.SyscallError` so Linux renders `no such file or directory`, not `getwd: …` (gate ruling, 2026-10-06) | 1 |

- **Non-regular `<id>.SQLITE`** (symlink, directory, fifo) in the snapshots folder: never competes for the BR-C2 winner; **no D1** (D1 counts regular entries only); never listed, counted, pruned or deleted (today's skip, list.go:153-155). It **does block the orphan sweep**: BR-C4's "no snapshot file of that ID" means no entry of any type named `<id>.sqlite` in any case (today's `names` map holds every entry type, list.go:139-148). `--from <id>` whose only entry is non-regular → F3.
- **`report.SnapshotID`** (report.go:61-64) strips one trailing extension equal to `.sqlite` in any letter case (`strings.EqualFold` on `filepath.Ext`); no ID pattern match. `20260927T143005Z.SQLITE` → `20260927T143005Z`; `latest.Sqlite` → `latest`; other names → base name unchanged. Doc: "its file name without the .sqlite extension, in any letter case". Status line, byte-identical to a lowercase-recorded store's (render_status.go:24,82-88,110-112):
  ```
  Snapshot  20260927T143005Z, taken 2026-09-27 10:30 EDT (8 days ago)
  ```
  R2 rebuild hint (refusal.go:140-146) renders `--from 20260927T143005Z`, which resolves through BR-C6. `--json` values only, no key changes: `status --json` `snapshot.id` → `"20260927T143005Z"`, `snapshot.path` stays on-disk absolute `…/20260927T143005Z.SQLITE` (document/status.go:117-118); `summary --json` and MCP `monthly_summary` `snapshot.id` → `"20260927T143005Z"` (document/summary.go:78).

Edge rows (additional):

| Input | Outcome | Exit |
|---|---|---|
| store recorded at `X.SQLITE`, `status` | `Snapshot  X, taken …`; `--json` `snapshot.id` = `X`, `snapshot.path` = on-disk `.SQLITE` path | 0 |
| store recorded at `X.SQLITE`, a command refusing with R2 | hint `--from X`, which succeeds | 1 |
| `--from X`, folder unlistable | F1 | 1 |
| `--from /d/X.SQLITE`, `/d` searchable not listable | F4 | 1 |
| only a directory or symlink named `X.SQLITE` (or `X.sqlite`) in the folder | not listed, no D1, `X.json` not an orphan; `--from X` → F3 | 0 / 1 |
| `latest.sqlite` + `latest.json` in the snapshots folder, `--from latest` | F3; path form `--from …/snapshots/latest.sqlite` succeeds | 1 / 0 |
| `--from x.sqlite` run from a folder with no search permission (or, on Linux, removed) | F7; `--from /abs/x.sqlite` or `--from ~/x.sqlite` from the same folder succeeds | 1 / 0 |

Changes to existing surfaces (additional):
8. A directory named `<id>.sqlite` in the snapshots folder passed as `--from <id>` gave `… is not a snapshot file; …` (from.go:106,124-127); now F3. Re-point import_from_test.go:383-396 to the path form (keeps the not-a-snapshot-file pin) and add an ID-form F3 case.
9. A symlink named `<id>.sqlite` in the snapshots folder was accepted by `--from <id>` (os.Stat follows it); now F3, since `snapshots` never listed it. The path form still follows the link and succeeds (from.go:80-83,95).
10. (orchestrator, from the SCENARIO-12a checkpoint, 2026-10-06) A hand-placed non-ID name in the snapshots folder (`latest.sqlite` + `latest.json`) passed as `--from latest` used to resolve (join + os.Stat); it now gives F3 (`quarry: no snapshot latest in ~/Library/Application Support/quarry/snapshots; run quarry snapshots to list the ones kept`), because `snapshots` never lists it (BR-C6). The path form `--from ~/…/snapshots/latest.sqlite` still resolves.

---

## Scenarios (Gherkin)

```gherkin
Scenario: SCENARIO-01 A sync refuses while another sync or prune is running
  Given another quarry sync holds quarry's lock
  When the user runs `quarry sync`
  Then it prints the lock-held line L1s and exits 1, taking no snapshot and leaving the store unchanged

Scenario: SCENARIO-02 A sync --from refuses while locked, before resolving the snapshot
  Given another writer holds the lock and the snapshot id does not exist
  When the user runs `quarry sync --from 20990101T000000Z`
  Then it prints L1s, not the unknown-snapshot line, and exits 1

Scenario: SCENARIO-03 Prune refuses while locked and deletes nothing
  Given a sync holds the lock and the folder holds more snapshots than --keep
  When the user runs `quarry snapshots prune`
  Then it prints L1p, exits 1, and deletes no snapshot or orphan manifest

Scenario: SCENARIO-04 Prune --dry-run runs while a sync is running
  Given a sync holds the lock
  When the user runs `quarry snapshots prune --dry-run`
  Then it lists what it would delete and exits 0

Scenario: SCENARIO-05 A lock left by a finished or killed run does not block
  Given quarry.lock exists from an earlier run that has exited
  When the user runs `quarry sync`
  Then the sync proceeds as normal

Scenario Outline: SCENARIO-06 An unusable lock file refuses with a fix
  Given quarry.lock is <state>
  When the user runs `quarry sync`
  Then it prints <line> and exits 1

  Examples:
    | state                          | line |
    | a directory                    | L3   |
    | a symlink                      | L3   |
    | mode 0000                      | L4b  |
    | missing in a read-only folder  | L4a  |

Scenario: SCENARIO-07 Usage and config errors come before the lock
  Given another writer holds the lock
  When the user runs `quarry snapshots prune --keep 0`
  Then it prints the usage line and exits 2

Scenario: SCENARIO-08 Reads and the MCP server are never blocked
  Given a sync holds the lock
  When the user runs `quarry status`
  Then it reports from the current store and exits 0

Scenario: SCENARIO-09 Help says only one writer runs at a time
  Given the shipped binary
  When the user runs `quarry sync --help`
  Then the Long carries the one-writer paragraph, and prune --help carries its paragraph and the new --dry-run line

Scenario: SCENARIO-10 An upper-case .SQLITE snapshot is listed with its on-disk paths
  Given the folder holds 20260927T143005Z.SQLITE and its .json
  When the user runs `quarry snapshots --json`
  Then it lists id 20260927T143005Z with path …/20260927T143005Z.SQLITE, counted in total_bytes

Scenario: SCENARIO-11 Prune deletes an .SQLITE snapshot and its manifest, never sweeping it as an orphan
  Given more snapshots than --keep, the oldest named <id>.SQLITE
  When the user runs `quarry snapshots prune`
  Then it deletes <id>.SQLITE and then <id>.json, and keeps every newer manifest

Scenario: SCENARIO-12a sync --from <id> resolves an .SQLITE snapshot
  Given only 20260927T143005Z.SQLITE and its manifest exist for that id
  When the user runs `quarry sync --from 20260927T143005Z`
  Then the store is built from it, recording the on-disk path

Scenario: SCENARIO-12b Status names a store built from an .SQLITE snapshot by its id
  Given the store was built from 20260927T143005Z.SQLITE
  When the user runs `quarry status`
  Then the Snapshot line reads `Snapshot  20260927T143005Z, taken …`

Scenario: SCENARIO-13 Two letter cases of one id: one is listed, the user is warned
  Given the folder lists both <id>.sqlite and <id>.SQLITE
  When the user runs `quarry snapshots`
  Then only <id>.sqlite is listed and warning D1 names both

Scenario: SCENARIO-14 Prune refuses when the store's recorded snapshot cannot be read
  Given the store recorded a snapshot path whose folder denies access, and more snapshots than --keep
  When the user runs `quarry snapshots prune`
  Then it prints the cannot-tell refusal with "cannot read <path>: permission denied", deletes nothing, and exits 1

Scenario: SCENARIO-15 Listing warns and marks nothing when the recorded snapshot cannot be read
  Given the same unreadable recorded path, and an entry with that id in the folder
  When the user runs `quarry snapshots`
  Then it warns "cannot tell which snapshot the store was built from: cannot read …", marks no entry as the store's, and exits 0

Scenario: SCENARIO-16 Auto-prune inside sync deletes nothing when the recorded path cannot be read
  Given the snapshot sync just used becomes unreadable before auto-prune
  When the user runs `quarry sync`
  Then the sync succeeds, and auto-prune deletes nothing and prints no Pruned line or new warning
```

---

## Sizing
| Scenario | Verdict — numbers |
| --- | --- |
| SCENARIO-01 | OWNS A RUN, test-first (exclusive claim) — 3 batches; new `internal/platform/lockfile` (Lstat regular, open read-only create 0600 `O_NOFOLLOW|O_NONBLOCK`, `flock(LOCK_EX|LOCK_NB)`, classified kinds; sync mode creates the quarry folder 0700, prune mode never creates it), `Locker` port in internal/snapshot/ports.go via `WithX` option, copy in `internal/snapshot`, wiring cmd/quarry/run.go:52-77,106-120; sync's acquire after `newServer`, before `ResolveBundle` and `ImportFrom`; released when RunE returns. Acceptance test holds a real lock on `<tmp home>/…/quarry.lock` (also the wiring pin). Absorbs 02, 05, 08; carries the first-sync 0700/0600 row. |
| SCENARIO-02 | FOLD into 01 (lock above `sync.go` `Changed("from")`; unknown id gets L1s). |
| SCENARIO-03 | OWNS A RUN, test-first (delete refusal) — 3 batches; prune RunE acquires only when `!dryRun` (snapshots_prune.go:~61). Absorbs 04, 07, 09; carries "prune with quarry folder missing creates nothing", "count ≤ N while locked → L1p", "malformed config before lock". |
| SCENARIO-04 | FOLD into 03 (dry-run unlocked). |
| SCENARIO-05 | FOLD into 01 (lock file present, no flock held → proceeds). |
| SCENARIO-06 | OWNS A RUN, test-first (Lstat symlink refusal) — 3 batches: primitive classification, per-command copy, cmd pins for both commands; carries L2, L5s/L5p (flock func injected through a constructor option, never a package var), prune variants of L3/L4, 0400 "locks fine" control. |
| SCENARIO-07 | FOLD into 03 (cobra Args before RunE). |
| SCENARIO-08 | FOLD into 01 (reads never call the lock; one test with the lock held). |
| SCENARIO-09 | FOLD into 03 (Long copy; acceptance pins both `sync --help` and `prune --help`; plus doc.go:9-11 sentence). |
| SCENARIO-14 | OWNS A RUN, test-first (destructive guard) — 3 batches; one recorded-path classifier inside `markStore` (list.go:108-120; replaces the swallowed `os.Stat` in `entriesAt`, list.go:257): not-exist keeps U6; anything else sets `StoreUnreadable`, no ID fallback (list.go:243), absolute `warnings[]` form. ENOENT control list_store_identity_test.go:200 stays green. Absorbs 15, 16. |
| SCENARIO-15 | FOLD into 14 (same classifier through `markStore`). |
| SCENARIO-16 | FOLD into 14 (`autoPrune` calls `markStoreSnapshot` directly, auto_prune.go:28 — needs its own classifier call; acceptance via a `WithImporter` fake swapping the just-written snapshot for a self-symlink (ELOOP), not chmod, which reaches `cannotListWarning`). |
| SCENARIO-10 | OWNS A RUN, test-first (decides what prune deletes) — 3 batches; one unexported pure name selector over `ReadDir` names: per ID the BR-C2 winning snapshot and manifest, strays, BR-C4 orphans (partial guard; non-regular entries block orphaning but never compete); replaces list.go:146-157 and destination.go:24,27; `snapshot.ID` any case (import.go:154); on-disk paths list.go:84, json_snapshots.go:68, `deleteSnapshot` (prune.go:148-163). Two-case rows as table tests on the selector. Absorbs 11. |
| SCENARIO-11 | FOLD into 10. |
| SCENARIO-12a | LIGHT — `snapshot` only: from.go:77-89 resolves through the selector (BR-C6), path form any case, manifest any case (BR-C7), F1–F6, changes 8–9. |
| SCENARIO-12b | LIGHT — `internal/report`: report.go:12,61-64 `SnapshotID` any case. |
| SCENARIO-13 | OWNS A RUN, test-first (strays never deleted) — 3 batches: read-dir option on the Server (`WithRemove` precedent) so two-case rows run at the boundary; D1 text, absolute `warnings[]`, warning order; stray-never-pruned pins (prune and auto-prune); BR-C5 via the selector (destination.go:93). |

Order: 01 → 03 → 06 → 14 → 10 → 12a → 12b → 13 (lock first; 14 makes the guard whole before 10 widens the deletable set to `.SQLITE` files; 12a and 13 reuse 10's selector).

**Traps:** a fifo opened read-only blocks without `O_NONBLOCK`; Lstat-then-open races without `O_NOFOLLOW`; "held to exit" means released when RunE returns, or in-process `run()` tests in one binary see a lock that stays held; cmd/quarry tests pinning the quarry folder's contents now see `quarry.lock`; run_store_faults_test.go:80 (folder 0500) now hits L4a first — pre-create `quarry.lock`.

## BDD Acceptance Progress
- [x] SCENARIO-01: A sync refuses while another sync or prune is running — `cmd/quarry/run_sync_lock_test.go` `Test_run_sync_refuses_while_another_writer_holds_the_lock`
- [x] SCENARIO-02: A sync --from refuses while locked, before resolving the snapshot — delivered by SCENARIO-01, `cmd/quarry/run_sync_lock_test.go` `Test_run_sync_from_refuses_on_the_lock_before_resolving_the_snapshot`
- [x] SCENARIO-05: A lock left by a finished or killed run does not block — delivered by SCENARIO-01, `cmd/quarry/run_sync_lock_test.go` `Test_run_sync_proceeds_past_a_lock_left_by_an_earlier_run`
- [x] SCENARIO-08: Reads and the MCP server are never blocked — delivered by SCENARIO-01, `cmd/quarry/run_sync_lock_test.go` `Test_run_status_reports_while_a_sync_holds_the_lock`
- [x] SCENARIO-03: Prune refuses while locked and deletes nothing — `cmd/quarry/run_prune_lock_test.go` `Test_run_snapshots_prune_refuses_while_another_writer_holds_the_lock`
- [x] SCENARIO-04: Prune --dry-run runs while a sync is running — delivered by SCENARIO-03, `cmd/quarry/run_prune_lock_test.go` `Test_run_snapshots_prune_dry_run_runs_while_a_writer_holds_the_lock`
- [x] SCENARIO-07: Usage and config errors come before the lock — delivered by SCENARIO-03, `cmd/quarry/run_prune_lock_test.go` `Test_run_snapshots_prune_refuses_usage_and_config_before_the_lock`
- [x] SCENARIO-09: Help says only one writer runs at a time — delivered by SCENARIO-03, `cmd/quarry/run_usage_test.go` `Test_run_help_says_only_one_writer_runs_at_a_time`
- [x] SCENARIO-06: An unusable lock file refuses with a fix — `cmd/quarry/run_sync_lock_file_test.go` `Test_run_sync_refuses_an_unusable_lock_file_with_a_fix`
- [x] SCENARIO-14: Prune refuses when the store's recorded snapshot cannot be read — `cmd/quarry/run_prune_recorded_test.go` `Test_run_snapshots_prune_refuses_when_the_recorded_snapshot_cannot_be_read`
- [x] SCENARIO-15: Listing warns and marks nothing when the recorded snapshot cannot be read — delivered by SCENARIO-14, `cmd/quarry/run_snapshots_recorded_test.go` `Test_run_snapshots_warns_and_marks_nothing_when_the_recorded_snapshot_cannot_be_read`
- [x] SCENARIO-16: Auto-prune inside sync deletes nothing when the recorded path cannot be read — delivered by SCENARIO-14, `internal/snapshot/auto_prune_recorded_test.go` `Test_sync_and_import_prunes_nothing_when_the_recorded_snapshot_cannot_be_read`
- [x] SCENARIO-10: An upper-case .SQLITE snapshot is listed with its on-disk paths — `cmd/quarry/run_snapshots_upper_case_test.go` `Test_run_snapshots_json_lists_an_upper_case_sqlite_snapshot_with_its_on_disk_paths`
- [x] SCENARIO-11: Prune deletes an .SQLITE snapshot and its manifest, never sweeping it as an orphan — delivered by SCENARIO-10, `internal/snapshot/prune_upper_case_test.go` `Test_prune_deletes_an_upper_case_sqlite_snapshot_then_its_manifest_and_keeps_every_newer_manifest`
- [x] SCENARIO-12a: sync --from <id> resolves an .SQLITE snapshot — `cmd/quarry/run_from_case_test.go` `Test_run_sync_from_an_id_rebuilds_the_store_from_an_upper_case_sqlite_snapshot`
- [x] SCENARIO-12b: Status names a store built from an .SQLITE snapshot by its id — `cmd/quarry/run_status_case_test.go` `Test_run_status_names_a_store_built_from_an_upper_case_sqlite_snapshot_by_its_id`
- [x] SCENARIO-13: Two letter cases of one id: one is listed, the user is warned — `cmd/quarry/run_snapshots_two_case_test.go` `Test_run_snapshots_lists_one_of_two_letter_cases_and_warns_naming_both`
