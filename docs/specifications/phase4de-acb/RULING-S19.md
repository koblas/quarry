# Copy ruling — SCENARIO-19 MCP `acb` (product-vision, 2026-10-05)

Supersedes specification.md :306 and SCENARIO-19.md items 1–8 where they differ. Implement verbatim.

## 0. The MCP rewording rule

Precedent: `accountRefusal`/`categoryRefusal` (internal/mcp/accounts.go:20-46), `document.NativeParameter` (internal/report/document/networth_rate_warnings.go:19). The sentence stays; only the CLI command phrase becomes the tool phrase:

- `quarry findings --type <t>` → `data_quality with type <t>`
- `quarry findings --type unclassified-account --status all` → `data_quality with type unclassified-account and status all`
- `quarry acb --security <id>` → `acb with security <id>`
- `quarry acb --json` (bare) → `acb with no arguments`

Verbatim on MCP: `run quarry sync …` (user-only), the statement `quarry acb has nothing to show`. The parity acceptance test asserts tool warnings = CLI warnings with exactly these substitutions.

## 1. R-6 on MCP

- Client isError, N = 1: `acb needs every brokerage and retirement account classified; 1 account is in neither accounts.registered nor accounts.non-registered in ~/Library/Application Support/quarry/config.toml; data_quality with type unclassified-account and status all lists it`
- N ≥ 2: same with `N accounts are … lists them`.
- stderr: `quarry: mcp: acb: <the client text>` via `verbatim()`.
- Mechanism: new `report.RefusalUnclassifiedAccounts` and a count field (e.g. `Count int`) on `RefusalError`, set by `unclassifiedAccountsRefusal` (refusal.go:112-123). CLI text unchanged. `refusalLine` (result.go:87) gets an arm.

## 2. Unknown security

- Client: `acb covers no security named "XYZ"; acb with no arguments lists every security it covers` (`%q`; first unmatched selector in argument order; `""` included).
- stderr: `quarry: mcp: acb: refused the call's security; details went to the client only`. The `RefusalUnknownSecurity` arm at result.go:87 becomes this class line.

## 3. `year` is an integer

- Schema `{"type":"integer","minimum":1,"maximum":9999}`; input `Year *int` (absent = no year).
- Handler: `report.ParseACBYear(fmt.Sprintf("%04d", y), now)` — one owner.
- Integer 24 accepted (like CLI `--year 0024`); result carries warning 2 `no sales in 24 …`.
- Next year or later: client `year 2027 is after this year; pass this year or an earlier one`; stderr `quarry: mcp: acb: refused the call's year; details went to the client only`.
- `"2024"` string, `0`, `10000`: schema rejects → generic `refused the call's arguments; details went to the client only`.
- The NotAYear arm is unreachable on MCP: `// unreachable: schema bounds year to 1..9999 and %04d always yields four digits`; no client copy.
- Bad-year rows become: `"2024"` string, 0, 10000 (schema), this year accepted, next year refused, 24 accepted.

## 4. Parameter descriptions

- `year`: `Tax year to report, such as 2024, up to this year: years lists only that year (even with no sale), and securities only those with a sale, or a return of capital above ACB, in it; each security's events stay its full history. Omit it for every year.`
- `security`: `Report only these securities, each given by id, ticker or name in any letter case; years and securities count only them. A security held only in registered accounts has no ACB and is left out, with a warning. Omit it for every security.`
- Tool description: spec :306 unchanged.

## 5. Warnings naming CLI commands — tool-worded on MCP

- 4a: `…; data_quality with type shares-without-cost lists them`
- 4b: `…; enter their cost in Quicken; acb with security <security_id> lists them`
- 4c: `…; data_quality with type shares-without-cost lists the added shares; enter the reinvested dividends' cost in Quicken`
- Verbatim: slot 2 form 1, 6b, every other slot. Config/adjustment warnings use absolute `cfg.Path`.
- Mechanism: `document.ACBWarnings` takes an advice value shaped like `NativeAdvice` (CLI value / MCP value). internal/cli/acb.go:86 and :88 pass the CLI value. Warnings still from the UNCUT report.

## 6. SKILL §8 row (directly before the generic `Failure` row)

`| acb refused: accounts not classified | exit 1, stderr ¤quarry: acb needs every brokerage and retirement account classified; …¤ | Classify them (¤references/findings.md¤, "Classifying accounts"), then run ¤quarry acb¤ again. |`

## 7. Text now false

- finding.go:322 Sentence: `Add this account's id to accounts.registered if it is an RRSP, RRIF, TFSA, RESP, FHSA, LIRA, a US 401(k) or IRA, or a similar registered plan, else to accounts.non-registered, in ~/Library/Application Support/quarry/config.toml; quarry acb leaves registered accounts out`
- `dataQualityDescription` (tools.go:77-84), same wrap width:

```
List the data-quality findings quarry's last sync found: problems to fix
in Quicken (duplicates, one-sided or unlinked transfers, uncategorized
splits, payees in mixed categories, payee name variants, similar or
unused categories, shares added with no cost), and investment accounts
not yet listed as registered or non-registered in quarry's config file,
which acb needs. Each finding has an id, the suggested fix, and the
transactions, accounts, payees or categories it is about. quarry never
fixes them: the user fixes them in Quicken and runs quarry sync, or adds
the account to quarry's config file, and they drop off. To ignore a
finding the user adds its id to findings.ignore in quarry's config file.
```

- `instructions` (tools.go:42-53): unchanged.

## 8. Event cap, Tools line, SKILL §9

- Cap 500 total `events`, document order (security order, then event order); every `years[]` and `securities[]` header kept; a later security keeps its header with `events: []`. N counted after the cut.
- Last warning: `acb lists the first 500 events of 1,234; pass security to narrow` (same advice when `security` given; `year` not offered).
- `mcp --help` (cli/mcp.go:40-41): `Tools: describe_schema, query, sync_status, data_quality, spending,\ncash_flow, recurring_charges, anomalies, search_transactions, holdings,\nnet_worth, acb.`
- SKILL §9: `… `search_transactions`, `holdings`, `net_worth`, `acb`, `data_quality` for the commands in section 4 …`

## Pins affected

- Scope widens: internal/report (RefusalKind + count field; refusal tests), internal/report/document (`ACBWarnings` advice param; every acb_warnings test), internal/cli/acb.go:86,88, internal/finding, internal/mcp.
- Acceptance parity (`run_mcp_acb_test.go`): fixture includes a 4a or 4b security.
- `run_mcp_descriptions_test.go`: acb schema const (integer year 1..9999; both descriptions); dataQualityDescription pin (~:50-56).
- `internal/finding/finding_test.go:218` Sentence — first run a flattened, positive-controlled search for other fix-sentence pins (goldens, CSV, `--json`).
- `cli/mcp_test.go:43-45`, `run_mcp_descriptions_test.go:286`: Tools line.
- `run_skill_text_test.go:226,240`: §8 row, §9.
- `result.go` log tests (`log_classes_internal_test.go`, `log_internal_test.go`): R-6 verbatim row, security class row.
- STATE.md Open debts: drop "finding.go:325 LIRA" and "dataQualityDescription — final pass" (resolved here).
