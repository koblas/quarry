# Review Report — round 01

### Target
Changed files, range 9782926..b22bc62 (232 files). Coverage gate: 0 uncovered added lines, 4 declared unreachable (all accepted by correctness and test reviewers). Mutation sample: 20 sampled of 247 candidates — 20 killed, 0 survived (1517s). `spec-check.py --run phase4de-acb`: OK. Lint: 0 issues. test-stats TOTAL 3969 (+555).

### Triggered reviewers
- arch-reviewer, correctness-reviewer, test-reviewer, refactor-advisor: triggered by internal/**, cmd/**

### Skipped reviewers
- api-reviewer: no HTTP files. pipeline-reviewer: no `.claude/**` changes.

### BLOCKER
none

### MAJOR
- correctness-reviewer: `internal/report/acb_walk.go:255` — after a split the pool share count is an exact big.Rat; an uneven consolidation leaves a sub-millionth leftover that every `Sign()` held/short test treats as held or short, while quarry's own holding count (`duckstore/shares.go:207-219`, millionths) and `superficialIndex.heldAt` say flat. Failure (reproduced): (a) buy 100, split 1/3, sell 33.333333, ROC 10.00 → ROC counted as a 10.00 capital gain, not-held warning missing, ghost position `0 / 0.00 / 0.0000`, `incomplete` false; (b) buy 1000, split 1/7, sell 142.857143 → warning 10a "sold 0 more shares…", sale `unknown_cost`, incomplete true. Fix (orchestrator ruling 2026-10-05): snap `w.pool.shares` to the nearest millionth (round half away, as `Millionths`) right after `splitShares` in `apply`; pin (a) no position/ghost row + not-held skip, (b) no warning 10, no unknown_cost, incomplete false. A sell of 33.333334 after 1:3 stays a real 1-millionth short.
- test-reviewer: `internal/mcp/acb_cap_test.go:69` — ruled `events: []` for a security the 500-cap cuts to zero is unpinned (`assert.Empty` passes for nil). Mutant `if keep == 0 { capped[i].Events = nil }` at `internal/mcp/acb.go:63` survived. Fix: `assert.Equal(t, []document.ACBEvent{}, doc.Securities[1].Events)`; same on the (500, 0) row of `Test_acb_caps_events_at_500_across_securities`.

### MINOR
- test-reviewer: `cmd/quarry/run_config_test.go:352` — `if name == "accounts" || name == "acb" { continue }` in a test body; build the `--currency`-honouring list as data.
- test-reviewer: cross-scenario cells unpinned end to end (behaviour correct by probe): short × superficial (oversold loss covered to exactly 0 within 30 days not marked; cover beyond the short marked); splits × short text/JSON cell; `--security` × superficial on a real walk (`acbStackedRows` + `--security ACME` text); MCP parity rows for warnings 3, 5, 9, 10b and short × year/security; 10b `warnings[]`/JSON cell.
- test-reviewer: `plugin/skills/quarry/SKILL.md:77` relays only "superficial-loss and incomplete warnings"; acb now also warns 5, 8, 9, 10 — route to the final product-vision pass.
- arch-reviewer / refactor-advisor: `acbAdjustmentsOf` and `classificationOf` duplicated in `internal/cli` and `internal/mcp` (drift risk; config and report cannot import each other).
- refactor-advisor: `securityWalk.apply` (acb_walk.go:200-258) does four things — Compose method (`newEvent`, `dispose`).
- refactor-advisor: `currencyFlag.argsCADOnly` (cli/currency.go) carries acb-specific copy in the shared flag type — take the message from the caller.
- refactor-advisor: security order comparator written twice (acb_select.go:30-35, acb_walk.go:88-93) — one `compareSecurities`.
- refactor-advisor: comments cite foreign code by file:line (acb_superficial.go `unreachable:` comments, mcp/accounts.go `acbYearRefusal`, acb_walk.go:62-63) — state the invariant without coordinates.

### NIT
- correctness-reviewer: `acb_superficial.go:274` `// unreachable:` above a line whose date arm is reachable — split the nil check onto its own line or reword. (test-reviewer: same marker at :38 — reword/drop.)
- correctness-reviewer: `internal/cli/acb.go:59,77` reads `now()` twice — read once like MCP.
- correctness-reviewer: warnings quote names raw while unknown-id lines use `tomlstr.BasicString` — no action (copy ruling if ever touched).
- arch-reviewer: `internal/mcp/accounts.go:12` `acbConfigShown` is a fourth copy of the config path literal.
- arch-reviewer: `internal/finding/doc.go:1` says "nine finding types"; there are ten.
- test-reviewer: duplicated decode structs across cmd/quarry acb tests.
- refactor-advisor: `RunE` compose; `ACBWarnings` stray blank line / doc tail; `forEachEvent` helper; repeated constants (`perShareDecimals`, cents-per-dollar `100`, `1_000_000` rate unit); ticker-present rule repeated (export `groupingTicker`); `acbRefusal` switch; `take(units)` shadows `units`; missing docs (`zeroIfNil`, `countSales`, `newSuperficialIndex`); `sell()` patched fields; comment budgets (`acbPool.add`, `ACB.Cut`, `ACBEvent`, `ACB`, `Selected`).

### Strengths
- Walk tables vary one variable per row (unknown-cost 14-row table; half-away/USD/no-short rows); 500/501 cap boundary; CLI × MCP parity over every edge row with none/year/security; refusal precedence on one fixture; read-time chokepoint keeps status/sync/MCP counts in agreement.

### Verdict: BLOCKED
Two MAJORs (acb_walk.go:255 split leftover; acb_cap_test.go:69 `events: []` pin).

### Orchestrator ruling after fix pass 1 (2026-10-05) — supersedes the snap ruling above

The per-split snap diverges from `duckstore.holdingSpans` and `superficialIndex.heldAt` on chained splits (1:3 then 3:1, sell 99.999999: walk flat, duckstore 0.000001 held). Invariant to build to: every held / flat / short decision in the walk (span open/close, not-held adjustment, oversold, `Oversold` amount, cover, `Incomplete`, `PerShare`, position filters) uses `Millionths(pool.shares)` — the stored-holding unit duckstore and heldAt use; the exact big.Rat stays for arithmetic. Remove the per-split snap. A disposition that leaves `Millionths(shares) <= 0` removes all ACB (so "shares <= 0 => ACB 0" holds in millionths). Pins: fixtures (a), (b) unchanged outcomes; control 33.333334 after 1:3 = -0.000001 short; chain row 1:3 then 3:1, sell 99.999999 = 0.000001 held (matches holdingSpans); a disposition leaving a positive sub-half-millionth exact remainder = flat with ACB 0.00.

### Orchestrator ruling after fix pass 2 (2026-10-05) — re-gate correctness MAJOR (split moves held() across 0)

Invariant: (1) after any acquisition or disposition that leaves `held() == 0`, the exact `pool.shares` is set to 0 (no dust survives a flat point, so a later split cannot turn flat into short or held); (2) the unknown-cost span closes whenever `held() <= 0` after ANY pool change, splits included; (3) a split that rounds a held pool to 0 millionths keeps the pool ACB (it joins the next acquisition; never silently lost) — "shares <= 0 => ACB 0" holds after dispositions, not after such a split. Pins (correctness probes): shape 1 buy 1,000 / split 1:7 / sell 142.857143 / split 7:1 → flat, held 0, incomplete false, no short, no warning; shape 2 buy 0.000001 for 1.00 / split 1:3 → shares 0, ACB 1.00 kept, no position row, PerShare nil; shape 3 no-cost add 0.000001 / split 1:3 → shares 0, span closed, incomplete false. Chain pin (0.000001 held) and fixtures (a)/(b) unchanged. This is fix pass 3, the last allowed for this surface.
