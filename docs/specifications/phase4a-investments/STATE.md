# phase4a-investments — current state

Scenarios complete: SCENARIO-01..02. Last updated by SCENARIO-02.

## Binding decisions
- Optional entities (`Security`, `SecurityQuote`, `Position`) live in one slice in `internal/importer/entities.go` with generated placeholders; missing → that data is empty, never a refusal. S04 appends `Lot` (SCENARIO-01, 02)
- Quote selection: deleted, NULL-date, NULL-price quotes and quotes of a non-imported security are dropped BEFORE the highest-`Z_PK` dedupe per (security, UTC day); every remaining quote is parsed, so an unreadable superseded duplicate still refuses. `prices` PK `(security_id, date)` gives 4b readers one row per day (SCENARIO-01)
- `store.Price.Price` and investment shares/split sides are int64 millionths, DECIMAL(18,6), rounded/checked on decimal text (`big.Rat`, never float); "too large" = 10^12 bound. Shares snap residue within 1e-9 to exact, beyond it refuse; sign kept. S04's gate tolerance 0.000001 assumes exact stored shares (SCENARIO-01, 02)
- `mapPositions` returns position Z_PK → (account, security) for non-deleted positions of the Position entity in imported accounts; S04 sums lots over this same map, so a position in a skipped account never forms a holding (SCENARIO-02)
- Investment rows: date posted-else-entered (reverse of cash; S04 orders a holding's walk by it); account skip precedes every refusal; undated precedes every other refusal, so no "no date" variants of the share/amount copy exist (SCENARIO-02)
- `split_new_shares`/`split_old_shares` set only on `action = 'split'`; S04's split multiply reads them (SCENARIO-02)
- `securities.currency` stored as recorded: NULL only when NULL, `""` stays `""` (SCENARIO-01)
- `securities_rows`/`prices_rows`/`investment_transactions_rows` are nullable BIGINT after `rates_fetch_error`, filled positionally by `importRunRows`, carried via `optionalRunColumns`/`carriedRun`, read with COALESCE→0 in `status.go`. S04 appends its own the same way. `FormatVersion = 6` stays (SCENARIO-01, 02)
- `Builder.InvestmentTransaction` is the only way tests add investment rows; `b.Transaction` makes a cash row (SCENARIO-02)
- A quote or position whose security is deleted/absent is skipped silently (I4-6 analogue); a non-zero-share investment row with no resolvable security refuses (placeholder, see unbuilt) (SCENARIO-01, 02)

## Left unbuilt
- Ruled copy and offender class for split ratio (NULL, 0, unreadable) and shares-without-security: today plain errors `errUnreadableSplitRatio` / `errSharesWithoutSecurity` (`internal/importer/investments.go`). S03 turns them into offenders and repoints `Test_import_fails_on_a_split_with_an_unreadable_ratio` and `Test_import_fails_on_shares_without_a_security` (no twins); it also rules whether a non-number/beyond-scale split side uses the ratio copy — SCENARIO-03
- `reasonSecurityNoName` and the NULL/empty-name refusal (decides whether `""` is "no name") — SCENARIO-03
- `ZLOT`/`Lot` optional entity, `document.Rows`/`NewRows` keys, Rows investment clause, `--json` `store.rows` keys, Shares line — SCENARIO-04
- Dropping `investment_transactions_not_imported` from `requiredRunColumns`, `surveyTransactions`/`store.NotImported` — SCENARIO-09

## Traps
- ZNAME is scanned into a plain `string`: a NULL name fails the import loudly via `rows.Scan` until S03 adds the refusal; `sql.NullString` earlier makes a nameless security import silently (SCENARIO-01)
- `ZQUOTEDATE` and investment dates must be `CAST(... AS REAL)` or the driver returns a Unix-epoch `time.Time` (SCENARIO-01, 02)
- A v9fixture row without its default `Z_ENT` is silently filtered by `Z_ENT = ?`; `TransactionRow.Entity` defaults to CashFlowTransaction (SCENARIO-01, 02)
- `not_imported` still counts investment rows that now also import — expected until S09; do not "fix" (SCENARIO-02)
- Fault-table row `ZTRANSACTION` matches `ZPOSTEDDATE`, which the investment query also contains — correct only while the cash query runs first; new rows match `ZUNITS` / `FROM ZPOSITION` (SCENARIO-02)
- Plan's case-sensitive narrow `-run 'Investment|…'` misses `run_sync_imports_investment_*`; use `(?i)` (SCENARIO-02)
- `schema.md` regenerates with `go test ./cmd/quarry/ -run Test_skill_schema_reference_matches_the_committed_file -update` (pattern `SchemaReference` matches nothing) (SCENARIO-01)
- Half-even on `float64` passes most rows and fails ties; round on the decimal text (SCENARIO-01)

## Open debts
- Offender classes chosen by S02 without a spec ruling: shares/amount/commission precision reuse `classTransactionPrecision`, too large `classTooLarge`, not a number `classNotANumber`, NULL amount `classMissingValue` (`internal/importer/offenders.go`, `reasons.go` `reasonInvestment*`). S03 may re-rule (SCENARIO-02)
- No Import-level test pins the `<v>` text in the share/amount refusal copy for negative or exponent values; only `parseShares` unit tests cover those forms. S03 adds if it touches the copy, else unowned — dies unless re-opened (SCENARIO-02)
- Spec S.7 conventions text says prices are in `securities.currency` "NULL when Quicken records none", but the importer stores `""` as `""`. Open question for the final product-vision pass (SCENARIO-01)
- Every new `Z_ENT = ?` query (lots in S04) needs an another-entity pin plus positive control, mutation-checked; securities, quotes, positions, investment transactions are done (SCENARIO-01, 02)
