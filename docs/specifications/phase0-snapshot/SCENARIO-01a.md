---
id: SCENARIO-01a
status: done
---

# SCENARIO-01a: Snapshot of an open file matching the reference

Size verdict: OWNS A RUN, absorbing **FOLD SCENARIO-03** (BR-13). No STATE.md yet; nothing inherited. Every file is new except `go.mod`.

Cadence: test-first — `internal/platform/atomicfile` (exclusive create + no-clobber rename), plus write-safety guards: `mode=ro` open of the live file, and nothing created before the probe (BR-8)
Acceptance test: `internal/snapshot/sync_test.go` `Test_sync_writes_a_verified_private_snapshot_of_an_open_file`
Acceptance test (SCENARIO-03, folded): `internal/snapshot/sync_test.go` `Test_sync_leaves_the_live_bundle_unchanged`
Narrow loop: `go test ./internal/platform/... ./internal/quicken/... ./internal/snapshot/`
Mutation checks:
- backup API swapped for a byte copy of `data` → `Test_sync_writes_a_verified_private_snapshot_of_an_open_file` (WAL-only row missing)
- `mode=ro` dropped from the read-only DSN → `Test_open_read_only_refuses_writes`
- SCENARIO-03 control: inject `PRAGMA wal_checkpoint(TRUNCATE)` on the live connection (error ignored), then toggle only `mode=ro`. Kept → `Test_sync_leaves_the_live_bundle_unchanged` green; dropped → red. Also run the plain `mode=ro` drop with no injection: the prediction is green (reads never write `data`, and the holder blocks the checkpoint on close). That result is expected; report it.
- dir creation moved before the probe → `Test_sync_creates_nothing_when_the_probe_fails`
- no-clobber commit swapped for `os.Rename` → `Test_commit_refuses_to_replace_an_existing_file`; `O_EXCL` dropped → `Test_create_refuses_an_existing_file`

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `go.mod:5-10` — `go get github.com/mattn/go-sqlite3`. If the sandbox denies it, re-run with allowed_domains `proxy.golang.org`, `sum.golang.org`, `storage.googleapis.com`. Then read the driver's `(*SQLiteDriver).Open` in the module cache and list every statement it runs on connect (see Traps).
- [x] Step 2: `internal/quicken/v9/v9fixture/fixture.go` (new) `OpenBundle(tb, dir)` — the BR-3 bundle `<dir>/Home.quicken/data`. Schema from `v9.ReferenceDDL`, WAL mode, autocheckpoint off, 1 account checkpointed into `data`, and a second WAL-only account with its name exported. The holder stays open until `tb.Cleanup`.
- [x] Step 3: `internal/snapshot/sync_test.go` (new) — both acceptance tests. The snapshots dir path contains a space (`…/Application Support/quarry/snapshots`). The 03 test compares only `data` bytes, `data` mtime and the bundle's name set.
- [x] Step 4: signature-only stubs so the tests compile: `v9.ReferenceDDL`/`ReferenceLabel`/`Reference`, `snapshot.NewServer`/`WithSnapshotDir`/`WithReference`/`(*Server).Sync`/`Manifest`. Run both tests; they must fail at their assertions.

### Build
- [x] Step 5: `internal/platform/atomicfile/atomicfile.go` (new) — exclusive `0600` create of a partial, and a no-clobber commit that returns `fs.ErrExist` when the target already exists. Red first: `Test_create_refuses_an_existing_file`, `Test_commit_refuses_to_replace_an_existing_file`, `Test_commit_moves_the_partial_into_place`, plus a fault test for a missing partial.
- [x] Step 6: `internal/platform/sqlschema/{doc,schema,fingerprint,diff}.go` (new) — `Schema` (table → columns); `Fingerprint` per BR-7; `Diff` with missing/unexpected tables and columns, a table-level miss suppressing its columns, bytewise-sorted lists that are never nil. Pinned vector computed outside Go (`printf 'ZA.X\nZA.Y\n' | shasum -a 256`), plus one case where different schemas give different prints. Diff table test covers each list and the suppression.
- [x] Step 7: `internal/platform/sqlite/{doc,sqlite}.go` (new):
  - `OpenReadOnly`: URI DSN with `mode=ro` and the path escaped; one connection; a probe that reads `sqlite_master`.
  - `OpenMemory` + `Exec`.
  - `Backup(ctx, src, destPath)`: backup API into an existing file, then `journal_mode=DELETE` on the destination.
  - `(*DB).Schema` via `pragma_table_info(?)`, and `QueryInt`.
  - Tests: `Test_open_read_only_refuses_writes`, a probe fault (non-SQLite bytes), a backup fault (dest dir missing), a schema read of quoted names, and the backup result being DELETE mode.
- [x] Step 8: `internal/quicken/v9/{doc,reference}.go` (new) — `//go:embed reference.sql`, `ReferenceLabel = "hardkoded/quicken-skills@752107b"`, and `Reference(ctx)` that executes the DDL in memory and introspects it. Tests:
  - A malformed-DDL fault test through the unexported helper.
  - `v9fixture` self-test: a byte copy of `data` alone has 1 account, while the live bundle has 2.
- [x] Step 9: `internal/snapshot/scope.go` (new) — unexported BR-6 scope filter, applied to both sides before fingerprint and diff. Test `Test_scope_keeps_Z_tables_only` covers `Z_PRIMARYKEY`, `Z_15USERTAGS`, `Z_METADATA`, `Z_MODELCACHE`, `sqlite_sequence` and a non-`Z` table. Pin the reference's scoped counts, taken from the sqlite3 CLI and README: **82 tables, 1,835 columns**.
- [x] Step 10: `internal/snapshot/{doc,ports,source,destination}.go` (new):
  - `Source` port and its adapter over `platform/sqlite`.
  - `Destination` port and its dir adapter: `0700` `MkdirAll`, a backup into the `0600` exclusive partial, manifest partial write, both commits via `atomicfile`.
  - Fault tests through fakes: a failure from each Destination method and from Source open/backup propagates wrapped.
- [x] Step 11: `internal/snapshot/manifest.go` (new) — `Manifest` with the exact JSON keys from Surface & Copy `--json`; a single `Encode()` (2-space indent, trailing `\n`); column entries `{"table","column"}`; empty lists encode `[]`. `Test_manifest_encodes_empty_lists_as_arrays`, plus a golden check on key order.
- [x] Step 12: `internal/snapshot/snapshot.go` (new) — `NewServer`, the options, and `Sync(ctx, bundlePath) (Manifest, error)` in BR-8 order: probe, prepare dir, backup, count `ZACCOUNT`, SHA-256, schema diff, commit manifest, commit snapshot.
  - Name is `20060102T150405Z` in UTC at sync start; `taken_at` is the same instant.
  - Tests: `Test_sync_creates_nothing_when_the_probe_fails` (garbage `data`, snapshots dir absent afterwards), plus a missing-reference error test.
  - Both acceptance tests go green here.

### Sweep
- [x] Step 13: fix what `go build ./... && golangci-lint run ./...` reports; `doc.go` per package; doc comments on every exported symbol, checked with `go doc`.

### Verify
- [x] Step 14: full verification (`.claude/rules/agent-briefs.md` → *Verification*, `-race` on the new packages), the listed mutations, and `.claude/scripts/spec-check.py phase0-snapshot`. Then tick SCENARIO-01a with its acceptance test, and SCENARIO-03 with "delivered by SCENARIO-01a" plus its folded test. **If the 03 test cannot pass against correct `mode=ro` code, STOP and report — do not tick 03, do not improvise.**

## Handoff

**Binding decisions:**
- `internal/quicken/v9` (+ `v9fixture`) is a data package, not a feature package. Production `internal/snapshot` never imports it: the reference arrives via `WithReference(label, sqlschema.Schema)`. Only tests and `cmd/quarry` (01b) import v9.
- BR-6 scope lives in `snapshot`; `sqlschema` stays generic. Fingerprint and diff both run on scoped schemas.
- `Manifest.Encode()` is the only encoder. The file bytes equal the bytes 01b prints (BR-10), and lists are never `null`.
- Commit order is manifest first, then snapshot. A snapshot without a manifest is never accepted (BR-8).
- `Destination` owns every write into the snapshots dir, including the backup into the partial. That is BR-11's fault seam: an R14 fake returns ENOSPC from its backup or manifest method.
- `Sync` takes a resolved bundle path. Discovery, `~/` expansion and R4–R7 validation belong to 04/05.

**Left unbuilt:**
- Error classification, refusal copy and exit codes: R8 (06), R9 plus a busy-timeout constant and `WithBusyTimeout` (07), R10/R11/R12 including `integrity_check` (08), R13 (13), and M1/W1 plus the outcome policy (10/09).
- R14 classification and its unit test: **unowned** in BDD Progress; the orchestrator assigns it.
- Removing partials after a failure: owned by the first rejection that happens after the partial exists (08), not by 12.
- `_2`/`_3` collision suffix (11); leftover `.partial` sweep (12); `Schema` table/column counts for the human `Schema` line (01b).

**Traps:**
- SQLite creates db files `0644`. Pre-create the partial `0600` with `O_EXCL`, then back up into it. Journal files inherit the db's mode.
- The backup copies the WAL flag in the header, so the snapshot would spawn `-wal`/`-shm` in the snapshots dir. Set `journal_mode=DELETE` on the destination before hashing.
- Driver on-connect statements (Step 1): anything that alters the live file's persistent state is a BR-1 blocker, so STOP. Connection-level pragmas: report them for a ruling. Pass no `_` DSN params.
- `-shm` changes under read-only readers (read-marks). Never assert on it.
- `:memory:` is per connection under `database/sql`: pin one connection.
- The DSN is a URI: escape the path (spaces, `?`, `#`).
- A crash mid-backup can leave `….sqlite.partial-journal`. 12's pattern must match it.
