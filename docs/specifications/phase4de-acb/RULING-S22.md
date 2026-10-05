# Ruling — SCENARIO-22 pooled oversell and short cover (product-vision, 2026-10-05)

Found by the SCENARIO-21 reference check: one USD fund on the real file sold 1,122.84 units beyond the pool on 2017-01-12; Quicken's holding went to −1,122.84 until a buy of 1,122.84 on 2017-01-30. Today quarry resets the pool (`internal/report/acb_walk.go:401-413`), books the excess at no cost with no warning, and the covering buy opens a phantom position. Option chosen: short pool with units-only cover (Q8 rules out adjusting the gain later; reset fails reference check (b)).

## Rule (CRA rules, after the add_shares / remove_shares bullet)

- **Oversold disposition** (sell or remove_shares of more units than the pool holds, pooled across non-registered accounts): removes all the pool's ACB; pool shares go negative by the excess (a short, ACB 0.00). A sale counts the excess units at no cost: gain = proceeds − outlays − the ACB removed, in the sale's tax year. The sale is marked `unknown_cost`. The gain is not adjusted later (too high by what the covering shares cost; overstating is the safe side).
- **Acquisition while short** (buy, reinvest, add_shares): its units cover the short first. Covered units add no ACB. Units beyond the short enter the pool at cost × beyond ÷ bought, rounded to the cent half away from zero (USD converted at the acquisition's rate first, then pro-rated). A no-cost acquisition that only covers opens no unknown-cost span; one with units beyond the short opens a span as today.
- **Unknown-cost span** ends when pool shares next reach 0 or below. Covering back to 0 opens nothing.
- **Split while short**: multiplies the negative shares; ACB stays 0.00.
- **Adjustments while shares ≤ 0**: "not held" — the existing warning fires and no ROC gain is counted.
- **Positions while short**: text positions list nothing (shares > 0 only). JSON `shares` negative, `acb` "0.00", `acb_per_share` null whenever shares ≤ 0, `incomplete` true while shares < 0.

## Warning slot 10 (after 9)

One line per oversold disposition, walk order. `<x>` via `humanize.Shares`, `<date>` DateLayout, `<security>` and `<account>` quoted like warning 5. stderr `quarry: warning: …`, `warnings[]`, exit 0; MCP same document, no rewording.

- 10a (sell): `"<security>": the sale on <date> in "<account>" sold <x> more shares than the non-registered accounts held; quarry counts them at no cost, so the sale's gain is too high by what they cost, and the next <x> shares acquired only bring the holding back to 0; correct the shares in Quicken if they are wrong`
- 10b (remove_shares): `"<security>": <x> more shares left "<account>" on <date> than the non-registered accounts held; the next <x> shares acquired only bring the holding back to 0; correct the shares in Quicken if they are wrong`
- Warning 5 still prints for the removal; both lines print.

## Effects

- No new JSON field.
- Sale row: `unknown_cost:true`; `years[].unknown_cost_sales` +1; year suffix and `--year` row suffix reused.
- Sell event: `shares_held` negative (e.g. "-10"), `acb` "0.00", `gain` = the sale's gain.
- Covering buy event: `cad` the full cost (cash flow), `shares_held` rises, `acb` rises only by the pro-rated cost of units beyond the short.
- `--security` text: Shares held negative and grouped; suffix `unknown cost` on the oversold sale.

## Changes to existing surfaces

- specification.md "No pooled oversell." (triage, ~:85) → "One pooled oversell on the real file (SCENARIO-21): a USD fund sold 1,122.84 units beyond the pool on 2017-01-12, covered 2017-01-30; rule in RULING-S22.md."
- Reference check item (c) "no pool oversell" (~:398) → "every pool oversell gets warning 10 and covers back to Quicken's holding."
- Unknown-cost bullet (~:206): "until its pool shares next reach 0" → "…reach 0 or below; an oversold sale is marked too".
- `acbPool.take` doc (`acb_walk.go:399-400`) describes the short.
- Edge rows: "Sale beyond holdings → short, warning 10a, unknown cost"; "Buy while short → covers units only, excess at pro-rata cost"; "Short open today → no text row; JSON shares < 0, incomplete true"; "ROC while short → skipped, existing not-held warning".
- No change to the Long or SKILL.

## Reference-check explanations (S21)

Acceptable only once verified: USD same-day multi-buy — recompute with Quicken's lot pick to 0.00 difference; no-cost add_shares — sale marked `unknown_cost` and the add listed by `quarry findings --type shares-without-cost`. Anything else that does not close is an unexplained difference → new scenario.

## Orchestrator rulings (2026-10-05)

- 10a/10b `<x>` is the short after the disposition (a pool at -10 that sells 5 shows 15).
- A no-cost acquisition that only covers a short keeps its event `UnknownCost` true; it opens no span.
