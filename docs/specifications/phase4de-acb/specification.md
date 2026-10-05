# Specification: Phase 4d+4e — registered-account classification and ACB

<!-- spec-check: v1 -->

## Intent & Goal

**Primary Goal**: the user can ask "what is my adjusted cost base, and what capital gains did I realize each tax year?" and get a CRA-style worksheet from `quarry acb` and MCP `acb`: average cost per security pooled across non-registered accounts, in CAD. Which accounts are registered is recorded once in quarry's config, by the skill in session, and unclassified investment accounts show up in `quarry findings`.

**Secondary Goals**:
- One owner per rule: the ACB walk lives in `report`; findings that need classification are computed at read time beside `report.Findings`.
- quarry never invents cost: Quicken's recorded cost basis is imported; shares with no cost are flagged and fixable in Quicken.
- Superficial losses are marked, never adjusted.

**Out of Scope**:
- Spouse/joint accounts (user: none in the file).
- Plan-type taxonomy (RRSP vs TFSA …): only registered / non-registered.
- A `quarry classify` command; quarry never writes config itself.
- Settlement dates, holiday calendars, denied-loss amounts and ACB bumps for superficial losses, deemed dispositions at FMV.
- Skill references `net-worth.md`/`investments.md` (4f) and the monthly summary (4g).

**User decisions (2026-10-05)**:
1. Fold 4d (classification) into 4e (`quarry acb`) as one spec.
2. Config names accounts by `acct-<n>` id.
3. Classification records only registered / non-registered (two lists).
4. Claude writes config.toml in session after the user says yes; no new command.
5. Every investment account in neither list is flagged (incl. closed, zero-holding, not-in-reports, linked-tracking).
6. F1: possible superficial losses are a mark + warning in `acb`, not a findings type (PRD amended).
7. F2: no spouse/joint accounts in the file.
8. F3: the 4 USD retirement accounts (IRA/401(k)) are registered — a classification fact the skill records.
9. F4: warn on sales dated December 24–31.
10. Scenarios approved 2026-10-05.

## Business Rules & Invariants

- **R-1 Classification** lives in config (`[accounts] registered / non-registered`, `acct-<n>` ids); not in the store. Finding `unclassified-account` is computed at read time for every account `store.IsInvestmentAccount` accepts that is in neither list; never `new`/`fixed`; `first_found_at` null. Every site that classifies findings with `cfg.Ignore` (findings, status, sync, MCP data_quality, sync_status) also takes the classification so counts agree.
- **R-2 Masking** (PRD :265): any `accounts.*` value or key not exactly `acct-<digits>` is echoed with every digit except the last four replaced by `*`, at every echo site.
- **R-3 Cost basis**: `investment_transactions.cost_basis DECIMAL(18,2)` NULL (Quicken ZCOSTBASIS, NULL when 0); FormatVersion 9.
- **R-4 ACB walk** (CRA rules as ruled below, "CRA rules — what quarry claims"): average cost pooled per `security_id` across `accounts.non-registered`; buy cost = −amount (commission already inside); sale removes ACB × sold ÷ held to the cent (exact remainder on the last unit); gain = proceeds − outlays − ACB removed; amounts converted at the BoC rate for the event date; tax year = calendar year of the recorded date. Architect preconditions P1 (sell amount net or gross of commission) and P2 (pool-internal move pairs; same-day buy+sell order) run before the core walk is planned.
- **R-5 Superficial loss** is a mark + warning only (rule below).
- **R-6 `acb` refuses** while any investment account is unclassified (ignoring the finding does not unblock it).

---

## Triage Brief

### 4d (triage 2026-10-05)

- Config: `internal/config` 4 keys (`snapshots.keep`, `quicken.path`, `findings.ignore`, `reporting.currency`); `knownKeys` hard-coded (parse.go:316), unknown keys warn; list-of-strings precedent `findings.ignore` (parse.go:223-243, spelling preserved); no table-of-tables/map precedent. Loader per command (cli/run.go:41) and per MCP tool call (mcp/server.go:32); warnings to stderr/`warnings[]`.
- **quarry never writes config.toml** (no command; SKILL.md:70 "quarry never writes that file, and you don't either unless the user asks"; findings.md:31 tells the user to hand-edit).
- Accounts: id `acct-<Z_PK>`, name (not unique in schema), type brokerage|retirement for investment (ZTYPENAME BROKERAGENORMAL/OTHER → brokerage; RETIREMENTIRA, DEFERREDCOMPRETIREMENT401K → retirement). Registered type is not derivable: ZTAXABLE = 0 on all 9 investment accounts (doesn't discriminate). No account number on ZACCOUNT; quarry imports none. No masking helper exists.
- Real file (counts): 23 live accounts; investment 9 (BROKERAGENORMAL 4, BROKERAGEOTHER 1, RETIREMENTIRA 2, DEFERREDCOMPRETIREMENT401K 2); 3 investment accounts closed (from 4c probe).
- Findings: 8 types (`internal/finding/finding.go:16-38`), ids `<type>:<entity>` (user contract, pasted into `findings.ignore`); detectors run at sync inside the snapshot build (`duckstore/findings.go:65-118`) and get no config; config reaches findings only at read time via `finding.Classify(states, ignore)` (report/findings.go:68; snapshot.WithIgnore for sync counts). Adding a type touches: finding consts/Types, report display switch (report/findings.go:146-175), cli render switch (cli/render_findings.go:128-139), findings Long + `--type` usage (cli/findings.go:46,119), MCP enum (mcp/tools.go:393), skill ref findings.md (pinned by run_skill_references_test.go:23,79), document/findings.go:74.
- Caller table (grep; LSP cross-package unreliable here): ConfigLoader consumers in every cli command; cfg.Ignore path cli/findings.go:91, cli/status.go:63, cli/sync.go:102, mcp/data_quality.go:31, mcp/sync_status.go:34, report/findings.go:68,69,115, snapshot/import.go:192; Types() users listed above; `store.IsInvestmentAccount` importer/statements.go:54, importer/validate.go:117, report/document/holdings.go:126; accounts table/v_account_balances shape only changes if a store column is added.


### 4e (triage 2026-10-05)

#### PRD (docs/initial-prd.md ~L250-258, L334)
Per security, pooled across non-registered accounts; buys add cost + commissions; sells remove pro-rata ACB, realize gain/loss; reinvested dividends add to ACB; splits/consolidations change units not ACB; USD trades → CAD at trade-date rate; ROC / phantom distributions via a small manual adjustments file per security and year; possible superficial losses (same security bought within 30 days before/after a loss) and positions starting without a purchase (transfer-in/opening balance) flagged as findings, not adjusted; output per-security ACB history + realized-gains summary per tax year (worksheet, not a filing); CAD reporting only; skill asks for classification the first time ACB is requested.

#### Store today
- `investment_transactions` (schema.go:89-103): id itxn-<pk>, account_id, security_id, date, action, shares (signed), amount (cash sign; buy −, sell +; add/remove/reinvest/split 0), commission DECIMAL(18,4) NULL when 0 (no reader yet), currency (= account currency), memo, split_new/old_shares. NO price, NO cost basis column. Importer reads ZUNITS/ZAMOUNT/ZCOMMISSION/ZNUMERATOR/ZDENOMINATOR (importer/investments.go:47-56). FormatVersion 8.
- Share walk `holdingSpans` (duckstore/shares.go:102-147) per (account, security), exact big.Rat, split ratio applied; tracks no cost; not reusable as-is for a pooled cost walk.
- Securities: shared Quicken security id across accounts. 84 securities, 81 tickers: 3 tickers duplicated across 2 security ids (VINIX, VTIAX, VTSAX; USD). 9 securities in >1 brokerage-type account; 46 across brokerage and retirement types.
- FX: fx_rates + ASOF join pattern; FirstRate warnings precedent.
- Twin: `quarry holdings`/`networth` (cli/holdings.go, report/holdings.go, document/holdings.go, mcp/holdings.go, tools.go). `report.ValueReads` port; interfacebloat cap 10 methods on report.Store.
- Findings: sync-time detectors (duckstore/findings.go:81); 4d adds the first read-time type. Superficial-loss / no-purchase-start need classification → read-time too.

#### Real file (counts, snapshot 20261004T184923Z)
Non-registered candidates = BROKERAGE* (5 accts: 2 CAD, 3 USD (1 closed)); retirement-type 4 (USD, 2 closed). ZTAXABLE useless.
| action | brokerage: n / units≠0 / commission≠0 / ZCOSTBASIS≠0 | retirement |
| buy | 92/92/22/92 | 180/180/0/180 |
| sell | 46/46/24/0 | 25/25/11/0 |
| reinvest_dividend | 1/1/0/1 | 4/4/0/4 |
| add_shares | 74/25/0/1 | 169/145/0/0 |
| remove_shares | 7/1/0/0 | 32/26/0/0 |
| split | 1 | 0 |
- Commission is INSIDE `amount` on buys (ZCOSTBASIS = |amount| on all 22 commission buys) → "cost + commissions" would double count. Sell `amount` net-of-commission unproven.
- Reinvest: amount 0, ZCOSTBASIS non-zero on all 5 → cost source would be ZCOSTBASIS (new column + importer read + format bump).
- Transfer-in: 24 of 25 brokerage add_shares with shares carry no cost (ZCOSTBASIS 0); 1 carries cost + ZACQUISITIONDATE.
- Lot tables: ZLOT (630; initial/latest cost basis & units, acquisition date), ZLOTMOD (1054; per-transaction before/after cost basis & units), ZLOTASSIGNMENT 0. Quicken cost removed per sell derivable from ZLOTMOD. Quicken's algorithm (ZPOSITION.ZCOSTBASISALGORITHM=1) is not CRA pooled average → no exact match expected.
- No stored realized gain, no price column on ZTRANSACTION.
- Trades are in account currency (no security/account currency mismatch). ~20 CAD vs ~118 USD brokerage trade rows.
- Brokerage buys/sells by year: 2015 17/11, 2016 29/1, 2017 16/13, 2018 2/0, 2019 5/13, 2020 3/4, 2021 8/1, 2022 2/1, 2023 2/0, 2024 8/2.
- No pooled oversell.

#### Caller table (grep; LSP cross-package unreliable)
- cfg.Ignore sites (4d classification too): mcp/sync_status.go:34, mcp/data_quality.go:31, cli/findings.go:91, cli/sync.go:102, cli/status.go:63; report.CountFindings (report/findings.go:92; mcp/sync_status.go:24); knownFindings/Classify report/findings.go:68-69,99; finding.Types/Known finding.go:28,36; --type literal cli/findings.go:119; document.Finding.FirstFoundAt document/findings.go:24,80.
- config: knownKeys parse.go:316-325; Config config.go:21-36.
- acb surface: root.AddCommand cli/root.go:28-41; sdk.AddTool + tool consts mcp/tools.go:27,264-316; report.Store/ValueReads report/store.go:9-37 + fakes (mcp/query_helpers_test.go fakeStore, timeout_test.go stallingStore embed nil report.Store → stubs needed).
- New investment_transactions column touches: store.InvestmentTransaction store.go:277; Rows store.go:320; duckstore.go:518; shares.go:82,31,86; importer/investments.go:21; schema.go:89,147; holdings.go:72; status.go:25; history.go:44; report/sql_conventions.go:24 (+hand copies cli/sql_test.go, cmd/quarry/run_shared_documents_test.go, schema.md); document/common.go:34; FormatVersion.


## Product Verdict

4d: SHIP WITH CHANGES (fold into 4e — accepted by the user). 4e: SHIP WITH CHANGES (changes 1-7 below, accepted). User forks F1-F4 decided as listed in Intent & Goal.

## Surface & Copy

### Part A — 4d classification (product-vision ruling)

User decisions (2026-10-05): fold 4d into 4e; config names accounts by `acct-<n>` id; record only registered / non-registered (two lists); Claude writes config.toml in session after the user says yes (no new command). Flag every investment account (incl. closed, zero-holding, not-in-reports, linked-tracking) that is in neither list (PV recommendation; user did not object).

#### Shape
- Finding detected at READ time (classification lives in config, not the snapshot): every site in the `cfg.Ignore` caller table also takes the classification so findings/status/sync/data_quality/sync_status agree; rule lives in `report` beside Findings/CountFindings; `status` path needs an accounts read (store.Status carries none); these findings are never `new`/`fixed`; `first_found_at` becomes null for this type (document/findings.go:24 non-pointer string today).
- Account set = every account `store.IsInvestmentAccount` accepts; no exceptions.

#### Config
```toml
[accounts]
registered = [
  "acct-12",  # Questrade TFSA
  "acct-15",  # RBC RRSP
]
non-registered = ["acct-3"]  # Questrade Margin
```
knownKeys gains `{accounts}`, `accounts.registered`, `accounts.non-registered`. Exact, case-sensitive matching; duplicates within a list tolerated.

Refusals (badValue/lookup precedent, every command, exit 1, `quarry: ` prefix, end `; fix the file and run the command again`; `<X>` registered|non-registered; `<v>` masked):
- `~/Library/Application Support/quarry/config.toml: accounts.<X> must be a list of account ids in quotes, such as ["acct-12"], got <v>`
- `…: accounts.<X> must hold only account ids in quotes, got <v> as item <n>`
- `…: an account must be in only one of accounts.registered and accounts.non-registered, got "<id>" in both`
- `…: accounts must be a table, such as accounts.registered = ["acct-12"], got <v>`

Warning (exit unchanged; findings, accounts, MCP data_quality; `~` path stderr, absolute in `warnings[]`; copies UnmatchedIgnoreWarnings):
- `~/Library/Application Support/quarry/config.toml: accounts.<X> lists "<v>", which is not an account in quarry's store; quarry skips it`

Masking (PRD :265): any value from `accounts.*` or key under `accounts` not exactly `acct-<digits>` echoed with every digit except the last four replaced by `*` ("12345678" → "****5678"; "RBC 2019 TFSA" unchanged). Sites: refusal got-text, "as item" text, "in both", unmatched warning, unknown-key warning for keys under [accounts]. One helper in internal/platform. Developer must check whether go-toml syntax-error text (parse.go:104) echoes value tokens; if so, mask there too.

#### Finding `unclassified-account`
- appended last in Types(); entity = account id; id `unclassified-account:acct-12`; order account name case-insensitive then id; ignorable.
- Heading: `Unclassified investment accounts`
- GroupClause: `list each account's id (acct-…) in accounts.registered or accounts.non-registered in ~/Library/Application Support/quarry/config.toml; see quarry findings --help`
- Sentence / JSON `fix` (fold variant): `Add this account's id to accounts.registered if it is an RRSP, RRIF, TFSA, RESP, FHSA or other registered plan, else to accounts.non-registered, in ~/Library/Application Support/quarry/config.toml; quarry acb leaves registered accounts out`
- Text row: id padded to widest, name padded+escaped (escapeCell), type, currency, `, closed` when closed, ignored marker:
```
Unclassified investment accounts (2): list each account's id (acct-…) in accounts.registered or …
  unclassified-account:acct-31  Old RRSP        retirement, CAD, closed
  unclassified-account:acct-12  Questrade TFSA  brokerage, CAD
```
- JSON item: `account_id`, `account`, `currency` set; other item keys null; `first_found_at` null; `fixed_at` null.
- CSV: account, currency filled; others empty; id in finding_id.
- `--type` usage literal cli/findings.go:119 gains `unclassified-account`; MCP enum derives from Types().
- Long table row: `unclassified-account  a brokerage or retirement account listed in neither accounts.registered nor accounts.non-registered in the config file`
- findings.md bullet: `` `unclassified-account`: a brokerage or retirement account, open or closed, that the config file lists as neither registered nor non-registered. Fixed in quarry's config, not Quicken; see "Classifying accounts". ``

#### accounts
- Status: append `registered` for any listed account (non-investment accepted); `unclassified` for an investment account in neither list; non-registered gets nothing. e.g. `closed, not in reports, registered`.
- JSON: `"registered": true|false|null` after `linked_tracking`; null when in neither list.
- Long new paragraph: `Status says registered for an account listed in accounts.registered in the config file, and unclassified for a brokerage or retirement account in neither accounts.registered nor accounts.non-registered; quarry findings lists those.`

#### Changes to existing surfaces
- findings Long after the type table: `unclassified-account is fixed in the config file, not in Quicken: it leaves the list as soon as the account is listed there, without a sync, and is never marked fixed.` + the TOML example.
- `--csv` help: `print one row per transaction, split, payee, category or account as CSV`.
- Unreadable-config warning on `quarry status` / MCP `sync_status` (replaces the phase2d/phase3a line; product-vision ruling 2026-10-05, SCENARIO-05): `cannot tell which findings you ignored or how you classified your accounts: <problem>; findings you ignored are counted as open, and every investment account is counted as unclassified` (`<problem>` `~` on stderr, absolute in `--json`/MCP; exit 0). Behaviour: with config unreadable, every investment account counts as an open `unclassified-account` (never under-report). Symbol `CannotTellIgnored` → `CannotTellChoices`.
- findings.md `--csv` mirror (`plugin/skills/quarry/references/findings.md:43`): `prints one row per transaction, split, payee, category or account` (orchestrator ruling 2026-10-05, mirror of the ruled help; 13b moves it to the Part B wording with `account or investment transaction`).
- findings Short unchanged.
- SKILL.md frontmatter: after "payee name variants)" add `, or which investment accounts are registered`.
- SKILL.md §6: replace "quarry never writes that file, and you don't either unless the user asks." with `quarry never writes that file. You write it only when the user asks, or to record an account classification the user just gave you (references/findings.md, "Classifying accounts").` Mirror in findings.md "Ignoring a finding". Pins run_skill_references_test.go:23,79 move.
- findings.md first line → `what to clean up in their Quicken file, and which accounts quarry needs classified`.
- schema.md (+ describe_schema if shared): `Which accounts are registered is not in the store; it is accounts.registered and accounts.non-registered in quarry's config, and quarry accounts --json reports it as registered.`
- findings.md new section "Classifying accounts": per unclassified-account ask "Is <account> (<type>, <currency>) a registered plan (RRSP, RRIF, TFSA, RESP, FHSA, LIRA or similar) or non-registered?" — never guess from the name or Quicken's "retirement" type; read config.toml first, add to existing [accounts] lists, never a second [accounts] header; show exact lines with `# <account name>` comments; write only after yes; then run `quarry findings --type unclassified-account --json` to confirm and relay warnings; without file access (MCP) give the lines. 4e adds the "first time ACB is requested" trigger to SKILL.md.

#### Edge rows
| Input | Finding | accounts Status / registered | Config line |
|---|---|---|---|
| Closed investment account, unlisted | row, `, closed` | `closed, unclassified` / null | none |
| Zero holdings / no investment txns, unlisted | row | `unclassified` / null | none |
| Not in reports / linked tracking, unlisted | row | `not in reports, unclassified` / null | none |
| Renamed in Quicken | none (id unchanged) | new name, class kept | none |
| Two accounts same name | two rows (id/type/currency) | each own class | none |
| `acct-99` names no account | none | n/a | warning, exit 0 |
| Name or number written instead of id | account stays listed | unclassified | warning, masked |
| `accounts.registred` (misspelled key) | account stays listed | unclassified | unknown-key warning |
| Unquoted number item | n/a | n/a | refusal, masked, exit 1 |
| Same id in both lists | n/a | n/a | refusal, exit 1 |
| Non-investment account listed | none | `registered`/true (or nothing/false) | none |
| Classified and in findings.ignore | gone | class shown | existing "not a finding" warning |
| No store yet | existing refusal | existing refusal | none |

### Part B — 4e ACB (product-vision ruling)

User decisions (2026-10-05): F1 superficial loss = mark + warning in `acb`, NOT a findings type (PRD amended); F2 no spouse/joint accounts (pool all non-registered); F3 the 4 USD retirement accounts (IRA/401k) are registered (a classification fact the skill records); F4 warn on Dec 24–31 sales.

#### Changes (verdict SHIP WITH CHANGES)
1. Superficial loss: flag inside `acb` (needs ACB walk; no Quicken fix). `shares-without-cost` IS a findings type (fixable in Quicken).
2. Import ZCOSTBASIS → `investment_transactions.cost_basis DECIMAL(18,2) NULL` (NULL when 0), every investment txn; FormatVersion 8 → 9.
3. Never add commission on top of a buy: buy cost = −amount (commission inside amount, 22/22).
4. Architect preconditions before the core ACB scenario: P1 sell amount net or gross of commission (compare amount vs units × prices row on that date for the 24 commission sells); P2 count pool-internal share-move pairs and same-day buy+sell days.
5. `acb --currency`: CAD accepted; USD/native refused exit 2 (flag defined, so SKILL §8 "unknown flag = old binary" stays true).
6. MCP `acb` joins this spec (LIGHT).
7. Adjustments in config.toml as `[[acb.adjustment]]`; amounts parsed from literal text, never float64.

#### CRA rules — what quarry claims
- Average cost pooled per security (security_id) across accounts in `accounts.non-registered` only.
- Buy adds full cash cost (−amount, commission included). Sale removes ACB × sold ÷ held, rounded to the cent; selling the last unit removes the exact remainder. Gain = proceeds − outlays − ACB removed (Schedule 3 columns).
- Commission (sells, per P1): net (expected) → proceeds = amount + commission, outlays = commission; gross → proceeds = amount, outlays = commission. Commission DECIMAL(18,4) → outlays rounded to the cent half away from zero.
- Split/consolidation changes units once per security per date (even if several accounts record it); ACB unchanged. Reinvested dividend adds its `cost_basis`; NULL cost_basis → units at 0 cost, security `incomplete`, warning 4 (no finding).
- add_shares shares > 0 not a pool-internal move: adds units + cost_basis when non-NULL; NULL → units at 0.00, security `incomplete` from that date, finding `shares-without-cost`. Pool-internal move (same security and date, equal and opposite shares, both accounts non-registered) changes nothing (`moved` row). Unpaired remove_shares: removes pro-rata ACB, no gain, warning 5. Pairing ships only if P2 finds ≥1 pair; else every add/remove is unpaired (same copy).
- Ordering within a day: Quicken order (source_id) unless P2 shows a same-day buy and sell that order would invert → then "acquisitions before dispositions on the same day" (and say so).
- USD: each amount at the BoC rate for its date or latest earlier, rounded to the cent per event. Trade before first stored rate → security incomplete, left out of year totals (never valued at zero), warning 6.
- Tax year = calendar year of the date Quicken records; Dec 24–31 sales warned (warning 9). Not computed: settlement dates, holidays.
- Superficial loss (ITA s.54, CRA position incl. own RRSP/TFSA): flag a sale at a loss when, in ANY account in the file (registered included), the same security or one with the same ticker was acquired (buy, reinvested dividend, added shares; other than the shares sold) within 30 days before or after, and the file still holds some at end of day +30. Not claimed: spouse/affiliated outside the file, denied amount, ACB bump, permanent denial via registered. Gain stays as computed, marked.
- ROC above ACB: ACB → 0.00, excess is a capital gain on that date (ITA s.40(3)), warning 8.
- Pool by security_id; same-ticker securities in the pool → warning 7; superficial-loss detection treats same-ticker as identical.
- Worksheet, not a filing.

#### Surface & Copy — `quarry acb`
- Use `acb`; Short `Show adjusted cost base and realized capital gains per tax year, in CAD`
- Long (verbatim):
```
Show the adjusted cost base (ACB) of each security and the capital gains
and losses realized each tax year, the way the CRA defines them: average
cost per security, pooled across every account listed in
accounts.non-registered in ~/Library/Application Support/quarry/config.toml.
Registered accounts are left out. A buy adds what it cost, commission
included; a sale removes its share of the ACB, and its gain is the
proceeds less commission less that ACB. Reinvested dividends add their
cost; splits change shares, not ACB. USD trades convert to CAD at the Bank
of Canada rate for their date. Return of capital and reinvested
distributions from T3 slips come from acb.adjustment in the config file.

Amounts are always in CAD, as the CRA requires; reporting.currency does not
apply. A sale's tax year is the year of the date Quicken records, usually
the trade date; the CRA uses the settlement date, so check late-December
sales against your T5008.

Possible superficial losses are marked, not adjusted. Shares added with no
cost count at no cost until you enter it in Quicken (quarry findings
--type shares-without-cost). This is a worksheet to review with your
accountant, not a tax filing.
```
- Example: `  quarry acb` / `  quarry acb --year 2024` / `  quarry acb --security XEQT --json`
- Flags: `--year` `list the sales in tax `year` (YYYY) one by one`; `--security` `show the full history of the security with this `name`, ticker or id; repeat for more`; `--currency` `ACB is in CAD only; any other `currency` is refused`; `--json` global.

Text default:
```
Realized capital gains by tax year, in CAD

Year  Sales    Proceeds  Outlays         ACB  Gain or loss
2017     13   48,210.55    89.87   41,002.10      7,118.58
2024      2   12,004.00     9.99   12,900.40       -906.39  1 possible superficial loss

ACB on 2026-10-05, in CAD

Security                       Ticker   Shares         ACB  ACB per share
iShares Core Equity ETF        XEQT    410.000   10,412.33        25.3959
Vanguard Total Stock Market    VTI      62.000    9,880.01       159.3550  incomplete
```
- Year rows: only years with a sale; no cross-year total. Suffixes ", "-joined in order: `N possible superficial loss(es)` (humanize.Count), `N sale(s) of shares with unknown cost`.
- Position rows: securities with shares > 0 today, by name ignoring case; suffix `incomplete` when ACB rests on no-cost shares or a no-rate trade; shares as stored thousands-grouped; ACB per share = ACB ÷ shares, 4 decimals, display only.

Text `--year 2024`: caption `Sales in 2024, in CAD`; columns `Date  Security  Shares  Proceeds  Outlays  ACB  Gain or loss` (security = ticker else name, escapeCell); `Total` row; per-row suffixes `possible superficial loss` / `unknown cost`.

Text `--security`: caption `ACB history of "<name>" (<ticker>), in CAD`; columns `Date  Account  Action  Shares  Amount  Rate  CAD  Shares held  ACB  Gain or loss`; Amount = native with code (`-1,234.56 USD`); Rate blank for CAD; actions = stored names + `return of capital`, `reinvested distribution`, `moved` (pool-internal pair, one row); one block per security, blank line between.

`--json` (one shape):
```json
{"as_of":"2026-10-05","currency":"CAD","year":null|2024,
 "years":[{"year":2024,"sales":2,"proceeds":"…","outlays":"…","acb":"…","gain":"…",
   "possible_superficial_losses":1,"unknown_cost_sales":0,
   "sales":[{"date":"…","investment_transaction_id":"itxn-9","security_id":"sec-41","security":"…","ticker":"XEQT"|null,
     "account_id":"acct-3","account":"…","shares":"10","proceeds":"…","outlays":"…","acb":"…","gain":"…",
     "possible_superficial_loss":true,"unknown_cost":false}]}],
 "securities":[{"security_id":"sec-41","security":"…","ticker":"XEQT"|null,"currency":"CAD",
   "shares":"410","acb":"10412.33","acb_per_share":"25.3959"|null,"incomplete":false,
   "events":[{"date":"…","investment_transaction_id":"itxn-9"|null,"account_id":"acct-3"|null,"account":…,
     "action":"buy","shares":"10","amount":"-1234.56"|null,"amount_currency":"USD"|null,"usd_cad":"1.3512"|null,
     "cad":"-1668.14","outlays":null|"9.99","shares_held":"…","acb":"…","gain":null|"…"}]}],
 "warnings":[]}
```
- `years`: every year with a sale; with `--year` just that year (empty `sales` still listed). `securities`: every security with a pool event; `--security` the named; `--year` those sold that year; `events` always full history. `acb_per_share` null at 0 shares. Adjustment events: null `investment_transaction_id`/`account`. Arrays `[]` never null.

Warnings (stderr `quarry: warning: `, exit 0, also `warnings[]` with absolute paths), order:
1. Config warnings (incl. adjustment warnings).
2. Empty: `no non-registered account has bought or sold a security; quarry acb has nothing to show`; with `--year`: `no sales in 2024 in non-registered accounts; the sales are in <first year>–<last year>` (or the first form when none at all).
3. `N possible superficial loss(es) in <years>: the same security was acquired within 30 days before or after the sale, in any account, and still held 30 days after; quarry does not deny or adjust these losses; review them with your accountant`
4. `"<security>" has shares added with no cost, so its ACB is too low and its gains too high; quarry findings --type shares-without-cost lists them`
5. `"<security>": <shares> shares left "<account>" on <date> without a sale; quarry took their share of the ACB out and reports no gain; if they went to a registered account or to someone else, that is a disposition at market value; check it with your accountant`
6. `"<security>" has a <USD> trade on <d>, before <first>, the first exchange rate in the store, so its ACB is incomplete and its gains are left out of the year totals`; no rates: `… and the store has no exchange rates, so …; run quarry sync to fetch rates`
7. `"<ticker>" is <n> securities in Quicken (<name1>, <name2>); quarry keeps a separate ACB for each; if they are the same, merge them in Quicken`
8. `"<security>": return of capital on <d> is <x> more than its ACB, so its ACB is 0.00 and <x> is a capital gain in <year>`
9. `N sale(s) dated December 24–31, <year>: a sale settles a day or two after its trade date and counts for tax in the year it settles; check its date on your T5008`

Refusals:
| Input | Line | Exit |
|---|---|---|
| `--currency USD`/`native` | `quarry: acb is in CAD only, as the CRA requires; drop --currency USD` | 2 |
| `--year 24`, `--year 2024-03` | `quarry: --year "24" is not a year; use YYYY, such as 2024` | 2 |
| `--year` after this year | `quarry: --year 2027 is after this year; pass this year or an earlier one` | 2 |
| `--security` names nothing | `quarry: no security named "XYZ"; run quarry acb to list the securities it covers` | 1 |
| Any investment account unclassified (open or ignored) | `quarry: acb needs every brokerage and retirement account classified; N are in neither accounts.registered nor accounts.non-registered in ~/Library/Application Support/quarry/config.toml; quarry findings --type unclassified-account --status all lists them` | 1 |
| No store, format 8 | existing lines | 1 |
| Positional argument | existing | 2 |
- `--security` naming a security held only in registered accounts: exit 0, empty `securities`, warning `"<name>" is held only in registered accounts, so it has no ACB`.
- Ignoring an unclassified-account finding does not unblock acb.

#### MCP `acb` (LIGHT)
Params `year`, `security` (array). Description: `Adjusted cost base and realized capital gains per tax year, in CAD, the way the CRA defines them: average cost per security pooled across non-registered accounts; possible superficial losses marked, not adjusted. A worksheet, not a filing.` Result = the `--json` document; no `currency` param. Refusals = CLI lines with `year` for `--year`; stderr `quarry: mcp: acb: refused the call's <param>; details went to the client only`. Cap total `events` 500 via capList: `acb lists the first 500 events of N; pass security to narrow`.

#### Adjustments (config.toml)
```toml
[[acb.adjustment]]
security = "sec-41"             # XEQT, 2024 T3 box 42
date = 2024-12-31
return-of-capital = 12.34

[[acb.adjustment]]
security = "sec-41"
date = 2024-12-31
reinvested-distribution = 56.78  # T3 box 21 not paid in cash
```
- knownKeys: `{acb}`, `acb.adjustment`, `.security`, `.date`, `.return-of-capital`, `.reinvested-distribution`. `[[ ]]` is new parser work (`gotTableList` refused today, parse.go:305). Amounts from raw token text (parse.go:309 `e.value`) as decimals. A bad item refuses every command (intended).
- Refusals (exit 1; path `~/Library/Application Support/quarry/config.toml`; end `; fix the file and run the command again`; `<n>` 1-based file order):
  - `…: acb.adjustment must be a list of tables, each under its own [[acb.adjustment]] line, got <v>`
  - `…: acb.adjustment item <n> needs security, such as security = "sec-41"`
  - `…: acb.adjustment item <n> needs date, such as date = 2024-12-31`
  - `…: acb.adjustment item <n> needs return-of-capital or reinvested-distribution, such as return-of-capital = 12.34`
  - `…: acb.adjustment item <n>: security must be a security id in quotes, such as "sec-41", got <v>`
  - `…: acb.adjustment item <n>: date must be a date such as 2024-12-31, got <v>`
  - `…: acb.adjustment item <n>: <key> must be an amount in CAD above 0 with at most two decimals, such as 12.34, got <v>`
- Warnings (acb, MCP acb):
  - `…: acb.adjustment item <n> names "sec-99", which is not a security in quarry's store; quarry skips it`
  - `…: acb.adjustment item <n> is for "<name>", which no non-registered account holds on <date>; quarry skips it`
  - `…: acb.adjustment items <a> and <b> are both for "sec-41" on <date>; quarry applies both`
- No masking needed. Security ids appear in `acb --json` and `securities.id`.

#### Finding `shares-without-cost`
- After `unclassified-account` in Types(). Entity itxn id; id `shares-without-cost:itxn-123`. Read-time; non-registered accounts only; add_shares shares > 0, NULL cost_basis, not a pool-internal move. Order date then id.
- Heading `Shares added with no cost`. GroupClause `enter what each one cost on its Add Shares transaction in Quicken, then run quarry sync; until then quarry acb counts those shares at no cost`.
- Fix/JSON `fix`: `Open this Add Shares transaction in Quicken and enter the shares' cost basis, then run quarry sync; until then quarry acb counts them at no cost, so its gains on this security are too high`
- Row: `  shares-without-cost:itxn-123  2016-03-01  Questrade Margin  XEQT  100 shares`
- JSON item: date, account_id, account, currency set; new keys appended to every FindingItem (null elsewhere): `investment_transaction_id`, `security_id`, `security`, `shares`. CSV: same four columns appended to the header end.
- `--type` literal gains it; Long row `shares-without-cost  shares added to a non-registered account with no cost basis, which quarry acb needs` + sentence: never marked fixed; leaves the list on the sync after the cost is entered.
- findings.md bullet: `` `shares-without-cost`: shares moved or added into a non-registered account with no cost basis in Quicken. Open the Add Shares transaction and enter the cost (from the old broker's statement); quarry acb counts them at no cost until then. ``

#### Store
- `investment_transactions.cost_basis DECIMAL(18,2)` NULL (ZCOSTBASIS, NULL when 0). FormatVersion 9.
- Conventions sentence: `cost_basis is the cost Quicken records for a buy, reinvested dividend or added shares (NULL when none). ACB and capital gains are in no table or view: quarry acb (MCP acb) computes them; never derive them in SQL.` → sql_conventions.go, hand copies, schema.md.

#### Changes to existing surfaces
| Where | New |
|---|---|
| SKILL description | drop `for gains or ACB beyond saying quarry does not cover them yet`; add `; adjusted cost base and realized capital gains per tax year (CAD)` after holdings |
| SKILL §4 | row `\| ACB, capital gains for a tax year \| quarry acb [--year <y>] [--security <s>] --json; see references/findings.md "Classifying accounts" first \|` |
| SKILL §6 / findings.md | writing config also allowed for `acb.adjustment` lines the user dictates from a T3 slip: show the lines, write after yes |
| SKILL §7 | delete the ACB bullet; new `**Tax:** quarry acb is a worksheet: say so, relay superficial-loss and incomplete warnings, never call a loss deductible or denied.` |
| SKILL trigger | `The first time ACB or gains are asked for, run quarry findings --type unclassified-account --json; if any, classify them (references/findings.md) before running quarry acb.` Classification question examples add `a US 401(k) or IRA` |
| SKILL §9, `mcp --help` Tools | add `acb` after `net_worth` |
| findings `--csv` help | `print one row per transaction, split, payee, category, account or investment transaction as CSV` (supersedes 4d's "…category or account…") |
| `--type` usage, findings Long table | add `shares-without-cost` |
| PRD CLI table | add `acb` row; ACB bullet: superficial loss marked in `acb`, not a finding (F1) |
| describe_schema | shares sql_conventions |
What dies: SKILL §7 ACB "not yet" line; description exclusion. FindingItem/CSV only gain fields.

#### Edge rows
| Input | Outcome |
|---|---|
| Account unclassified (any, even empty) | refusal, exit 1 |
| No non-registered trades | empty line, exit 0 |
| Sold out, re-bought later | ACB restarts from 0.00 (exact remainder removed) |
| Split recorded in 2 accounts | applied once |
| Reinvest NULL cost_basis | units at 0 cost, incomplete, warning 4 (no finding) |
| add_shares no cost | finding + incomplete + `unknown cost` on later sales |
| Pool-internal move | `moved` row, ACB unchanged |
| Remove to registered / unpaired | warning 5, pro-rata ACB out, no gain |
| Loss sale, re-buy in RRSP within 30 days, held | marked, warning 3 |
| Loss sale, all sold, none held at +30 | not marked |
| Gain sale with re-buy | not marked |
| USD trade before first rate / no rates | warning 6, incomplete, out of year totals |
| Sale Dec 24–31 | warning 9 |
| ROC > ACB | ACB 0.00, `return of capital` row gain = excess, warning 8 |
| Adjustment, no units held then | skipped, warning |
| Same ticker, 2 ids in pool | 2 rows, warning 7 |
| Closed non-registered account | trades in the pool |
| Fractional shares | exact rational units; ACB to the cent |
| Old store (format 8) | existing refusal |

#### Reference check (last scenario)
Exact on the real store: (a) ZCOSTBASIS = |amount| on every brokerage buy; (b) pool units per security on every event date = sum of holdingSpans over non-registered accounts; (c) no pool oversell; (d) first fx rate ≤ first USD pool trade; (e) securities with exactly one buy (or one cost-carrying lot) later sold out: quarry's ACB removed = Quicken's ZLOTMOD cost-basis change to the cent. List every other sale where quarry's ACB differs from ZLOTMOD with the reason (Quicken lot algorithm 1 vs CRA average; no cost; FX); unexplained difference → new scenario. Report counts: superficial-loss marks, shares-without-cost rows, unpaired removes. Manual (user): one year's per-sale proceeds and outlays vs the broker's T5008, or a past Schedule 3.

Note: Part B's findings `--csv` help line supersedes Part A's.

---

## Scenarios (Gherkin)

```gherkin
Feature: Registered-account classification and ACB

  Scenario: SCENARIO-01 Accounts show their classification
    Given a config whose accounts.registered and accounts.non-registered list some account ids
    When quarry accounts runs
    Then Status says registered for a listed registered account and unclassified for an investment account in neither list
    And --json carries registered true, false or null

  Scenario: SCENARIO-02 A malformed classification is refused
    Given a config whose accounts lists are malformed, or list one id in both
    When any command runs
    Then it refuses with the ruled line, masking account-number-like text, and exits 1

  Scenario: SCENARIO-03 An id that names no account is warned
    Given a config listing an id that is not an account in the store
    When quarry accounts or quarry findings runs
    Then it prints the ruled warning, masked, and exits 0

  Scenario: SCENARIO-04 Unclassified investment accounts are findings
    Given an investment account in neither list
    When quarry findings runs
    Then it lists unclassified-account:<id> with the ruled copy, and the row disappears once the id is added to a list, without a sync

  Scenario: SCENARIO-05 Finding counts include unclassified accounts
    Given an unclassified investment account
    When quarry status runs
    Then its findings count includes it, as sync, MCP data_quality and sync_status do

  Scenario: SCENARIO-06 Sync imports Quicken's cost basis
    Given investment transactions whose Quicken cost basis is set or zero
    When quarry sync runs
    Then investment_transactions.cost_basis holds it, NULL when zero, and the store format is 9

  Scenario: SCENARIO-07 ACB adjustments are read from config
    Given acb.adjustment items, valid and malformed
    When a command loads the config
    Then valid items load as decimal CAD amounts and a malformed item is refused with the ruled line

  Scenario: SCENARIO-08a ACB is pooled per security across non-registered accounts
    Given non-registered CAD and USD buys and sells with commissions across two accounts
    When the report computes ACB
    Then each sale's proceeds, outlays, ACB removed and gain, each year's totals and each security's ACB follow the ruled CRA rules

  Scenario: SCENARIO-08b ACB and gains per tax year
    Given non-registered CAD and USD buys and sells with commissions across two accounts
    When quarry acb runs
    Then it prints realized gains per tax year and today's ACB per security, pooled across the accounts

  Scenario: SCENARIO-09 Reinvested dividends and splits
    Given a reinvested dividend with a Quicken cost and a split recorded in two accounts
    When quarry acb runs
    Then the reinvest adds its cost and the split changes shares once, not ACB

  Scenario: SCENARIO-10 Shares moved between accounts
    Given shares moved between two non-registered accounts and shares removed with no matching add
    When quarry acb runs
    Then the move changes nothing and the unpaired removal takes out its share of ACB with the ruled warning

  Scenario: SCENARIO-11 Return of capital and reinvested distributions
    Given acb.adjustment items for return of capital and a reinvested distribution, one return of capital above the ACB
    When quarry acb runs
    Then they lower and raise ACB, and the excess return of capital is a gain with the ruled warning

  Scenario: SCENARIO-12 Possible superficial losses are marked
    Given a sale at a loss and the same security bought within 30 days in a registered account and still held at day 30
    When quarry acb runs
    Then the sale and its year are marked and the ruled warning prints, with the loss unadjusted

  Scenario: SCENARIO-13a Shares added with no cost leave ACB incomplete
    Given shares added to a non-registered account with no cost basis
    When quarry acb runs
    Then the security is incomplete and its later sales say unknown cost

  Scenario: SCENARIO-13b Shares added with no cost are findings
    Given shares added to a non-registered account with no cost basis
    When quarry findings runs
    Then it lists shares-without-cost:<itxn id> with the ruled copy, as status, sync and MCP counts do

  Scenario: SCENARIO-14 ACB data-quality warnings
    Given a USD trade before the first exchange rate, two securities sharing a ticker, and a sale dated December 28
    When quarry acb runs
    Then each prints its ruled warning and the incomplete security is left out of the year totals

  Scenario: SCENARIO-15 Sales of one tax year
    Given sales in several years
    When quarry acb runs with --year 2024
    Then it lists 2024's sales one by one with a total

  Scenario: SCENARIO-16 One security's history
    Given a security with buys, sells and an adjustment
    When quarry acb runs with --security and its ticker
    Then it prints every event with shares held, ACB and gain

  Scenario: SCENARIO-17 ACB refuses what it cannot answer
    Given an unclassified investment account, or --currency USD, a bad or future --year, or an unknown --security
    When quarry acb runs
    Then it prints the ruled refusal with its exit code

  Scenario: SCENARIO-18 Nothing to show
    Given no non-registered account has traded
    When quarry acb runs
    Then it prints the empty-result warning and exits 0

  Scenario: SCENARIO-19 MCP acb
    Given the MCP server over a synced store
    When a client calls acb
    Then it gets the same document as quarry acb --json, capped at 500 events

  Scenario: SCENARIO-20 Docs carry the new surface
    Given SKILL.md, references/findings.md, schema.md, the SQL conventions and the PRD
    When the doc drift tests run
    Then they carry the ruled copy, including the classify-first trigger

  Scenario: SCENARIO-21 Reference check on the real Quicken file
    Given copies of the user's snapshot and store in a scratch HOME with the accounts classified
    When quarry sync and quarry acb run
    Then the reference assertions hold and every ACB difference from Quicken's lots has a stated reason
```

---

## Sizing

Architect sizing pass, 2026-10-05. S08 and S13 splits approved by the user 2026-10-05. Package counting as 4c: `internal/finding`, `store/duckstore` (adapter behind report's port) and a single `snapshot` option argument don't count as a second feature package. Twin cost (4c): unit p90 ≈ 2.9M IE; a new command ≈ 2.6M.

| Scenario | Verdict — numbers |
| --- | --- |
| SCENARIO-01 | OWNS A RUN (sonnet), 3 batches, config + report: `[accounts]` lists + knownKeys; accounts Status words, JSON `registered`, Long paragraph; owns the schema.md "registered is not in the store" sentence (+ describe_schema if shared). |
| SCENARIO-02 | OWNS A RUN (sonnet), 3 batches, config + new `internal/platform` mask helper (behaviour classes pinned in its package); owns every config-time masking site (refusals, "as item", "in both", unknown-key warning under `[accounts]`, go-toml syntax-error probe parse.go:104). |
| SCENARIO-03 | FOLD into 05 (one warning composer at accounts, findings, data_quality); acceptance test rides in 05. |
| SCENARIO-04 | OWNS A RUN (opus), 4 batches, finding + report: type + read-time detector + accounts on the `Findings` port result; JSON `first_found_at` null, CSV, `--csv` help (Part A form); text, Long row + after-table paragraph + TOML, `--type`, findings.md bullet + first line; fixture strategy / golden re-pins (every fixture without `[accounts]` raises the finding from here on). |
| SCENARIO-05 | OWNS A RUN (opus), 4 batches, report + snapshot option + mcp; absorbs 03: status (accounts on the `Status` result + `CountFindings`), sync count via a snapshot option wired at cli/sync.go:102, MCP data_quality + sync_status, S03's warning. Builds the one read-time detector chokepoint 13b extends. Orchestrator 2026-10-05: S04 already wires MCP data_quality's listing (it calls `Server.Findings`); 05 adds the counts and the rest. |
| SCENARIO-06 | OWNS A RUN (sonnet), 3 batches, importer + store. **Runs probes P1/P2 first** (results → Handoff → STATE). Column + field + duckstore read/write; importer reads ZCOSTBASIS (NULL when 0); FormatVersion 9 + re-pins; conventions sentence 1 (cost_basis) + hand copies + schema.md. |
| SCENARIO-07 | OWNS A RUN (sonnet), 3 batches, config: `[[acb.adjustment]]` (new array-table parse; `platform/money` from raw token text) + knownKeys; 7 refusal lines with amount bounds. Adjustment warnings go to 11. |
| SCENARIO-08a | OWNS A RUN (opus), 4 batches, report + duckstore; absorbs 09. Port read in an embedded interface (report.Store at its 10-method cap); walk (buy = −amount, pro-rata to the cent, exact remainder, commission arm per P1, tax year, same-day order per P2); BoC rate per event; reinvest/split arms; sold-out-and-rebought, closed account, fractional shares. Read covers every investment account incl. registered and file-wide holdings (12 needs them). Acceptance at `Server.ACB`. |
| SCENARIO-08b | OWNS A RUN (opus), 4 batches, document + cli: full `--json`; text default; Short/Long/Example/all flags (`--currency` defined, refused in 17); root registration + all-commands tables; config warnings (warning 1). Docs: whole SKILL description line (4d fragment + 4e edits), SKILL §7 delete + Tax line, PRD CLI row + ACB bullet. Fixtures classify every investment account from here on. |
| SCENARIO-09 | FOLD into 08a (acceptance at `Server.ACB`; Then unchanged). |
| SCENARIO-10 | OWNS A RUN (sonnet), 3 batches, report: pool-internal pairing + `moved` row; unpaired remove + warning 5; add_shares with cost. If P2 finds 0 pairs → move arm dropped, Given unsatisfiable → back to the user; then LIGHT. |
| SCENARIO-11 | OWNS A RUN (sonnet), 3 batches, report + cli wiring of cfg adjustments: ROC/RD events; ROC above ACB + warning 8; the 3 adjustment warnings. |
| SCENARIO-12 | OWNS A RUN (opus), 3 batches, report: ±30-day bounds both sides; held at day +30; same ticker; "other than the shares sold"; registered accounts included; year suffix; warning 3. |
| SCENARIO-13a | LIGHT, report: warning 4, `N sale(s) of shares with unknown cost` year suffix, `incomplete` suffix (3 ruled lines); NULL-cost reinvest arm. Becomes OWNS A RUN if warning 4's copy ruling adds a variant. |
| SCENARIO-13b | OWNS A RUN (opus), 4 batches, finding + report: `shares-without-cost` detector (excludes 10's pairs) through 05's chokepoint (status/sync/MCP counts agree, R-1); 4 FindingItem/CSV keys; **final** `--csv` help (Part B, moves 04's pin); text, Long row + sentence, `--type`, findings.md bullet. Also updates the `--csv` mirror sentence at `plugin/skills/quarry/references/findings.md:42` to the Part B `--csv` wording (orchestrator 2026-10-05). |
| SCENARIO-14 | OWNS A RUN (sonnet), 3 batches, report: warning 6 (both variants) + out of totals; warning 7; warning 9 (bounds Dec 23/24/31, Jan 1); relative order of warnings 3–9. |
| SCENARIO-15 | OWNS A RUN (sonnet), 3 batches; absorbs 18: `--year` caption/columns/Total/suffixes, JSON year filter; both empty-warning forms; full 1–9 warning order. |
| SCENARIO-16 | OWNS A RUN (sonnet), 2–3 batches: selector (name, ticker, id; repeated); history renderer; registered-only warning (owned here). |
| SCENARIO-17 | OWNS A RUN (sonnet), 4 batches; absorbs 20: refusals R-6 (ignoring doesn't unblock), `--currency`, `--year` bad/future (clock seam), unknown `--security`; docs: findings.md "Classifying accounts", SKILL §6 + "Ignoring a finding" mirror (4d sentence + acb.adjustment allowance), SKILL trigger + 401(k)/IRA examples, SKILL §4 row. |
| SCENARIO-18 | FOLD into 15. |
| SCENARIO-19 | OWNS A RUN (sonnet), 3 batches, mcp (5–6 ruled lines > light-lane limit; 4c S17 precedent). Warnings from the full report before the cut. Owns SKILL §9 + `mcp --help` Tools line, conventions sentence 2 + hand copies + schema.md. |
| SCENARIO-20 | FOLD into 17 (acceptance = the classify-first trigger drift pin); other doc lines go to their owners. |
| SCENARIO-21 | No architect/developer: orchestrator reference check; a rule gap becomes a new scenario built before the gate. |

**Copy ownership (Changes to existing surfaces):** Part A — findings Long paragraph + TOML → 04; `--csv` help → 04 (Part A), 13b writes the final Part B form; findings.md first line → 04; SKILL frontmatter fragment → 08b (one owner for the description line); SKILL §6 + findings.md mirror → 17; "Classifying accounts" → 17 (04's bullet reference dangles until 17, by design); schema.md registered sentence → 01. Part B — SKILL description/§7/PRD → 08b; SKILL §4, §6 acb.adjustment allowance, trigger + examples → 17; SKILL §9 + `mcp --help` → 19; `--type` + Long row for shares-without-cost → 13b; conventions sentence 1 → 06, sentence 2 → 19.

**Mid-feature copy rulings expected:** before 13a (warning 4 says findings lists them, but a NULL-cost reinvest raises no finding); before 16 (registered-only warning has no slot in the order 1–9); before 08a only if P2 finds an inverted same-day buy/sell, or P1 is mixed/inconclusive.

**Traps:** report.Store at its 10-method interfacebloat cap → new reads in an embedded interface or on existing Findings/Status results; `snapshot` cannot import `report` → read-time count injected as a snapshot option; golden blast radius (52 test files use investment fixtures; 12 cmd count pins) → 04 decides fixture strategy; 08a's read covers registered accounts too; acb fixtures classify every investment account from 08b on; adjustment amounts via `internal/platform/money` (config must not import report).

## BDD Acceptance Progress

- [x] SCENARIO-01: Accounts show their classification — `internal/cli/accounts_classification_test.go` `Test_accounts_show_their_classification`
- [x] SCENARIO-02: A malformed classification is refused — `cmd/quarry/run_config_test.go` `Test_run_refuses_an_account_number_in_an_accounts_list_masked`
- [x] SCENARIO-04: Unclassified investment accounts are findings — `cmd/quarry/run_findings_unclassified_test.go` `Test_run_findings_lists_an_unclassified_account_until_the_config_classifies_it`
- [x] SCENARIO-05: Finding counts include unclassified accounts — `cmd/quarry/run_status_unclassified_test.go` `Test_run_status_counts_an_unclassified_account_until_the_config_classifies_it`
- [x] SCENARIO-03: An id that names no account is warned — delivered by SCENARIO-05 — `cmd/quarry/run_accounts_unmatched_test.go` `Test_run_accounts_and_findings_warn_a_listed_id_that_names_no_account`
- [ ] SCENARIO-06: Sync imports Quicken's cost basis
- [ ] SCENARIO-07: ACB adjustments are read from config
- [ ] SCENARIO-08a: ACB is pooled per security across non-registered accounts
- [ ] SCENARIO-09: Reinvested dividends and splits
- [ ] SCENARIO-08b: ACB and gains per tax year
- [ ] SCENARIO-10: Shares moved between accounts
- [ ] SCENARIO-11: Return of capital and reinvested distributions
- [ ] SCENARIO-12: Possible superficial losses are marked
- [ ] SCENARIO-13a: Shares added with no cost leave ACB incomplete
- [ ] SCENARIO-13b: Shares added with no cost are findings
- [ ] SCENARIO-14: ACB data-quality warnings
- [ ] SCENARIO-15: Sales of one tax year
- [ ] SCENARIO-18: Nothing to show
- [ ] SCENARIO-16: One security's history
- [ ] SCENARIO-17: ACB refuses what it cannot answer
- [ ] SCENARIO-20: Docs carry the new surface
- [ ] SCENARIO-19: MCP acb

## Reference check

SCENARIO-21 is run by the orchestrator after SCENARIO-19 and before the gate, on a scratch HOME holding copies of the newest snapshot and the store, with the accounts classified. Results go in `REFERENCE-CHECK.md`.

- [ ] SCENARIO-21: Reference check on the real Quicken file
