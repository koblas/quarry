# discovery-quicken-library — current state

Scenarios complete: SCENARIO-01..05 (all five; 02..05 folded into SCENARIO-01's run).
Last updated by SCENARIO-01.

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
  reading the second. A missing location (`ErrNotExist` on the folder or any ancestor) is
  silent — scan continues to the next location. (SCENARIO-01)
- Per-entry `os.Stat` faults (e.g. a symlink loop) keep the existing generic
  `unreadableRefusal(home, path, cause)` regardless of which location the candidate is under;
  only the location-level `ReadDir` fault is folder-specific (R3 vs R3b).
  `documentsUnreadableRefusal`/`quickenDocumentsUnreadableRefusal` differ in copy, not just
  the path named — R3 keeps the Privacy & Security wording, R3b is shorter ("check the
  folder's permissions"). (SCENARIO-01)
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
- A location-level `ReadDir` fault and a per-entry `os.Stat` fault both surface as `error` from
  `scanForBundles`; `DiscoverBundle` distinguishes them with `errors.As(err, &RefusalError{})` —
  a per-entry fault is already a fully-formed `RefusalError` and must be returned as-is, not
  re-wrapped by the location's refusal builder.

## Open debts
- `cmd/quarry/run_test.go` — phase0's STATE.md already flagged this file (758 lines) as due
  for a split "before the next feature adds to either" (it named this file and
  `internal/snapshot/sync_faults_test.go`); this scenario added ~200 lines to it, so that
  debt is now triggered and still unowned.
