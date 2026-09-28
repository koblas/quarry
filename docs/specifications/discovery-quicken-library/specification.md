# Specification: Discover bundles in Quicken's Documents folder

<!-- spec-check: v1 -->

## Intent & Goal

**Primary Goal**: `quarry sync` without `--quicken` finds the user's Quicken file where Quicken Classic for Mac actually keeps it — `~/Library/Application Support/Quicken/Documents` — as well as `~/Documents`, without ever guessing between files.

**Out of Scope**: iCloud Drive, recursive search, `~/Library/Application Support/Quicken` itself (holds `Backups/`), config-file paths (Phase 1), location preference.

## Business Rules & Invariants

- **DQ-1 Two locations, fixed order:** `~/Documents`, then `~/Library/Application Support/Quicken/Documents`. Top level of each only; per-directory filter unchanged (dotfiles ignored, case-insensitive `.quicken` suffix, symlinks followed, dangling links and non-directories skipped).
- **DQ-2 One pool:** exactly one distinct bundle across both → use it (through `ResolveBundlePath`, unchanged). Two or more → R2. Never prefer a location.
- **DQ-3 Dedupe by file identity** (`os.SameFile`); keep the first path found in location order. `Source` shows that path as found.
- **DQ-4 Missing location is silent** (ENOENT on the folder or any ancestor → zero candidates).
- **DQ-5 Unreadable location refuses**, even if the other location has exactly one bundle. Check in location order; first unreadable wins (R3 before R3b).
- **DQ-6** With `--quicken`, neither location is read.

---

## Triage Brief

Single caller `internal/cli/sync.go:72` → `internal/snapshot/discover.go` `DiscoverBundle(home)` (signature stays). Strings naming `~/Documents`: sync Long help (`sync.go:50`), flag help (`sync.go:111`), R1 (`discover.go:66`), R2 (`:72-74`), R3 (`:81-83`). Tests pinning them: `internal/snapshot/discover_test.go` (whole file), `cmd/quarry/run_test.go` `Test_run_discovers_the_bundle_from_documents_without_quicken` (~228-281). **Already exists — do not re-plan:** per-directory filtering, `ResolveBundlePath`, `causeText`, `documentsUnreadableRefusal`.

## Product Verdict

SHIP WITH CHANGES (scoping), folded in: top-level-only search (hardkoded's recursive search would hit `Quicken/Backups` → R2 on every run); dedupe by identity; unreadable location refuses; R2 always full `~`-paths. User ruled: union, never prefer a location.

## Surface & Copy

Replaces the matching phase0-snapshot rows; implement verbatim.

`sync` Long help, last paragraph:
```
Without --quicken, quarry looks for .quicken files in ~/Documents and in
~/Library/Application Support/Quicken/Documents, and uses the one it finds
if there is exactly one.
```

`--quicken` flag help: ``"`path` to the .quicken file to snapshot (default: the only one in ~/Documents or Quicken's Documents folder)"``

| # | Condition | Exact stderr | Exit |
|---|---|---|---|
| R1 | No distinct bundle in either location | `quarry: no .quicken file found in ~/Documents or ~/Library/Application Support/Quicken/Documents; pass one with --quicken <path>` | 1 |
| R2 | ≥2 distinct bundles across both | `quarry: found 3 .quicken files (~/Documents/Business.quicken, ~/Documents/Home.quicken, ~/Library/Application Support/Quicken/Documents/Home.quicken); choose one with --quicken <path>` (all, `~`-paths, bytewise sort) | 1 |
| R3 | `~/Documents` exists but unreadable | unchanged: `quarry: cannot read ~/Documents: operation not permitted; allow your terminal to access the Documents folder in System Settings > Privacy & Security > Files and Folders, or pass --quicken <path>` (OS reason verbatim) | 1 |
| R3b | `~/Documents` readable or missing; Quicken Documents folder (or ancestor) exists but unreadable | `quarry: cannot read ~/Library/Application Support/Quicken/Documents: permission denied; check the folder's permissions, or pass --quicken <path>` (OS reason verbatim) | 1 |

Stdout empty and nothing written for all four. Candidate entry stat failure keeps the existing `unreadableRefusal` text.

### Edge-case rows (`~/D` = `~/Documents`, `~/L` = `~/Library/Application Support/Quicken/Documents`)

| Input class | Outcome |
|---|---|
| Both missing | R1 |
| `~/D` missing, `~/L` has 1 | uses it; Source shows `~/L/…`; exit 0 |
| `~/D` has 1, `~/L` missing | uses it; exit 0 |
| Both exist, both empty | R1 |
| One each, distinct | R2, `~/D` path first |
| Two in `~/D` | R2 with full `~/Documents/…` paths |
| Two in `~/L` | R2 with full `~/L` paths |
| `~/D/X.quicken` → symlink to the `~/L` bundle | one; uses `~/Documents/X.quicken` |
| `~/L` symlinked to `~/D` | each bundle counted once |
| `~/D` unreadable, `~/L` has 1 | R3 |
| `~/D` has 1, `~/L` unreadable | R3b |
| Both unreadable | R3 |
| Bundles only under `~/Library/Application Support/Quicken/Backups/**` | not searched; R1 |
| `--quicken` given | no discovery; neither folder read |

---

## Scenarios (Gherkin)

Orchestrator sizing: one run. SCENARIO-02..05 fold into SCENARIO-01 (same function, same tests).

```gherkin
Scenario: SCENARIO-01 — Bundle only in Quicken's Documents folder is used
  Given ~/Documents is missing and ~/Library/Application Support/Quicken/Documents holds one open bundle
  When I run `quarry sync` without --quicken
  Then it snapshots that bundle, the Source line shows its ~/Library/… path, and exit is 0
```

```gherkin
Scenario Outline: SCENARIO-02 — Bundles counted as one pool across both folders
  Given <layout>
  When I run `quarry sync` without --quicken
  Then stderr is <line> and exit is <code>

  Examples:
    | layout                                                 | line                                        | code |
    | both folders missing                                   | R1 naming both folders                      | 1    |
    | one bundle in each folder                              | R2 listing both ~-paths, ~/Documents first  | 1    |
    | two bundles in ~/Documents, none in the Quicken folder | R2 listing full ~/Documents/… paths         | 1    |
    | bundles only under ~/Library/…/Quicken/Backups         | R1                                          | 1    |
```

```gherkin
Scenario: SCENARIO-03 — Same bundle reached two ways counts once
  Given ~/Documents/X.quicken is a symlink to the only bundle in the Quicken folder
  When I run `quarry sync` without --quicken
  Then it snapshots once, Source shows ~/Documents/X.quicken, and exit is 0
```

```gherkin
Scenario Outline: SCENARIO-04 — An unreadable folder refuses instead of guessing
  Given <layout>
  When I run `quarry sync` without --quicken
  Then stderr is <line>, nothing is written, and exit is 1

  Examples:
    | layout                                         | line |
    | ~/Documents unreadable, Quicken folder has one | R3   |
    | ~/Documents has one, Quicken folder unreadable | R3b  |
```

```gherkin
Scenario: SCENARIO-05 — Help names both folders
  When I run `quarry sync --help`
  Then the help text and the --quicken flag help name both folders verbatim
```

---

## BDD Acceptance Progress
- [x] SCENARIO-01: Bundle only in Quicken's Documents folder is used — `cmd/quarry/run_test.go` `Test_run_discovers_the_bundle_from_documents_without_quicken`
- [x] SCENARIO-02: Bundles counted as one pool across both folders — delivered by SCENARIO-01 `cmd/quarry/run_test.go` `Test_run_pools_bundles_across_both_documents_folders`
- [x] SCENARIO-03: Same bundle reached two ways counts once — delivered by SCENARIO-01 `cmd/quarry/run_test.go` `Test_run_counts_a_bundle_reached_two_ways_once`
- [x] SCENARIO-04: An unreadable folder refuses instead of guessing — delivered by SCENARIO-01 `cmd/quarry/run_test.go` `Test_run_refuses_when_a_discovery_location_is_unreadable`
- [x] SCENARIO-05: Help names both folders — delivered by SCENARIO-01 `cmd/quarry/run_test.go` `Test_run_sync_help_names_both_documents_folders`
