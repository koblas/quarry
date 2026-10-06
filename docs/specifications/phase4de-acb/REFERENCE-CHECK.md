# Reference check — SCENARIO-21 (2026-10-05)

Run by the orchestrator on copies of snapshot `20261004T184923Z` in a scratch HOME (the user's store and config untouched), binary built from the feature branch after SCENARIO-22. Counts only; no names, tickers or amounts of the user's data beyond the anonymised arithmetic below.

## Classification used (scratch config only)

9 investment accounts: 4 USD retirement → registered (user decision F3); 1 brokerage whose name names a registered plan → registered; 4 brokerage (1 CAD, 3 USD, 1 of them closed) → non-registered. `quarry findings --type unclassified-account --status all` then lists 0. The user's own config is not written; classifying for real happens in session on the user's yes (skill "Classifying accounts").

## Results

| Check | Result |
|---|---|
| (a) cost_basis = −amount on every non-registered buy | 75 / 75 exact, 0 NULL |
| Reinvests with NULL cost_basis and non-zero amount (S13a debt) | 0 of 1 — no new scenario |
| (b) pool units on every event date = Σ holding_shares over non-registered accounts | 139 / 139 (holding spans' `to_date` is inclusive). Before SCENARIO-22: 137 / 139 — the 2 misses were the oversell below |
| (c) pool oversell | 1 on the real file (USD fund sold 1,122.84 units beyond the pool 2017-01-12, covered 2017-01-30). Rule gap → SCENARIO-22 (RULING-S22.md). Now: warning 10a, sale marked unknown cost, pool covers back to 0 = Quicken's holding |
| (d) first fx rate ≤ first USD pool trade | yes; 0 unvalued events |
| (e) ACB removed vs Quicken ZLOTMOD | CAD: 2 / 2 exact (single acquisition). USD (CAD vs native lot cost): 16 single-rate sales = native × rate to the cent; 13 sales where Quicken's lot cost is 0 → all marked `unknown_cost`; the rest explained below |

### (e) USD differences, each verified

- **Same-day multi-buy, Quicken specific lots vs CRA average** (1 security, 2 sales): three buys on one day; Quicken's first sale removed exactly the first two lots (native × 1.3106 = the two buys' CAD cost to the cent), its second sale 79 / 294 of the third lot (within 0.01 of rounding). quarry averages, as the CRA requires. Difference closes to 0.00 under Quicken's lot pick.
- **No-cost add_shares** (1 security with a ratio of 0.655, plus others): 220 shares added with no cost (probably a split entered as Add Shares). quarry counts them at zero cost; every such sale (25) is marked `unknown_cost` and each add is listed by `quarry findings --type shares-without-cost` (24 rows). Warning 4a names the securities (14).
- **Multi-acquisition securities, allocation only**: 15 securities with several acquisition rates; for every one quarry's CAD cost is conserved exactly (Σ acquisition CAD = Σ ACB removed + ACB remaining). Per-sale differences are allocation between sales (average vs Quicken's lots), not lost or invented cost.
- **The short (SCENARIO-22)**: the only security whose cost is not conserved — by the ruling, the covering buy's cost counts nowhere and the 2017 gain is overstated by it, marked unknown cost with warning 10a.

No unexplained difference remains.

## Counts

- Securities in the pool 63; events 149; sales 46 over 8 years; positions incomplete 2.
- Possible superficial-loss marks: 0.
- Unknown-cost sales: 25 (24 no-cost-add sales + 1 oversold sale).
- shares-without-cost rows: 24. Unpaired removes: 1 (warning 5). Return of capital above ACB: 0 (no adjustments configured).
- Warnings: slot 4 × 14, slot 5 × 1, slot 10 × 1.

## Left to the user (manual)

One year's per-sale proceeds and outlays against the broker's T5008, or a past Schedule 3, once the accounts are classified for real.
