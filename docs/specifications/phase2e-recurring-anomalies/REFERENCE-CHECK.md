# Reference check: phase2e-recurring-anomalies

Date: 2026-10-01. Source: David's Quicken file, snapshot `20260930T072052Z`, synced with this branch in a scratch HOME (real store untouched).

- `recurring --since 2000`: 51 series, 22 active.
- `anomalies --since 2024`: 232 listed of 2,601 charges checked; 46 not judged. (`--since 2000`: 825 of 10,765; 552 not judged.)

Review: output of both commands sent to David. Verdict: "looks mostly right, continue with the gate" — no rule re-ruled. Raised for the final product-vision pass: one long payee name widens the recurring Payee column to ~77 characters (no width cap ruled); anomaly volume (~9% of charges since 2024) and multi-price-change series (VIDEOTRON 6, OPTIMUM 8) accepted as shipped.
