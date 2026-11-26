# quarry — Milestones, risks and open questions

Part of the quarry product spec; index in [README.md](README.md).

## Milestones

The work ships in five phases; Phase 1 is the hard part, because every later phase trusts its reconciliation.

Each phase ships only when its check passes.

| Phase | Scope | Gate |
| --- | --- | --- |
| 0 — Port prior art | dweekly schema, recipes, skill; backup snapshot | Schema verified on your v9 file |
| **1 — Import + store** | Banking, credit, splits, transfers, categories | Balances reconcile for all cash accounts |
| 2 — CLI + analysis | Views, CLI, cleanup worklist, anomaly detection (slices 2a–2f; `export` dropped for `sql --csv`; 2d covers every finding type) | Matches Quicken reports, 2 years |
| 3 — Skill + MCP | `SKILL.md`, named tools, read-only query | Claude answers the use-case questions correctly |
| 4 — Investments | Holdings, ACB, prices, net worth; monthly scheduled summary | Share counts match Quicken |

Phases 0–2 alone deliver the core goal (the data as a queryable source); the MCP server and investments build on a store that is already proven correct. No dates are set yet.

## Risks and open questions

The main risk is Quicken's undocumented schema; reconciliation on every sync is the mitigation.

| Risk | Impact | Mitigation |
| --- | --- | --- |
| Quicken's Core Data schema is undocumented and changes between versions | Wrong or failed imports | Schema fingerprint per import; mapping isolated in one module; sync fails loudly on reconciliation errors |
| Semantics hidden in the schema (split transfers, voided items, pending, scheduled vs posted) | Subtly wrong totals | Golden fixture file covering each case; balance reconciliation catches drift |
| Investment lots and cost basis may be computed by Quicken rather than stored | Phase 4 cannot match Quicken's gains | Import transactions first; reproduce cost basis only if validation shows it is needed |
| A snapshot taken while Quicken is closed (encrypted, no accounts) or mid-write | Corrupt snapshot | Backup API while Quicken is open; import rejects empty snapshots and runs an integrity check |
| Sensitive data reaching a conversation | Privacy exposure | Row caps, account numbers from config masked to the last four digits, local-only MCP; card and account numbers in free text are masked upstream by the institution or Quicken |

**Open questions**

None block the design. Facts about a particular user's data (which accounts are registered, return-of-capital adjustments, whether trade history is complete) are gathered in session, not designed in: the skill asks for them when a report first needs them, stores the answers in `quarry`'s config, and `quarry` reports what is missing as findings.

Pinning a snapshot (for example a tax year-end) so `prune` skips it is left out of v1; add it if the count cap proves too blunt.

Quicken's category tax line is not imported; tax totals are by user-named category until it is.
