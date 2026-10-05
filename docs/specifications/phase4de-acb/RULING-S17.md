# Copy ruling — SCENARIO-17 (product-vision, 2026-10-05)

Supersedes specification.md :159, :295, :299, :361, :363 and the SCENARIO-17 plan where they differ. Implement verbatim.

## P0 precedence

positional argument (2) → `--currency` (2) → `--year` (2) → config refusal (1) → store open/read refusal (1) → R-6 (1) → unknown `--security` (1). In `--json` mode every refusal leaves stdout empty and prints the same stderr line.

## 1. R-6 (exit 1)

- N = 1: `quarry: acb needs every brokerage and retirement account classified; 1 account is in neither accounts.registered nor accounts.non-registered in ~/Library/Application Support/quarry/config.toml; quarry findings --type unclassified-account --status all lists it`
- N ≥ 2: `quarry: acb needs every brokerage and retirement account classified; N accounts are in neither accounts.registered nor accounts.non-registered in ~/Library/Application Support/quarry/config.toml; quarry findings --type unclassified-account --status all lists them`
- N = 0 never produces the line. Counts open and ignored unclassified accounts, closed ones included.

## 2–3. acb `--currency` (exit 2)

- Every non-CAD value (USD, native, EUR, `""`, anything unreadable, any case): `quarry: acb is in CAD only, as the CRA requires; run it without --currency`
- CAD in any case is accepted and ignored. No value is echoed. The generic `--currency must be CAD, USD or native` line is NOT used for acb. Help string unchanged.

## 4. SKILL §6 (one sentence)

Replace "; quarry never writes that file, and you don't either unless the user asks." with:

`. quarry never writes that file. You write it only when the user asks, to record an account classification the user just gave you (references/findings.md, "Classifying accounts"), or to add acb.adjustment lines for amounts the user reads you from a T3 slip (references/findings.md, "ACB adjustments"); show the user the exact lines first, and write them only after the user says yes.`

In SKILL.md the code spans are `¤findings.ignore¤`, `¤~/Library/Application Support/quarry/config.toml¤` and `¤acb.adjustment¤`.

findings.md "Ignoring a finding" bullet (:34) mirror, verbatim (same words in backticks):

`- quarry never writes that file. You write it only when the user asks, to record an account classification the user just gave you (see "Classifying accounts"), or to add acb.adjustment lines for amounts the user reads you from a T3 slip (see "ACB adjustments"); show the user the exact lines first, and write them only after the user says yes.`

## 5. SKILL §4

- acb row directly after the holdings row: `| ACB, capital gains for a tax year | ¤quarry acb [--year <y>] [--security <s>] --json¤; see ¤references/findings.md¤ "Classifying accounts" first |`
- Trigger = LAST paragraph of §4 (after "Dates are …"): `The first time ACB or gains are asked for, run ¤quarry findings --type unclassified-account --status all --json¤; if it lists any account, classify them (¤references/findings.md¤, "Classifying accounts") before running ¤quarry acb¤.`

## 6. Classification question

`(RRSP, RRIF, TFSA, RESP, FHSA, LIRA, a US 401(k) or IRA, or similar)`

## 7. findings.md new sections

After "Ignoring a finding", before "A spreadsheet", in this order:

````
## Classifying accounts

- quarry acb needs every brokerage and retirement account, open or closed, listed as registered or non-registered in `~/Library/Application Support/quarry/config.toml`. Quicken's file does not say which is which.
- List the ones left with `quarry findings --type unclassified-account --status all --json`. Ignoring one does not stop quarry acb from needing it.
- For each, ask the user: "Is <account> (<type>, <currency>) a registered plan (RRSP, RRIF, TFSA, RESP, FHSA, LIRA, a US 401(k) or IRA, or similar) or non-registered?" Never guess from the account's name or from Quicken calling it a retirement account.
- Read config.toml first. Add each id to the existing `registered` or `non-registered` list under `[accounts]`, or wherever the file already sets them (`accounts.registered = …`, `accounts = { … }`). If the file has neither, add an `[accounts]` table at its end. Never write a second `[accounts]` line.
- Put a `# <account name>` comment beside each id, show the user the exact lines, and write them only after the user says yes.
- Then run `quarry findings --type unclassified-account --status all --json` to confirm none are left, and relay any line in `warnings`.
- Without access to the file (through the MCP server), give the user the lines to add themselves.

```toml
[accounts]
registered = [
  "acct-12",  # Questrade TFSA
  "acct-15",  # RBC RRSP
]
non-registered = ["acct-3"]  # Questrade Margin
```

## ACB adjustments

- quarry acb takes return of capital and reinvested distributions, which a fund reports on a T3 slip and Quicken does not hold, from `acb.adjustment` items in the config file. Ask the user for each amount, which kind it is, its security and its date; never compute or guess one.
- The security's id (`sec-…`) is `security_id` in `securities` of `quarry acb --json`.
- Read config.toml first and add one `[[acb.adjustment]]` item per amount at the end of the file. Show the user the exact lines, and write them only after the user says yes.
- Then run `quarry acb --json` and relay any `acb.adjustment` line in `warnings`.

```toml
[[acb.adjustment]]
security = "sec-41"  # XEQT
date = 2024-12-31
return-of-capital = 12.34

[[acb.adjustment]]
security = "sec-41"  # XEQT
date = 2024-12-31
reinvested-distribution = 56.78
```
````

## Pins affected

- SCENARIO-17.md: binding "unreadable value keeps generic line" reversed; Step 1 `--currency usd`/`native`/`USD` rows expect the one acb line; Step 4 `Test_acb_currency_flag_accepts_only_cad`: EUR gets the acb line.
- Shared bad-value tables: acb excluded or own row — `internal/cli/currency_test.go:20,22-27,68-80`; `cmd/quarry/run_read_usage_test.go:14,81`; `cmd/quarry/run_usage_test.go:212`.
- Docs pins: `cmd/quarry/run_skill_text_test.go:189` (§4 row + trigger last), `:218` (§6 sentence); `cmd/quarry/run_skill_references_test.go:23,79-91` (mirror bullet, both sections, question, `--status all` confirm, "never write a second `[accounts]` line"). Step 2's acceptance test asserts `--status all` in trigger and confirm.

## Follow-ups (S19 / final pass)

- `internal/finding/finding.go:325` fix sentence lacks LIRA, 401(k)/IRA — align if S19 touches it.
- SKILL §8 has no R-6 row — S19 considers one pointing at "Classifying accounts".
