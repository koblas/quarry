---
id: SCENARIO-05
status: open
---

# SCENARIO-05: sync snapshots the file named by quicken.path

Cadence: code-first (nothing on the mandatory test-first set: no bug fix, no write-safety guard, no atomic adapter; sync only reads the Quicken file)
Acceptance test: `cmd/quarry/run_quicken_path_test.go` `Test_run_sync_snapshots_the_file_named_by_quicken_path`
Acceptance test (SCENARIO-06, folded): `cmd/quarry/run_quicken_path_test.go` `Test_run_sync_prefers_the_quicken_flag_over_quicken_path`
Acceptance test (SCENARIO-07, folded): `cmd/quarry/run_quicken_path_test.go` `Test_run_sync_refuses_a_quicken_path_that_does_not_exist`
Acceptance test (SCENARIO-08, folded): `cmd/quarry/run_quicken_path_test.go` `Test_run_sync_refuses_a_quicken_path_that_is_not_a_bundle`
Acceptance test (SCENARIO-11, folded): `cmd/quarry/run_quicken_path_test.go` `Test_run_sync_from_ignores_quicken_path`
Acceptance test (SCENARIO-12, folded): `cmd/quarry/run_discovery_test.go` `Test_run_pools_bundles_across_both_documents_folders`
Narrow loop: `go test ./internal/snapshot/ -run 'ResolveBundle|DiscoverBundle' && go test ./cmd/quarry/ -run 'quicken_path|discovers|pools_bundles|counts_a_bundle|discovery_location|sync_help'`
Mutation checks: (none mandatory; five load-bearing, one at a time) (1) config over flag — swap the two branches of `ResolveBundle` → `Test_ResolveBundle_prefers_the_flag_path_over_the_configured_one`; (2) discovery over config — drop the configured branch → `Test_run_sync_snapshots_the_file_named_by_quicken_path` (the A/B decoys turn it into the several-files refusal); (3) configured path examined when `--quicken` is given — resolve it before returning the flag's → `Test_run_sync_prefers_the_quicken_flag_over_quicken_path`; (4) `--from` reads `quicken.path` — call `ResolveBundle` above the `--from` branch, `sync.go:93` → `Test_run_sync_from_ignores_quicken_path`; (5) K1/K2 for a flag-origin path — pick the copy by "configured is set", not by the path used → `Test_ResolveBundle_keeps_the_flag_refusals_when_quicken_path_is_also_set`
Runs: A (1-3) | B1 (4-6) | V (7-8)
Size: OWNS A RUN — 3 batches, 1 feature package (snapshot) + cli; `cmd/quarry` gets tests only

Contract (`## Surface & Copy` → *quicken.path outcomes* and *Changes to existing surfaces* #1 discovery sentence, #2, #4, #5, verbatim; `runWith` adds `quarry: `):
- `quarry sync`, `quicken.path` set → syncs it; stdout identical in form to `--quicken` (nothing names the origin), stderr empty, exit 0.
- `quarry sync --quicken X` → X; `quicken.path` type-checked by the loader only, never stat'ed. `quarry sync --from <id>` → never resolves a bundle.
- K1–K5 and the amended #4/#5 lines → stdout empty, one stderr line, exit 1, no `snapshots` folder created. K3/K4/K5 are today's flag lines byte for byte.
- Flag-origin lines (`bundle.go:39`, `notABundleRefusal` `:104-106`) unchanged: `bundle_test.go` and `run_discovery_test.go:17-124` take no hunk.

Callers (all `grep`; gopls `findReferences` on `ResolveBundlePath` returned the declaration only, so it failed its control): `ResolveBundlePath` — `discover.go:47`, `cli/sync.go:152`, `bundle_test.go` ×13 (signature kept); `DiscoverBundle` — `cli/sync.go:154`, `discover_test.go` (kept); cli `resolveBundle` — `sync.go:96` only (goes); `notABundleRefusal` — `bundle.go:49,55`.

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_quicken_path_test.go` (new) `Test_run_sync_snapshots_the_file_named_by_quicken_path` — `v9fixture.OpenBundle(t, ~/Books)`, empty dirs `~/Documents/A.quicken` + `B.quicken` (as `run_discovery_test.go:241-246`), `writeConfig` (`run_config_test.go:42-47`); bare `sync`; Source line, stderr empty, exit 0. No stub needed: it compiles against `run` and is red today on the several-files refusal (exit 1) — quote that
- [x] Step 2: same file, folded tests — `…refuses_a_quicken_path_that_does_not_exist` (K1, `~/Books/Missing.quicken`) and `…that_is_not_a_bundle` (K2, regular file `~/Books/notes.txt`): exact stderr with `configShown`, stdout empty, exit 1, no `snapshots` folder; red today on the no-bundle-found line. `…prefers_the_quicken_flag_over_quicken_path` (bundle reached as `~/Documents/A.quicken` by symlink, precedent `run_discovery_test.go:280-295`; same Missing config as K1) and `…from_ignores_quicken_path` (one `sync --quicken`, then the Missing config, store removed, `sync --from <id>` as `run_from_test.go:18-47`; store file exists, stderr empty, exit 0): both **green on arrival** — report so, no manufactured red; mutations 3 and 4 are their proof and the K1 test is their control arm
- [x] Step 3: `cmd/quarry/run_discovery_test.go:196-197,206-207,216-217,226-227,237-238,247-248,257-258` `Test_run_pools_bundles_across_both_documents_folders` — amend the seven `wantStderr` rows to the ruled #4 / #5 lines (SCENARIO-12's acceptance test; its "two bundles in ~/Documents" row is the scenario's Given); red on the old copy — quote one row of each line

### Build
- [ ] Step 4: `internal/snapshot/bundle.go:25-64` `ResolveBundlePath`, `:102-106` `notABundleRefusal` — unexported core taking the origin's two refusals (not-exist `:38-40`; not-a-bundle `:48-50`, `:54-55`); `ResolveBundlePath` stays the flag-origin wrapper; new `BundleChoice` + `ResolveBundle(home, choice)`: flag, else configured (K1/K2 copy), else `DiscoverBundle`. New `internal/snapshot/resolve_bundle_test.go` (`package snapshot_test`): `Test_ResolveBundle_prefers_the_flag_path_over_the_configured_one` (both are valid open bundles), `Test_ResolveBundle_refuses_a_bad_configured_path` (K1 missing; K2 plain file; K2 bundle without `data` — two arms), `Test_ResolveBundle_keeps_the_flag_refusals_when_quicken_path_is_also_set` (flag missing, flag plain file, each with a valid configured bundle)
- [ ] Step 5: reused rows + discovery copy — add to `Test_ResolveBundle_refuses_a_bad_configured_path`: K3 `.qdf` (symlink setup as `run_discovery_test.go:31-41`), K4 `v9fixture.ClosedWALBundle`, K5 unreadable `data` (chmod 000, root skip, as `bundle_test.go:120-134`), each equal to the flag line; `Test_ResolveBundle_accepts_a_configured_path_with_a_trailing_slash_or_through_a_symlink` (returned path cleaned). `discover.go:133-138` `noBundleFoundRefusal` (#4), `:140-151` `multipleQuickenBundlesRefusal` (#5), config file shown from one unexported const shared with K1/K2; unit pins `discover_test.go:44-45,160-161,166-167`; Step 3 goes green here
- [ ] Step 6: `internal/cli/sync.go:96` call `snapshot.ResolveBundle` with `quickenPath` and `cfg.QuickenPath`; delete `resolveBundle` `:148-155`; `newSyncCommand` doc `:33-34`; Long discovery sentence `:55-57` → the ruled five lines; `--quicken` help `:140-141` (#2). Pin: `run_usage_test.go:18-32` `Test_run_sync_help_names_both_documents_folders` (both strings, rendered `--quicken path`). Steps 1-2 go green here; run mutations 1-5 (2-4 need this step's wiring)

### Sweep
- [ ] Step 7: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments on `BundleChoice`, `ResolveBundle`, the core; `ResolveBundlePath` doc `:25-28` still true

### Verify
- [ ] Step 8: full verification + `spec-check.py phase2c-snapshots-config` → tick SCENARIO-05, and 06, 07, 08, 11, 12 as "delivered by SCENARIO-05" with their tests last on the line; rewrite `STATE.md`; `status: done`

## Handoff

**Binding decisions** — a later scenario must not contradict these without saying so:
- The path's origin reaches resolution as a value: `snapshot.ResolveBundle(home, BundleChoice{Flag, Configured})` owns the precedence (flag > configured > discovery); cli only maps `--quicken` and `cfg.QuickenPath` into it — `snapshot` never imports `internal/config`, and precedence in `internal/cli` would be logic in the delivery layer
- `ResolveBundlePath(home, path)` keeps its signature and flag copy, and `discover.go:47` keeps resolving the sole discovered bundle through it — that is what keeps flag-origin lines byte-identical by construction; a `fromConfig bool` on it would drag discovery into K2
- `BundleChoice.Flag == ""` means "not given" — sound only because `sync.go:72-74` refuses a blank `--quicken` before `RunE`
- K1, K2, #4 and #5 name the config file from one unexported literal in `snapshot` (`~/Library/Application Support/quarry/config.toml`), not from `cfg.Path` — P2c-1 (no env override, no `--config`) keeps it true and `DiscoverBundle(home)` keeps its signature; a future `--config` must turn it into a field on `BundleChoice`
- `ResolveBundle` is called only on the plain-sync branch (`sync.go:95-100`); `--from`, and `snapshots` / `prune` when 16/24 add them, load config but never call it

**Left unbuilt** — named so nobody assumes it exists:
- Edge row "`snapshots` / `prune`, `quicken.path` missing on disk → no effect" — pin belongs to SCENARIO-16 (`snapshots`) and SCENARIO-24 (`prune` starts loading config)
- sync Long auto-prune paragraph (#1, first half) — SCENARIO-29; `idNotFoundRefusal` (#3) — SCENARIO-16
- Edge rows "`--quicken X` + relative `quicken.path`" (C2r) and "`--from` + `quicken.path = 12`" (C2q) — already pinned by SCENARIO-01, `run_config_test.go:79-128`; not re-planned

**Traps** — things that look right and are not:
- Picking K1/K2 by "is `quicken.path` set" passes every existing flag test (none writes a config file) — mutation 5's test is the only guard
- No ruled copy: a sole *discovered* bundle that fails validation keeps the flag line (`… pass the .quicken bundle with --quicken <path>`, pinned `discover_test.go:204-216`); left byte-identical, raise at the final `product-vision` pass if it should name `quicken.path`
- `cfg.QuickenPath` keeps a trailing slash (`homepath.Expand` only); `filepath.Abs` at `bundle.go:30` cleans it — do not clean in `internal/config`
- `v9fixture.OpenBundle` always names the bundle `Home.quicken`; `writeStatusFixtureBundle` leaves a transfer warning on stderr, so it cannot back a "stderr empty" assertion
- Cobra strips the backticks in flag help; the Long sentence grows from 3 lines to 5, and `Test_run_prints_the_sync_help` (`run_usage_test.go:35-`) pins the neighbouring paragraphs — leave them

## Phase report

Run A done (steps 1-3 ticked). No production code touched; no stubs needed (tests compile against `run`).

Files:
- `cmd/quarry/run_quicken_path_test.go` (new): helpers `quickenPathConfig`, `assertRefusedBeforeSnapshotting`; tests 05, 07, 08, 06, 11.
- `cmd/quarry/run_discovery_test.go:194-265`: seven `wantStderr` rows now the ruled #4 / #5 lines (`... or set quicken.path in " + configShown`).

State (narrow `go test ./cmd/quarry/ -run 'quicken_path|pools_bundles'`):
- RED at assertion: `Test_run_sync_snapshots_the_file_named_by_quicken_path` (exit 1, `found 2 .quicken files (~/Documents/A.quicken, ~/Documents/B.quicken); choose one with --quicken <path>`); `...does_not_exist` (K1) and `...is_not_a_bundle` (K2) both get today's `no .quicken file found ...; pass one with --quicken <path>`; all 7 `Test_run_pools_bundles_across_both_documents_folders` subtests (old copy without `or set quicken.path in ...`).
- GREEN on arrival, as planned: `...prefers_the_quicken_flag_over_quicken_path` (06) and `...from_ignores_quicken_path` (11); mutations 3/4 are their proof, K1 test is the control arm.

For B1 (steps 4-6): nothing to undo. `configShown` (`run_config_test.go:34`) is the const the tests use for the config path. Step 6 must also make `Test_run_sync_help_names_both_documents_folders` pin (not yet written).
