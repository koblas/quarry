# Reference check: phase2f-fx

Date: 2026-10-01. Source: David's Quicken file, snapshot `20260930T072052Z`, synced with this branch in a scratch HOME (real store untouched), against the live Bank of Canada Valet API.

- First sync: `Rates     USD/CAD 2013-02-07 to 2026-10-01 (3,406 new)` — IEXE0101 2013-02-07..2016-12-30 (974), FXUSDCAD 2017-01-03..2026-10-01 (2,432); first rate = first transaction date, so no "before" warnings. `rates_checked_from` 2013-02-07.
- Second sync: `(up to date)`. Sandbox-blocked run: `none (not fetched; see warning)`, warning on stderr, exit 0.
- Verified network facts (were unverified Traps): Valet URL and JSON shape, IEXE0101 exists with CAD-per-USD unit, FXUSDCAD starts 2017-01-03, cutover clean (2016-12-30 IEXE0101 → 2017-01-03 FXUSDCAD).
- spend 2025: CAD 625,103.10 / USD 450,372.08 / native CAD 305,773.36 + USD 229,927.62. cashflow --by year, recurring, anomalies, accounts reviewed in CAD/USD/native; sampled USD charges with applied rate.

Verdict: "looks right, continue with the gate" — no rule re-ruled.
