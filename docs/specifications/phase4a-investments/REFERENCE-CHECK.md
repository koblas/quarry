# Reference check: phase4a-investments (SCENARIO-12)

Method: the binary built from this branch runs `quarry sync --from 20260930T072052Z` with `HOME` set to a scratch directory that holds a copy of the user's snapshot (`.sqlite` + `.json`). The user's real store is never touched. Only the Rows / Shares / stderr lines are recorded here, never holdings.

## Run 1 — 2026-10-04, HEAD after SCENARIO-11

- `sync --from 20260930T072052Z`: exit 1.
- stderr: `quarry: cannot import snapshot 20260930T072052Z: an investment transaction on 2017-01-12 in "Fidelity Investments" has a commission of 8.4998, which has more than 2 decimal places (and 17 more); ~/Library/Application Support/quarry/quarry.duckdb was not changed; run quarry sync --from 20260930T072052Z once quarry supports it`
- Rule gap: 18 `sell` commissions with 3–4 decimals (exact at 4). Ruled by product-vision 2026-10-04 → commission DECIMAL(18,4); appended as SCENARIO-13.
