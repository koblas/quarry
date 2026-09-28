---
id: SCENARIO-01d
status: done
---

# SCENARIO-01d: sync imports the Quicken data into a new store

Size: OWNS A RUN (snapshot sequencing + cli render + cmd wiring + fixture upgrades; multi-package). Sequencing ADR already written: `docs/adr/002-sync-sequences-import-in-snapshot.md`.
Cadence: code-first — no mandatory item touched: the swap/partial adapter (`duckstore`) is 01a's and already test-first; the skip-on-mismatch guard protects quarry's derived store, not a user file, and adds no write path.
Acceptance test: `cmd/quarry/run_import_test.go` `Test_run_imports_the_quicken_data_into_a_new_store`
Narrow loop: `go test ./internal/snapshot/ ./internal/cli/ ./internal/quicken/v9/v9fixture/ ./cmd/quarry/`
Mutation checks: skip-on-mismatch guard in `(*snapshot.Server).SyncAndImport` → `Test_sync_and_import_skips_the_import_on_a_schema_mismatch`; O1/O1b choice on `Outcome.Store` → `Test_stdout_write_refusal_points_to_from_only_once_the_build_was_reached`; S3 refusal wraps the importer error → `Test_sync_and_import_frames_an_import_failure_as_a_store_refusal`

**Before dispatch (orchestrator):** two outcomes have no ruled copy — get a scoped `product-vision` copy ruling, then paste it into `## Surface & Copy`:
1. `Rows` line singulars (`1 transaction`, `1 split`, `1 transfer`, `1 payee`, `1 category`, `1 tag`?). Plan assumes singular at exactly 1, like every other count clause.
2. Placement of `Store`/`Rows` on the extras-only success path. Plan assumes after the `+ table`/`+ column` diff rows (the whole Phase 0 output is the unchanged prefix).

User-visible contract (plain `quarry sync`, `--json` unchanged — manifest only until SCENARIO-02):
- success: Phase 0 block (+ diff rows) then `Store     ~/Library/Application Support/quarry/quarry.duckdb` and `Rows      N transactions, N splits, 0 transfers, N payees, N categories, N tags` (thousands-grouped); exit 0.
- schema mismatch: unchanged (M1); import not attempted, no store lines, no `quarry.duckdb`; exit 1.
- import failure (interim S3 frame, ruled): stdout empty; stderr `quarry: cannot build the store in ~/Library/Application Support/quarry: <reason>; run quarry sync --from <id>`; snapshot + manifest kept; exit 1.
- stdout write fails: O1 when no build was reached (mismatch path), O1b once it was; exit 1.

## Implementation Plan

### Acceptance (red)
- [x] Step 1: new `cmd/quarry/run_import_test.go` `Test_run_imports_the_quicken_data_into_a_new_store` — `v9fixture.NewBuilder()`…`WriteBundle(t, home/Documents)`: CAD + USD accounts, nested categories, ≥2 of every noun, split transactions, payees, tags + `LinkUserTag`, no `ZTRANSFER` legs; run `sync --quicken`; assert exact stdout (Phase 0 block + Store + Rows), empty stderr, exit 0; open `quarry.duckdb` via `internal/platform/duckdb` `OpenReadOnly` and assert every table's ids, `source_id`s and `CAST(amount AS VARCHAR)` amounts
- [x] Step 2: `internal/snapshot/ports.go:1-49` `Importer`; `snapshot.go:22-87` `WithImporter`, `WithStorePath`; new `internal/snapshot/import.go` `Outcome{Manifest, Store *store.Result}`, `(*Server).SyncAndImport` — signature-only (calls `Sync`, never imports). Red at the stdout assertion

### Build
- [x] Step 3: `internal/store/duckstore/duckstore.go:15` export `storeFileName` as `FileName`; `cmd/quarry/run.go:30-49` `newServer` — store dir `home/Library/Application Support/quarry`, `WithImporter(importer.NewServer(importer.WithStore(duckstore.New(dir))))`, `WithStorePath(filepath.Join(dir, duckstore.FileName))`; `var _ snapshot.Importer = (*importer.Server)(nil)` in `run.go`
- [x] Step 4: `import.go` `SyncAndImport` — `Sync`; mismatch → `Outcome{Manifest}` + the `MismatchError`, no import; other `Sync` error → returned unchanged; else `Import(manifest.Snapshot.Path)` → `Outcome.Store`; import error → store refusal in the S3 frame (dir from `WithStorePath`, id = snapshot basename) that **wraps** the cause, never via `FailureOutcome`; no importer → `errNoImporter`. New `internal/snapshot/sync_and_import_test.go` (hand-written fake `Importer` recording calls): `Test_sync_and_import_imports_the_committed_snapshot` (control: called once with the committed path, `Store` = fake's result), `Test_sync_and_import_skips_the_import_on_a_schema_mismatch` (`MissingSchemaBundle`: fake never called, `Store == nil`, `MismatchError`), `Test_sync_and_import_frames_an_import_failure_as_a_store_refusal` (exact S3 text + `errors.Is` sentinel; snapshot + manifest on disk), `Test_sync_and_import_does_not_import_when_the_snapshot_fails`, `Test_sync_and_import_refuses_without_an_importer`
- [x] Step 5: `import.go` `(Outcome).StdoutWriteRefusal(home, err)` — O1 text (moved verbatim from `internal/cli/sync.go:26-32`, then delete it there) when `Store == nil`, O1b when set. `Test_stdout_write_refusal_points_to_from_only_once_the_build_was_reached` (table: nil / non-nil `Store`, exact text each)
- [x] Step 6: `internal/quicken/v9/v9fixture/fixture.go:35-64` `OpenBundle` and `:169-189` `ExtraSchemaBundle` — typed accounts (`ZTYPENAME`, `ZCURRENCY`) + `Z_PRIMARYKEY` rows for CashFlowTransaction, CategoryTag, UserTag; `OpenBundle` keeps its split (WAL-only account still written after the PASSIVE checkpoint; `fixture_test.go:18` stays green). Leave `MissingSchemaBundle` untyped. Before Step 7, so the cmd tests stay green when RunE starts importing
- [x] Step 7: `internal/cli/render.go:42-52` — new `renderStore(store.Result, home)` (Store + Rows lines, `formatThousands` `:17-23`, singular per ruling); `render_internal_test.go` tests: thousands-grouped Rows, singular at 1, zero counts. `internal/cli/sync.go:63-110` RunE — `srv.SyncAndImport` (`:80`); human output = `renderSuccess` + `renderStore` when `Outcome.Store != nil`; `--json` unchanged; stdout failure `:98-100` → `outcome.StdoutWriteRefusal`; mismatch return `:106` unchanged
- [x] Step 8: cmd tests — `run_success_test.go:32-62` exact stdout + `Store` and `Rows      0 transactions, 0 splits, 0 transfers, 0 payees, 0 categories, 0 tags`; `:95-112` rename to `Test_run_points_to_from_when_writing_stdout_fails_after_the_build`, assert O1b; `:114-133` passes unchanged (manifest-only JSON); `run_schema_test.go:74-118` extras exact stdout gains Store/Rows (placement per ruling); `:24-72` add "no `quarry.duckdb`"; new `Test_run_keeps_the_snapshot_message_when_writing_stdout_fails_on_a_schema_mismatch` (O1, `MissingSchemaBundle` + `failingWriter`); in `run_import_test.go` `Test_run_refuses_an_unmappable_value_and_keeps_the_snapshot` (EUR account: reason substring only, stdout empty, snapshot + manifest kept, no `quarry.duckdb`, no `.partial`)

### Sweep
- [x] Step 9: fix what `go build ./... && golangci-lint run ./...` reports; `internal/snapshot/doc.go:1-6` package comment now covers the import sequence; `internal/cli/run.go:16-19` `Execute` doc names `SyncAndImport`; doc comments on every new exported symbol. No help-text edits (01b/03 own them)

### Verify
- [x] Step 10: full verification (`.claude/rules/agent-briefs.md`); `git add` new files before `uncovered-diff.py`; `go list -deps -test ./internal/snapshot ./internal/cli ./internal/importer | grep duckdb` → 0 hits, `go list -deps ./cmd/quarry | grep duckdb` → hits (control); `spec-check.py phase1-import-store` → tick SCENARIO-01d with its acceptance test (not SCENARIO-18)

## Handoff

**Binding decisions** — a later scenario must not contradict these without saying so:
- Sequencing per ADR-002: `snapshot.Importer` (one method, shaped to `importer.Server.Import`), `WithImporter`, `WithStorePath`, `(*Server).SyncAndImport(ctx, bundlePath) (Outcome, error)`; `Sync` unchanged — 03's `--from` entry reuses the import step and `Outcome`.
- `Outcome.Store == nil` ⇔ no build reached; O1/O1b is `(Outcome).StdoutWriteRefusal` — 01b's V1 must set `Store` (with a not-built flag) so V1's stdout failure gets O1b.
- Store path enters snapshot only via `WithStorePath(filepath.Join(dir, duckstore.FileName))` — S1–S3 copy uses its dir, S4/V1/I2 its file; snapshot never spells `quarry.duckdb`.
- Import failures return a store refusal that wraps the cause (`errors.As` reaches `*importer.UnmappableError`), never through `FailureOutcome` — the snapshot stays committed.
- `cli.ServerFactory` unchanged; `cli` renders `Outcome`, decides nothing.

**Left unbuilt** — named so nobody assumes it exists:
- S4 frame, S1/S2 classification, I2 pre-swap ctx check — SCENARIO-14 (until then every import failure, incl. a post-commit cancel, prints in the S3 frame).
- `--json` `store` key and `store: null` on mismatch — SCENARIO-02 (SCENARIO-18's acceptance test is 02's; 01d builds the skip but does not tick 18).
- Balances/Splits/Transfers lines, V1 — 01b/01c; `; N investment transactions not imported` — 01c.
- sync help text (01b), `--from` flag (03).

**Traps** — things that look right and are not:
- `MissingSchemaBundle` has no `Z_PRIMARYKEY` rows: removing the skip still fails (reason 7) and creates no store, so cmd-level "no store" assertions cannot catch it — the snapshot-level fake test is the proof.
- O1b and S3 copy tell the user to run `--from`, which only exists after 03 — ruled copy; do not "fix" it.
- `cmd/quarry`'s test binary now links DuckDB — first outside `duckstore`/`platform/duckdb`; linux-small CI has OOMed linking it (`devenv.nix`). macOS Verify will not show it.
- `go list -deps` without `-test` misses test-only imports; the confinement check needs `-test`.
