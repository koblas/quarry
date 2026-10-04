package cli

import (
	"fmt"
	"math"
	"strings"

	"github.com/koblas/quarry/internal/finding"
	"github.com/koblas/quarry/internal/platform/homepath"
	"github.com/koblas/quarry/internal/platform/humanize"
	"github.com/koblas/quarry/internal/platform/sqlschema"
	"github.com/koblas/quarry/internal/report/document"
	"github.com/koblas/quarry/internal/snapshot"
	"github.com/koblas/quarry/internal/store"
)

// formatMB renders bytes as decimal megabytes with one decimal place and
// thousands-grouped whole MB.
func formatMB(bytes int64) string {
	tenths := int64(math.Round(float64(bytes) / 100000))
	return fmt.Sprintf("%s.%d MB", humanize.Thousands(int(tenths/10)), tenths%10)
}

// accountsPhrase renders n as "1 account" or "N accounts", thousands-grouped.
func accountsPhrase(n int) string {
	return humanize.Count(n, "account", "accounts")
}

// renderSuccess renders m's result block, then the diff rows a mismatch or
// extras-only Schema line summarizes.
func renderSuccess(m snapshot.Manifest, home string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%-10s%s\n", "Snapshot", homepath.Abbreviate(home, m.Snapshot.Path))
	fmt.Fprintf(&b, "%-10s%s\n", "Manifest", homepath.Abbreviate(home, m.Snapshot.Manifest))
	fmt.Fprintf(&b, "%-10s%s\n", "Source", homepath.Abbreviate(home, m.Snapshot.Source))
	fmt.Fprintf(&b, "%-10s%s, %s\n", "Size", formatMB(m.Snapshot.Bytes), accountsPhrase(m.Snapshot.Accounts))
	fmt.Fprintf(&b, "%-10s%s\n", "SHA-256", m.Snapshot.SHA256)
	fmt.Fprintf(&b, "%-10s%s\n", "Schema", schemaLine(m.Schema))
	writeDiffRows(&b, m.Schema)
	return b.String()
}

// schemaLine renders the Schema line: exact match, extras-only, or a
// DIFFERS line for a mismatch.
func schemaLine(s snapshot.SchemaInfo) string {
	if !s.Verified {
		line := fmt.Sprintf("DIFFERS from reference %s: %s missing",
			s.Reference, sqlschema.CountPhrase(len(s.MissingTables), len(s.MissingColumns)))
		if s.HasExtras() {
			line += fmt.Sprintf(", %s not in reference",
				sqlschema.CountPhrase(len(s.UnexpectedTables), len(s.UnexpectedColumns)))
		}
		return line
	}

	line := fmt.Sprintf("matches reference %s (%s tables, %s columns)",
		s.Reference, humanize.Thousands(s.ReferenceTables), humanize.Thousands(s.ReferenceColumns))
	if s.HasExtras() {
		line += fmt.Sprintf(", plus %s not in it",
			sqlschema.CountPhrase(len(s.UnexpectedTables), len(s.UnexpectedColumns)))
	}
	return line
}

// writeDiffRows appends one row per s's diff entries: missing before
// unexpected, tables before columns within each.
func writeDiffRows(b *strings.Builder, s snapshot.SchemaInfo) {
	for _, table := range s.MissingTables {
		writeDiffRow(b, "-", "table", table)
	}
	for _, col := range s.MissingColumns {
		writeDiffRow(b, "-", "column", col.Table+"."+col.Column)
	}
	for _, table := range s.UnexpectedTables {
		writeDiffRow(b, "+", "table", table)
	}
	for _, col := range s.UnexpectedColumns {
		writeDiffRow(b, "+", "column", col.Table+"."+col.Column)
	}
}

// writeDiffRow appends one diff row: two leading spaces, sign, a
// space-padded 8-wide label, then value.
func writeDiffRow(b *strings.Builder, sign, label, value string) {
	fmt.Fprintf(b, "  %s %-8s%s\n", sign, label, value)
}

// renderStore renders result's Store, Rows, Balances, Splits, Shares, Transfers and
// Findings and Rates lines, appended after renderSuccess's block once a build was reached.
func renderStore(result store.Result, home string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%-10s%s\n", "Store", homepath.Abbreviate(home, result.Path))
	fmt.Fprintf(&b, "%-10s%s\n", "Rows", rowsPhrase(result.Counts, result.NotImported))
	bc := result.Validation.Balances
	fmt.Fprintf(&b, "%-10s%s\n", "Balances", balancesPhrase(balanceCounts{Checked: bc.Checked, NeverReconciled: len(bc.NeverReconciled), InvestmentAccounts: bc.InvestmentAccounts}))
	fmt.Fprintf(&b, "%-10s%s\n", "Splits", splitsPhrase(result.Validation.Splits.Checked))
	fmt.Fprintf(&b, "%-10s%s\n", "Shares", sharesPhrase(result.Validation.Shares.Checked))
	writeTransfersLine(&b, result.Validation.Transfers)
	fmt.Fprintf(&b, "%-10s%s\n", "Findings", findingsPhrase(result.Findings, result.FindingsCarried))
	fmt.Fprintf(&b, "%-10s%s\n", "Rates", ratesPhrase(result.Rates, result.Counts.Transactions))
	return b.String()
}

// ratesPhrase renders the rates a build stored: the span with "(N new)", "(up to date)" or a failed-fetch clause, or why there are none.
// transactions tells an empty store apart from one the Bank of Canada has no rates for.
func ratesPhrase(rates store.RatesSummary, transactions int) string {
	if rates.First.IsZero() {
		return noRatesPhrase(rates.FetchError != "", transactions)
	}
	span := "USD/CAD " + rates.First.Format(document.DateLayout) + " to " + rates.Last.Format(document.DateLayout)
	switch {
	case rates.Partial:
		return span + " (" + humanize.Thousands(rates.Added) + " new, not all fetched; see warning)"
	case rates.FetchError != "":
		return span + " (not refreshed; see warning)"
	case rates.Added > 0:
		return span + " (" + humanize.Thousands(rates.Added) + " new)"
	default:
		return span + " (up to date)"
	}
}

// noRatesPhrase renders the Rates line of a store with no rates.
func noRatesPhrase(fetchFailed bool, transactions int) string {
	switch {
	case fetchFailed:
		return "none (not fetched; see warning)"
	case transactions == 0:
		return "none (no transactions to convert)"
	default:
		return "none (the Bank of Canada has no rates for your transaction dates)"
	}
}

// findingsPhrase renders the open count, with "(M new)" once history was carried and
// "K fixed since the last sync" and "J ignored"; "run quarry findings" follows only while any are open.
func findingsPhrase(c finding.Counts, carried bool) string {
	if c.Open == 0 {
		return "none open" + fixedClause(c.NewlyFixed) + ignoredClause(c.Ignored)
	}
	phrase := humanize.Thousands(c.Open) + " open"
	if carried && c.New > 0 {
		phrase += " (" + humanize.Thousands(c.New) + " new)"
	}
	return phrase + fixedClause(c.NewlyFixed) + ignoredClause(c.Ignored) + "; run quarry findings to list them"
}

// ignoredClause renders ", J ignored", empty when none are ignored.
func ignoredClause(ignored int) string {
	if ignored == 0 {
		return ""
	}
	return ", " + humanize.Thousands(ignored) + " ignored"
}

// fixedClause renders ", K fixed since the last sync", empty when none were fixed.
func fixedClause(newlyFixed int) string {
	if newlyFixed == 0 {
		return ""
	}
	return ", " + humanize.Thousands(newlyFixed) + " fixed since the last sync"
}

// writeTransfersLine appends tc's Transfers count line.
func writeTransfersLine(b *strings.Builder, tc store.TransferCheck) {
	fmt.Fprintf(b, "%-10s%s\n", "Transfers", transfersPhrase(tc.Paired, len(tc.OneSided)))
}

// writeOneSidedRows appends one "?" row per one-sided leg.
func writeOneSidedRows(b *strings.Builder, legs []store.OneSidedTransfer) {
	for _, row := range oneSidedRows(legs) {
		fmt.Fprintln(b, row)
	}
}

// transfersPhrase renders the paired and one-sided transfer counts as
// "none", "N paired", or "N paired, M one-sided" once any leg is one-sided.
func transfersPhrase(paired, oneSided int) string {
	switch {
	case paired == 0 && oneSided == 0:
		return "none"
	case oneSided == 0:
		return humanize.Thousands(paired) + " paired"
	default:
		return fmt.Sprintf("%s paired, %s one-sided", humanize.Thousands(paired), humanize.Thousands(oneSided))
	}
}

// balancesCheckedPhrase renders the checked clause: no accounts checked,
// exactly one matching, or the plural count.
func balancesCheckedPhrase(n int) string {
	switch n {
	case 0:
		return "no accounts to check"
	case 1:
		return "1 account matches Quicken's last reconciled balance"
	default:
		return humanize.Thousands(n) + " accounts match Quicken's last reconciled balance"
	}
}

// balanceCounts is the balance gate's account counts, named so they cannot be swapped.
type balanceCounts struct {
	Checked, NeverReconciled, InvestmentAccounts int
}

// balancesPhrase appends the never-reconciled and investment-account
// clauses (each omitted at zero, joined with " and ") to the checked clause.
func balancesPhrase(c balanceCounts) string {
	return balancesCheckedPhrase(c.Checked) + balancesExtrasPhrase(c)
}

// balancesExtrasPhrase renders the never-reconciled and investment-account
// clauses, each omitted at zero and joined with " and ", prefixed with a
// "; " separator when any exist.
func balancesExtrasPhrase(c balanceCounts) string {
	var extras []string
	if c.NeverReconciled > 0 {
		extras = append(extras, humanize.Count(c.NeverReconciled, "never reconciled", "never reconciled"))
	}
	if c.InvestmentAccounts > 0 {
		extras = append(extras, humanize.Count(c.InvestmentAccounts, "investment account not checked", "investment accounts not checked"))
	}
	if len(extras) == 0 {
		return ""
	}
	return "; " + strings.Join(extras, " and ")
}

// xOfYPhrase renders "X of Y <noun>", the noun agreeing with y.
func xOfYPhrase(x, y int, singular, plural string) string {
	noun := plural
	if y == 1 {
		noun = singular
	}
	return fmt.Sprintf("%s of %s %s", humanize.Thousands(x), humanize.Thousands(y), noun)
}

// balancesDifferPhrase renders bc's V1 clause: how many of the checked
// accounts differ, plus its never-reconciled and investment-account extras.
func balancesDifferPhrase(bc store.BalanceCheck) string {
	return "DIFFER for " + xOfYPhrase(len(bc.Mismatched), bc.Checked, "account", "accounts") +
		balancesExtrasPhrase(balanceCounts{NeverReconciled: len(bc.NeverReconciled), InvestmentAccounts: bc.InvestmentAccounts})
}

// splitsDifferPhrase renders sc's V1 clause: how many of the checked
// transactions differ.
func splitsDifferPhrase(sc store.SplitCheck) string {
	return "DIFFER for " + xOfYPhrase(len(sc.Mismatched), sc.Checked, "transaction", "transactions")
}

// sharesDifferPhrase renders sc's clause: how many of the checked holdings differ.
func sharesDifferPhrase(sc store.ShareCheck) string {
	return "DIFFER for " + xOfYPhrase(len(sc.Mismatched), sc.Checked, "holding", "holdings")
}

// splitsPhrase renders the checked clause: no transactions checked,
// exactly one equalling its splits, or the plural count.
func splitsPhrase(checked int) string {
	switch checked {
	case 0:
		return "no transactions to check"
	case 1:
		return "the 1 transaction equals the sum of its splits"
	default:
		return "all " + humanize.Thousands(checked) + " transactions equal the sum of their splits"
	}
}

// rowsPhrase renders c as the cash clause, then the investment clause, then n's
// investment-transaction clause when nonzero.
func rowsPhrase(c store.Counts, n store.NotImported) string {
	phrase := strings.Join([]string{
		humanize.Count(c.Transactions, "transaction", "transactions"),
		humanize.Count(c.Splits, "split", "splits"),
		humanize.Count(c.Transfers, "transfer", "transfers"),
		humanize.Count(c.Payees, "payee", "payees"),
		humanize.Count(c.Categories, "category", "categories"),
		humanize.Count(c.Tags, "tag", "tags"),
	}, ", ")
	phrase += "; " + strings.Join([]string{
		humanize.Count(c.InvestmentTransactions, "investment transaction", "investment transactions"),
		humanize.Count(c.Securities, "security", "securities"),
		humanize.Count(c.Prices, "price", "prices"),
	}, ", ")
	if n.InvestmentTransactions > 0 {
		phrase += "; " + humanize.Count(n.InvestmentTransactions, "investment transaction", "investment transactions") + " not imported"
	}
	return phrase
}

// sharesPhrase renders the Shares line of a build whose share counts all matched Quicken's.
func sharesPhrase(checked int) string {
	switch checked {
	case 0:
		return "no holdings to check"
	case 1:
		return "1 holding matches Quicken's share count"
	default:
		return humanize.Thousands(checked) + " holdings match Quicken's share counts"
	}
}

// formatMoney renders cents as a thousands-grouped, 2-decimal amount with a
// leading "-" for a negative value.
func formatMoney(cents int64) string {
	negative := cents < 0
	if negative {
		cents = -cents
	}
	s := fmt.Sprintf("%s.%02d", humanize.Thousands(int(cents/100)), cents%100)
	if negative {
		return "-" + s
	}
	return s
}

// formatShares renders millionths of a share thousands-grouped with trailing
// fractional zeros trimmed, and a leading "-" for a negative count.
func formatShares(millionths int64) string {
	const perShare = 1_000_000
	// Split before negating: the whole and fraction parts of math.MinInt64 fit, its magnitude does not.
	whole, frac := millionths/perShare, millionths%perShare
	negative := millionths < 0
	if negative {
		whole, frac = -whole, -frac
	}
	s := humanize.Thousands(int(whole))
	if frac != 0 {
		s += "." + strings.TrimRight(fmt.Sprintf("%06d", frac), "0")
	}
	if negative {
		return "-" + s
	}
	return s
}

// accountLabel renders "Name (CUR[, closed][, inactive])": inactive is
// shown only when the account is open (not closed) and not active.
func accountLabel(name, currency string, closed, active bool) string {
	suffix := ""
	switch {
	case closed:
		suffix = ", closed"
	case !active:
		suffix = ", inactive"
	}
	return fmt.Sprintf("%s (%s%s)", escapeCell(name), currency, suffix)
}

// securityLabel renders "Name (TICKER)": the ticker is shown only when
// present and different from the name.
func securityLabel(name string, ticker *string) string {
	if ticker == nil || *ticker == name {
		return escapeCell(name)
	}
	return fmt.Sprintf("%s (%s)", escapeCell(name), escapeCell(*ticker))
}

// shareMismatchRows renders one "!" row per mismatch, in the order given:
// account and security label columns padded to the block's widest value,
// share counts and difference right-aligned to their own column's widest.
func shareMismatchRows(mismatches []store.ShareMismatch) []string {
	accounts := make([]string, len(mismatches))
	securities := make([]string, len(mismatches))
	quarry := make([]string, len(mismatches))
	quicken := make([]string, len(mismatches))
	diff := make([]string, len(mismatches))
	for i, m := range mismatches {
		accounts[i] = accountLabel(m.Account, m.Currency, m.Closed, m.Active)
		securities[i] = securityLabel(m.Security, m.Ticker)
		quarry[i] = formatShares(m.Quarry)
		quicken[i] = formatShares(m.Quicken)
		diff[i] = formatShares(m.Difference)
	}
	accountWidth, securityWidth := widestLen(accounts), widestLen(securities)
	quarryWidth, quickenWidth, diffWidth := widestLen(quarry), widestLen(quicken), widestLen(diff)

	rows := make([]string, len(mismatches))
	for i := range mismatches {
		rows[i] = fmt.Sprintf("  ! %-*s  %-*s  quarry %*s  Quicken %*s  difference %*s",
			accountWidth, accounts[i], securityWidth, securities[i],
			quarryWidth, quarry[i], quickenWidth, quicken[i], diffWidth, diff[i])
	}
	return rows
}

// balanceMismatchRows renders one "!" row per mismatch, in the order
// given: account label and date columns padded to the block's widest
// value, quarry/Quicken/difference amounts right-aligned to their own
// column's widest value.
func balanceMismatchRows(mismatches []store.BalanceMismatch) []string {
	labels := make([]string, len(mismatches))
	quarry := make([]string, len(mismatches))
	quicken := make([]string, len(mismatches))
	diff := make([]string, len(mismatches))
	for i, m := range mismatches {
		labels[i] = accountLabel(m.Name, m.Currency, m.Closed, m.Active)
		quarry[i] = formatMoney(m.Quarry)
		quicken[i] = formatMoney(m.Quicken)
		diff[i] = formatMoney(m.Difference)
	}
	labelWidth := widestLen(labels) + 2
	quarryWidth, quickenWidth, diffWidth := widestLen(quarry), widestLen(quicken), widestLen(diff)

	rows := make([]string, len(mismatches))
	for i, m := range mismatches {
		rows[i] = fmt.Sprintf("  ! %-*s%s  quarry %*s  Quicken %*s  difference %*s",
			labelWidth, labels[i], m.StatementDate.Format("2006-01-02"),
			quarryWidth, quarry[i], quickenWidth, quicken[i], diffWidth, diff[i])
	}
	return rows
}

// splitMismatchRows renders one "!" row per mismatch, in the order given:
// date fixed, account label and payee columns padded to the block's widest
// value (empty payee rendered as "(no payee)"), amount/splits totals
// right-aligned to their own column's widest value.
func splitMismatchRows(mismatches []store.SplitMismatch) []string {
	labels := make([]string, len(mismatches))
	payees := make([]string, len(mismatches))
	amounts := make([]string, len(mismatches))
	totals := make([]string, len(mismatches))
	for i, m := range mismatches {
		labels[i] = accountLabel(m.Account, m.Currency, m.Closed, m.Active)
		payees[i] = payeeLabel(m.Payee)
		amounts[i] = formatMoney(m.Amount)
		totals[i] = formatMoney(m.SplitsTotal)
	}
	labelWidth := widestLen(labels) + 2
	payeeWidth := widestLen(payees) + 2
	amountWidth, totalWidth := widestLen(amounts), widestLen(totals)

	rows := make([]string, len(mismatches))
	for i, m := range mismatches {
		rows[i] = fmt.Sprintf("  ! %s  %-*s%-*samount %*s  splits %*s",
			m.Date.Format("2006-01-02"), labelWidth, labels[i], payeeWidth, payees[i],
			amountWidth, amounts[i], totalWidth, totals[i])
	}
	return rows
}

// payeeLabel renders payee, or "(no payee)" when it is empty.
func payeeLabel(payee string) string {
	if payee == "" {
		return "(no payee)"
	}
	return escapeCell(payee)
}

// oneSidedRows renders one "?" row per one-sided leg, in the order given, each
// behind the lead "  ? ".
func oneSidedRows(legs []store.OneSidedTransfer) []string {
	leads := make([]string, len(legs))
	for i := range leads {
		leads[i] = "  ? "
	}
	return legRows(legs, leads)
}

// legRows renders one row per one-sided leg behind its own lead: date, padded
// label and payee columns, the amount right-aligned, then the other account.
func legRows(legs []store.OneSidedTransfer, leads []string) []string {
	labels := make([]string, len(legs))
	payees := make([]string, len(legs))
	amounts := make([]string, len(legs))
	for i, leg := range legs {
		labels[i] = accountLabel(leg.Account, leg.Currency, leg.Closed, leg.Active)
		payees[i] = payeeLabel(leg.Payee)
		amounts[i] = formatMoney(leg.Amount)
	}
	labelWidth := widestLen(labels) + 2
	payeeWidth := widestLen(payees) + 2
	amountWidth := widestLen(amounts)

	rows := make([]string, len(legs))
	for i, leg := range legs {
		rows[i] = fmt.Sprintf("%s%s  %-*s%-*s%*s  other account: %s",
			leads[i], leg.Date.Format("2006-01-02"), labelWidth, labels[i], payeeWidth, payees[i],
			amountWidth, amounts[i], otherAccountLabel(leg))
	}
	return rows
}

// otherAccountLabel renders the account a one-sided leg names: "unknown"
// for a numeric link, the recorded name, or the name marked "(not in this
// file)" when no imported account carries it.
func otherAccountLabel(leg store.OneSidedTransfer) string {
	switch {
	case leg.OtherAccount == nil:
		return "unknown"
	case leg.OtherAccountID == nil:
		return escapeCell(*leg.OtherAccount) + " (not in this file)"
	default:
		return escapeCell(*leg.OtherAccount)
	}
}

// widestLen returns the length of the longest of ss, 0 for an empty slice.
func widestLen(ss []string) int {
	n := 0
	for _, s := range ss {
		n = max(n, len(s))
	}
	return n
}

// renderStoreFailure renders the V1 block: Store, Rows, Transfers, and each
// of Balances/Splits/Shares in its DIFFER form only when that check itself failed.
func renderStoreFailure(result store.Result, storeExisted bool, home string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%-10s%s\n", "Store", storeFailureLine(result.Path, storeExisted, home))
	fmt.Fprintf(&b, "%-10s%s\n", "Rows", rowsPhrase(result.Counts, result.NotImported))

	if mismatched := result.Validation.Balances.Mismatched; len(mismatched) > 0 {
		fmt.Fprintf(&b, "%-10s%s\n", "Balances", balancesDifferPhrase(result.Validation.Balances))
		for _, row := range balanceMismatchRows(mismatched) {
			fmt.Fprintln(&b, row)
		}
	} else {
		bc := result.Validation.Balances
		fmt.Fprintf(&b, "%-10s%s\n", "Balances", balancesPhrase(balanceCounts{Checked: bc.Checked, NeverReconciled: len(bc.NeverReconciled), InvestmentAccounts: bc.InvestmentAccounts}))
	}

	if mismatched := result.Validation.Splits.Mismatched; len(mismatched) > 0 {
		fmt.Fprintf(&b, "%-10s%s\n", "Splits", splitsDifferPhrase(result.Validation.Splits))
		for _, row := range splitMismatchRows(mismatched) {
			fmt.Fprintln(&b, row)
		}
	} else {
		fmt.Fprintf(&b, "%-10s%s\n", "Splits", splitsPhrase(result.Validation.Splits.Checked))
	}

	if mismatched := result.Validation.Shares.Mismatched; len(mismatched) > 0 {
		fmt.Fprintf(&b, "%-10s%s\n", "Shares", sharesDifferPhrase(result.Validation.Shares))
		for _, row := range shareMismatchRows(mismatched) {
			fmt.Fprintln(&b, row)
		}
	} else {
		fmt.Fprintf(&b, "%-10s%s\n", "Shares", sharesPhrase(result.Validation.Shares.Checked))
	}
	writeTransfersLine(&b, result.Validation.Transfers)
	writeOneSidedRows(&b, result.Validation.Transfers.OneSided)
	return b.String()
}

// storeFailureLine renders the Store line for a failed build: NOT REBUILT
// when a previous store existed, NOT BUILT for a first run.
func storeFailureLine(path string, storeExisted bool, home string) string {
	abbreviated := homepath.Abbreviate(home, path)
	if storeExisted {
		return fmt.Sprintf("NOT REBUILT (%s unchanged)", abbreviated)
	}
	return fmt.Sprintf("NOT BUILT (no store at %s yet)", abbreviated)
}
