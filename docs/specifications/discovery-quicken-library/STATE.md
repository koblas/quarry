# discovery-quicken-library — current state

Scenarios complete: SCENARIO-01..05 (all five; 02..05 folded into SCENARIO-01's run).
Last updated by REVIEW-01 fix pass.

Inherits `docs/specifications/phase0-snapshot/STATE.md` by reference — everything there
about `Sync`, the refusal shape, `ResolveBundlePath`, and the CLI wiring still holds. This
file only records what SCENARIO-01 changed or added on top of it.

## Binding decisions
- `DiscoverBundle(home string) (string, error)` now pools two locations, in fixed order:
  `~/Documents`, then `~/Library/Application Support/Quicken/Documents`
  (`quickenDocumentsDir`). Signature unchanged — `internal/cli/sync.go:72`'s call site needed
  no edit beyond the help-copy strings. (SCENARIO-01)
- `bundleCandidate{path, info}` carries the `os.Stat` result taken during the scan so
  `dedupeByIdentity` never re-stats; `info` must come from `os.Stat` (follows symlinks), not
  `DirEntry.Info()` (`Lstat`-based), or identity dedupe silently misses a file-symlink row.
  `dedupeByIdentity` keeps the first candidate found per distinct `os.SameFile` identity,
  preserving scan order, so `Source` shows the path the bundle was first reached by.
  (SCENARIO-01)
- Location check order is also the refusal order: `DiscoverBundle` returns on the first
  location it cannot `ReadDir` (R3 for `~/Documents`, R3b for the Quicken folder) without
  reading the second. Each location carries its own `missing func(error) bool` predicate
  (`documentsMissing`, `quickenDocumentsMissing`) so `scanForBundles` can decide silently vs.
  refuse without `DiscoverBundle` distinguishing error shapes itself — `scanForBundles`
  always returns either no error or an already-built `RefusalError`. `documentsMissing` is
  `ErrNotExist` only; `quickenDocumentsMissing` (DQ-4 amended) also treats `ENOTDIR` as
  missing — an ancestor of the Quicken folder being a plain file reads the same as the
  location not existing. `~/Documents` keeps R3 for `ENOTDIR`; do not add that branch to
  `documentsMissing`. (SCENARIO-01, REVIEW-01)
- Per-entry `os.Stat` faults (e.g. a symlink loop) keep the existing generic
  `unreadableRefusal(home, path, cause)` regardless of which location the candidate is under;
  only the location-level `ReadDir` fault is folder-specific (R3 vs R3b).
  `documentsUnreadableRefusal`/`quickenDocumentsUnreadableRefusal` (both `func(cause error)
  error`, no `home` param — the copy is fixed per location) differ in copy, not just the path
  named — R3 keeps the Privacy & Security wording, R3b is shorter ("check the folder's
  permissions"). (SCENARIO-01, REVIEW-01)
- R2's message (`multipleQuickenBundlesRefusal`) lists full `~`-abbreviated paths across both
  locations, bytewise-sorted, no bare filenames and no "in ~/Documents" — bytewise sort
  already puts `~/Documents` before `~/Library/...` (`'D' < 'L'`), no special-casing needed. A
  future third location keeps this shape. (SCENARIO-01)
- `~/Library/Application Support/Quicken/Backups` is never read — `quickenDocumentsDir` joins
  `.../Quicken/Documents` only. Do not generalize to a `filepath.Glob` over `Quicken/*`.
  (SCENARIO-01)
- `sync`'s Long help and `--quicken` flag help name both locations verbatim per
  `specification.md` → *Surface & Copy*. (SCENARIO-01)

## Left unbuilt
None for this feature — DQ-1 through DQ-6 are all built and tested.

## Traps
- `os.SameFile` never errors (`go doc os SameFile`); it's not in the fallible-call inventory.
- Bytewise `sort.Strings` on full `~`-paths is sufficient for the "`~/Documents` first" edge
  row — don't add a custom comparator.
- `scanForBundles(home, dir, missing, refusal)` never returns a bare `error` — a location's
  `missing` predicate decides silence, everything else (`ReadDir` fault or per-entry stat
  fault) is already a `RefusalError` when it comes back, so `DiscoverBundle` just propagates
  it. Adding a third failure shape there means building it as a `RefusalError` inside
  `scanForBundles`, not returning a raw `error` for `DiscoverBundle` to re-classify.

## Open debts
- A per-entry candidate whose stat fails with ENOTDIR (e.g. `~/Documents/Old.quicken` → symlink through a regular file) is refused via `unreadableRefusal`, not skipped as dangling. Pre-existing filter; ruled at final product-vision: skip as dangling (`errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ENOTDIR)` in the per-entry stat check, plus a test row); build on the next change to `discover.go`.
- `cmd/quarry/run_test.go` — phase0's STATE.md already flagged this file (758 lines) as due
  for a split "before the next feature adds to either" (it named this file and
  `internal/snapshot/sync_faults_test.go`); this scenario added ~200 lines to it, so that
  debt is now triggered and still unowned.
