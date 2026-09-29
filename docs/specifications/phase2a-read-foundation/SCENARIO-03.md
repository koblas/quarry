---
id: SCENARIO-03
status: done
---

# SCENARIO-03: status --json returns the store description as a document

Size verdict: OWNS A RUN (small, one package: renderer + one branch in `status.go`; nothing in `report`/`store`/`duckstore` changes). Fit for `model: "sonnet"`.
Cadence: code-first (no write-safety guard, no atomic adapter, no bug fix touched)
Acceptance test: `cmd/quarry/run_status_json_test.go` `Test_run_status_json_describes_the_store_sync_built`
Narrow loop: `go test ./internal/cli/ -run 'StatusJSON|renderStatus' && go test ./cmd/quarry/ -run 'run_status'`
Mutation checks: NULL-`taken_at` guard in `newStatusDocument` → `Test_renderStatusJSON` "snapshot time not recorded"; `""`-source guard → its "source not recorded"; `rows.transfers` sourced from `Counts.Transfers` (not `TransfersPaired`) → the "full document" subtest (fixture 3141 vs paired 3112 differ)

## Surface (from specification.md Surface & Copy, `status`)
`quarry status --json`: stdout is one document, same encoder as sync's `renderJSON` (2-space indent, trailing newline), exit 0, stderr empty. Human path unchanged. Key order exactly: `store{path,format_version,quarry_version,built_at,rows{8 keys}}`, `snapshot{id,path,taken_at,source,sha256}`, `dates{first,last}`, `balances{checked,never_reconciled,investment_accounts}`, `splits{checked}`, `transfers{paired,cross_currency,one_sided}`, `not_imported{investment_transactions}`, `warnings`. No `omitempty`. Paths absolute as stored (no `homepath.Abbreviate`, `home` unused). `built_at`/`taken_at` = `.UTC().Format(time.RFC3339)`; `dates` = `2006-01-02` as-is (no `.In(time.Local)`). `taken_at` null when `Run.Snapshot.TakenAt.IsZero()`, `source` null when `== ""` (whitespace-only stays a string), `dates.first/last` null when `FirstDate.IsZero()`; keys always kept. `warnings` is `[]string{}` (encodes `[]`, never `null`).

## Existing surface (survey)
- Reuse as-is: `rowsDocument` + `newRowsDocument(store.Counts)` (`json.go:36-45,154-160`, the 8 keys; `transfers` = `Counts.Transfers` = all transfer rows), `notImportedDocument` (`json.go:113-116`), `jsonDateLayout` (`json.go:13`).
- Do NOT reuse `balancesDocument`/`splitsDocument`/`transfersDocument` (`json.go:47-53,77-81,94-99`): sync's carry mismatch lists; status's are int-only shapes.
- Inputs: `store.Status` fields (`Path, FormatVersion, QuarryVersion, BuiltAt, Run, FirstDate, LastDate`); `report.SnapshotID(Run.Snapshot.Path)`; `Run.Snapshot.{Path,SHA256,TakenAt,Source}`; `Run.BalancesChecked/BalancesNeverReconciled/InvestmentAccounts`, `Run.Counts.Transactions` (splits.checked), `Run.TransfersPaired/TransfersCrossCurrency/TransfersOneSided`, `Run.InvestmentTransactionsNotImported`. All exist (S01/S02); no store/port change.
- `newStatusCommand` has one caller: `root.go:27`. `jsonOut *bool` is the persistent root flag, already threaded into `newSyncCommand` (`root.go:26`).

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_status_json_test.go` (new) `Test_run_status_json_describes_the_store_sync_built` — copy the fixture/sync setup of `run_status_test.go:21-75` (`Test_run_status_describes_the_store_sync_built`); run `status --json`; assert exit 0, stderr empty, stdout equals the exact expected document bytes (proves key order + indent) with store path/snapshot path/source absolute (not `~`), `taken_at` = manifest's `taken_at`, `sha256` = manifest's, `format_version` = `duckstore.FormatVersion`, `built_at` RFC3339 UTC ending `Z`, `rows.transfers` = 2, `balances`/`transfers` values as that fixture's human block. Reuse `onlyFileWithSuffix`, `storePathUnder`, `snapshotID` helpers; `built_at`/`quarry_version` asserted by shape (`(devel)` in test binary; `built_at` parses and is UTC) since they are not fixed.
- [x] Step 2: `internal/cli/status.go:12`, `internal/cli/json_status.go` (new) — signature-only stubs: `newStatusCommand(newReport, jsonOut *bool)` and `renderStatusJSON(st store.Status) ([]byte, error)`; `root.go:27` passes `jsonOut`. Test must fail at its assertion (human block printed), not at compile.

### Build
- [x] Step 3: `internal/cli/json_status.go` — `statusDocument` + `statusStoreDocument`, `statusSnapshotDocument` (`TakenAt *string`, `Source *string`), `statusDatesDocument` (`First, Last *string`), `statusBalancesDocument`, `statusSplitsDocument`, `statusTransfersDocument` types in spec key order; `newStatusDocument(st)` and `renderStatusJSON`. `jsonPayee`-style pointer helper for the string-or-nil fields (reuse `jsonPayee` at `json.go:234-240` only if it reads clean; it maps `""`→nil, exactly the source rule). Encoder identical to `json.go:118-135`; encode-failure branch marked `// unreachable:` like `json.go:131`. Tests in `internal/cli/json_status_internal_test.go` (new) `Test_renderStatusJSON`, subtests over `statusFixture()` (`render_status_internal_test.go:57`): full document exact bytes; no transactions → `"first":null,"last":null`; `TakenAt` zero → `"taken_at":null` and key present; `Source` `""` → `"source":null`; whitespace-only source → string kept; `BuiltAt` given in a non-UTC zone → UTC `Z`; `taken_at` unchanged under `useZone(EDT)` (UTC, never local); warnings encode as `[]`; `dates` unaffected by `useZone`.
- [x] Step 4: `internal/cli/status.go:22-37` — branch on `*jsonOut` between `renderStatusJSON(st)` and `renderStatus(...)`; single `Fprint` (write-fault mapping unchanged: existing `Test_run_status_reports_exit_1_when_stdout_cannot_be_written` covers the shared write).

### Sweep
- [x] Step 5: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments on new types (1-2 lines each); update `newStatusCommand` doc to mention `--json`.

### Verify
- [x] Step 6: full verification per `.claude/rules/agent-briefs.md` → *Verification* (`git add` new files before `uncovered-diff.py`), `.claude/scripts/spec-check.py phase2a-read-foundation`, tick SCENARIO-03 with its acceptance test, set `status: done`, fold Handoff into STATE.md.

## Handoff

**Binding decisions:**
- `status --json` reuses sync's encoder settings (2-space indent, trailing newline) and `rowsDocument`/`notImportedDocument`/`jsonDateLayout`; status's `balances`/`splits`/`transfers` are new int-only types, distinct from sync's list-carrying ones - S07/S09 typed JSON follows the same "document types live in `internal/cli/json*.go`, pointers for nullable, `[]T{}` never nil" rule.
- `warnings` is always `[]string{}` for status - S04-07 `accounts` and S10 `sql` fill their own; do not centralise until a second producer needs it.
- Paths absolute in every `--json` (P2a-8): no `homepath.Abbreviate` on the JSON path.

**Left unbuilt:**
- R1-R3 refusals with `--json` (stdout empty) - S15-17; `status takes no arguments` U8 - S18; U9 hint - S19; O2 text - S21.
- `quarry_version` is emitted as stored; no `--json` test can prove it is wired (see STATE.md trap), only `(devel)` shape.

**Traps:**
- `balances.never_reconciled` in status JSON is an integer count, sync's `store.balances.never_reconciled` is a list - reusing `balancesDocument` silently emits the wrong shape.
- `Counts.Transfers` (all transfer rows) is `rows.transfers`; `transfers.paired` is a smaller count - a fixture with zero one-sided legs cannot tell them apart, so the unit fixture keeps one-sided > 0.
- `dates` must not go through `.In(time.Local)` (calendar days); only the human `taken` phrase converts to local.
