# Reference check: phase2d-findings

Date: 2026-10-01. Source: David's Quicken file, snapshot `20260930T072052Z`, synced with this branch in a scratch HOME (real store untouched).

Sync: all checks passed; 836 open findings — uncategorized 241, duplicate 174, payee-variants 135, unlinked-transfer 106, mixed-categories 101, unused-category 50, one-sided-transfer 29, similar-categories 0.

Review: up to 20 findings per heuristic type (unlinked-transfer, mixed-categories, payee-variants, unused-category; similar-categories had none) sent to David. Verdict: "the unlinked transfers look mostly right, continue with the gate" — no type re-ruled. Known false-positive shape kept as accepted: reimbursements that mirror an expense (e.g. SENTRY −89.00 vs a Netskope Expensify +89.00 deposit) — ignorable via `findings.ignore`.
