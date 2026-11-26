# quarry — Overview, prior art, goals

Part of the quarry product spec; index in [README.md](README.md).

## Overview

`quarry` turns a Quicken for Mac data file into a clean, local, queryable financial database, exposed through a CLI and an MCP server so both David and Claude can run analysis over decades of history.

Quicken holds years of categorized transactions, balances and investment activity, but that data is locked in an undocumented Core Data SQLite store. Quicken's own reports are fixed and hard to combine, and exporting CSVs by hand for every question is slow and lossy (splits, transfers and investment lots flatten badly). The goal is to make that data a first-class source for any further analysis, without changing how Quicken is used day to day.

## Prior art

Two open-source projects already read Quicken for Mac's SQLite store for AI agents; `quarry` reuses their schema knowledge but adds an owned, reconciled store and the cleanup loop.

| Project | What it does | What `quarry` takes from it |
| --- | --- | --- |
| [hardkoded/quicken-skills](https://github.com/hardkoded/quicken-skills) (shell + SQL, MIT) | Agent skills over a backup-API snapshot with normalized `q_*` views: net worth, spending, investments, data hygiene | Closest design to `quarry`; test schema captured from a real 9.x file; imports daily FX because Quicken keeps only the current rate; hygiene check list |
| [dweekly/quicken-mac-mcp](https://github.com/dweekly/quicken-mac-mcp) (TypeScript, MIT) | Skill + 8-tool MCP server + `qmac` CLI over the live database, read-only, `raw_query` capped at 500 rows; CSV exporter | [Schema reference](https://github.com/dweekly/quicken-mac-mcp/blob/main/docs/schema.md) (84 entities; splits in `ZCASHFLOWTRANSACTIONENTRY`; dates in Core Data epoch, seconds since 2001-01-01) |
| [HarryDolan/qquery](https://github.com/HarryDolan/qquery) (Python) | Early CLI over a copied Quicken for Mac database | Confirms the approach has held across Quicken versions |

What neither does: reconcile against Quicken's balances on every import, keep stable IDs so findings are tracked from open to fixed across snapshots, or ship as a single binary. Both confirm a key constraint: Quicken encrypts the database when the app is closed, so a snapshot must be taken while Quicken is open.

### Reusing MIT-licensed code

`quarry` starts from dweekly/quicken-mac-mcp's code and schema knowledge instead of rediscovering it; its MIT license permits copying, porting to Go and modifying, provided the copyright and license notice travel with the code.

| Asset | Source | Use in `quarry` |
| --- | --- | --- |
| Schema reference (84 entities, Core Data semantics) | dweekly [`docs/schema.md`](https://github.com/dweekly/quicken-mac-mcp/blob/main/docs/schema.md) | Basis for the importer mapping; Phase 0 becomes verification, not discovery |
| SQL recipes: spending with transfer exclusion, `(Uncategorized)` bucket, two-level notes, portfolio with lots | dweekly MCP tools and skill references | Ported into the importer and views; their results double as expected values in tests |
| Skill layout (`SKILL.md` + `references/`) | dweekly plugin | Template for the `quarry` skill |
| Database auto-detection (one `.quicken` bundle in `~/Documents`, else explicit path) | dweekly server | `quarry sync` file discovery |
| CSV exporter | dweekly `scripts/export_sovereign_csv.py` | Independent cross-check of importer output |
| Backup-API snapshot, daily FX download, hygiene checks, 9.x test schema | [hardkoded/quicken-skills](https://github.com/hardkoded/quicken-skills) (also MIT) | `quarry sync`, `fx_rates`, `findings`, golden fixture |

- Both projects' copyright notices and MIT text go in `THIRD_PARTY_NOTICES`, and ported files carry a header naming their source.
- The README credits both projects; improvements that are not `quarry`-specific (schema corrections, v9 quirks) are offered upstream.

## Goals and non-goals

**Goals**

1. Extract the full history, including closed accounts, from Quicken Classic for Mac (v9): accounts, transactions, splits, transfers, categories, tags, payees, securities, investment activity. `quarry` takes its own snapshot with SQLite's backup API while Quicken is open; no manual export or copy step.
2. Normalize it into a documented, stable schema that is independent of Quicken's internal schema and its version changes.
3. Make the data usable four ways: SQL directly, a CLI with `--json` output, a Claude skill, and a local MCP server.
4. Provide correct building blocks for common analysis: transfer-aware spending, recurring charges, cash flow, net worth over time (CAD and USD accounts), investment performance.
5. Feed data-quality findings back as a cleanup worklist applied in Quicken, so Quicken itself, and every later snapshot or export, gets cleaner over time.
6. Build on the existing MIT-licensed work (see Prior art) rather than rediscovering Quicken's schema.
7. Keep all data local by default; nothing leaves the Mac except query results a user or Claude explicitly requests.

**Non-goals**

- Writing to Quicken programmatically. Fixes flow back as a worklist David applies in Quicken, which stays the system of record.
- Querying the live Quicken database. The live file is touched only by the backup API during `quarry sync`; every query runs on a snapshot.
- Bank aggregation or live account syncing. Quicken remains the ingestion point.
- Supporting Quicken for Windows (`.QDF`). The ingest layer is pluggable so a CSV/QIF importer can be added later.
- Dashboards or any UI in v1.
- Financial advice or automated decisions (trades, payments).
