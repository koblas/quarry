# Final product-vision pass (step 10) — 2026-10-05, HEAD 8453aed

Verdict: SHIP WITH CHANGES. Implement verbatim.

## MAJOR 1 — Quicken names in acb warnings are Go-quoted (`%q`)

`internal/report/document/acb_warnings.go` wraps security and account names in raw `"%s"`; a `"`, newline or ESC in a Quicken name makes the line ambiguous or splits it without the `quarry: warning:` prefix. Rule: every name from the Quicken file goes through `%q` (strconv.Quote), as holdings warnings do (`document/holdings.go:127`, `holdings_left_out.go:38`). Config-echoed values keep `tomlstr.BasicString` (:56, :62 unchanged).

Sites: :59 adjustment not-held (`%s is for %q, which no non-registered account holds on %s; quarry skips it`); :100 slot 2b (`%q is held only in registered accounts, so it has no ACB`); :147/:151/:155 4c/4b/4a leading `"%s"` → `%q`; :172 warning 5 security and `left "%s"` account → `%q`; :195/:198/:201 6c/6b/6a security → `%q` (6c's `("%s")` currency code stays); :219 warning 7 ticker → `%q` and each member `%q`, ", "-joined (reverses "members unquoted"): `"VTI" is 2 securities in Quicken ("Vanguard Total Stock", "Vanguard Total Stock CAD"); quarry keeps a separate ACB for each; if they are the same, merge them in Quicken`; :236 warning 8 security → `%q`; :281 10a / :289 10b security and account → `%q`.

Plain ASCII names without `"` or `\` give the same bytes; printable Unicode kept. Pins that move (warning 7): `cmd/quarry/run_acb_warnings_test.go:20`, `cmd/quarry/run_mcp_acb_warnings_test.go:25`, `cmd/quarry/run_acb_security_history_test.go:79`, `internal/report/document/acb_warnings_slots_test.go:25,38,50,51`. Add one document-level row per slot with a name containing `"` and `\n`: assert the escaped form and a single line. Spec Surface & Copy: "`<security>`/`<account>` quoted like 5/8" in warnings 2b–10 now means Go-quoted (`%q`).

## MAJOR 2 — SKILL §7

- `plugin/skills/quarry/SKILL.md:75` `## 7. Not covered yet` → `## 7. Gains and tax`
- :77 → `- **Realized gains, ACB:** quarry acb is a worksheet to review with an accountant, not a filing: say so, relay every line in its warnings with its numbers (section 2), and never call a loss deductible or denied.`
- The **Tax:** bullet unchanged.
- Pins: `cmd/quarry/run_skill_text_test.go:28` (heading), `:223` (`skillSection7`).

## Open debts ruled

- Closed as built: acb Long rewrap; findings `--help` table alignment; unreadable-config wording on status/sync_status; slot-5 intra order (walk order: security, then event date — now the ruling; pin optional); warning 6c and null `cad`/`gain`; `dataQualityDescription`; findings Long opening stays.
- Record in spec as built — non-table `acb` config: `…/config.toml: acb must be a table, such as [[acb.adjustment]], got <v>; fix the file and run the command again` (exit 1, every command); `acb = { adjustment = 3 }` / `[]` → existing `acb.adjustment must be a list of tables, each under its own [[acb.adjustment]] line, got …`. Inline `acb.adjustment = [{…}]` is refused by that line — accurate, not blocking.
- Follow-ups (PR known limitations, not fixes): split-restored no-cost span residual (1-millionth input); half-tie rounding residual; REVIEW-03 pins; S14 no-rate superficial pin.
- Not reopened: warnings for unnamed securities under `--security` (S16 ruling). NIT: `mcp --help` could add "security names" to the as-Quicken-holds-them sentence.

Re-gate after the fix: test-reviewer and correctness-reviewer on acb_warnings.go and the skill text only.
