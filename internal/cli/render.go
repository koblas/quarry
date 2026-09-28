package cli

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/koblas/quarry/internal/platform/homepath"
	"github.com/koblas/quarry/internal/platform/sqlschema"
	"github.com/koblas/quarry/internal/snapshot"
	"github.com/koblas/quarry/internal/store"
)

// formatThousands renders n, which is always non-negative in this package's
// callers (byte counts, account counts, table/column counts), with a comma
// every three digits from the right.
func formatThousands(n int) string {
	s := strconv.Itoa(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

// formatMB renders bytes as decimal megabytes with one decimal place and
// thousands-grouped whole MB.
func formatMB(bytes int64) string {
	tenths := int64(math.Round(float64(bytes) / 100000))
	return fmt.Sprintf("%s.%d MB", formatThousands(int(tenths/10)), tenths%10)
}

// nounPhrase renders n with singular at exactly 1 and plural at 0 or more
// than 1, thousands-grouped.
func nounPhrase(n int, singular, plural string) string {
	if n == 1 {
		return "1 " + singular
	}
	return formatThousands(n) + " " + plural
}

// accountsPhrase renders n as "1 account" or "N accounts", thousands-grouped.
func accountsPhrase(n int) string {
	return nounPhrase(n, "account", "accounts")
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
		s.Reference, formatThousands(s.ReferenceTables), formatThousands(s.ReferenceColumns))
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

// renderStore renders result's Store, Rows, Balances, Splits and Transfers
// lines, appended after renderSuccess's block once a build was reached.
func renderStore(result store.Result, home string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%-10s%s\n", "Store", homepath.Abbreviate(home, result.Path))
	fmt.Fprintf(&b, "%-10s%s\n", "Rows", rowsPhrase(result.Counts, result.NotImported))
	fmt.Fprintf(&b, "%-10s%s\n", "Balances", balancesPhrase(result.Validation.Balances))
	fmt.Fprintf(&b, "%-10s%s\n", "Splits", splitsPhrase(result.Validation.Splits))
	fmt.Fprintf(&b, "%-10s%s\n", "Transfers", transfersPhrase(result.Validation.Transfers))
	return b.String()
}

// transfersPhrase renders tc as "none", "N paired", or "N paired, M
// one-sided" once any leg is one-sided.
func transfersPhrase(tc store.TransferCheck) string {
	switch {
	case tc.Paired == 0 && len(tc.OneSided) == 0:
		return "none"
	case len(tc.OneSided) == 0:
		return formatThousands(tc.Paired) + " paired"
	default:
		return fmt.Sprintf("%s paired, %s one-sided", formatThousands(tc.Paired), formatThousands(len(tc.OneSided)))
	}
}

// balancesCheckedPhrase renders bc's checked clause: no accounts checked,
// exactly one matching, or the plural count.
func balancesCheckedPhrase(n int) string {
	switch n {
	case 0:
		return "no accounts to check"
	case 1:
		return "1 account matches Quicken's last reconciled balance"
	default:
		return formatThousands(n) + " accounts match Quicken's last reconciled balance"
	}
}

// balancesPhrase appends bc's never-reconciled and investment-account
// clauses (each omitted at zero, joined with " and ") to its checked clause.
func balancesPhrase(bc store.BalanceCheck) string {
	return balancesCheckedPhrase(bc.Checked) + balancesExtrasPhrase(bc)
}

// balancesExtrasPhrase renders bc's never-reconciled and investment-account
// clauses, each omitted at zero and joined with " and ", prefixed with a
// "; " separator when any exist.
func balancesExtrasPhrase(bc store.BalanceCheck) string {
	var extras []string
	if n := len(bc.NeverReconciled); n > 0 {
		extras = append(extras, nounPhrase(n, "never reconciled", "never reconciled"))
	}
	if bc.InvestmentAccounts > 0 {
		extras = append(extras, nounPhrase(bc.InvestmentAccounts, "investment account not checked", "investment accounts not checked"))
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
	return fmt.Sprintf("%d of %d %s", x, y, noun)
}

// balancesDifferPhrase renders bc's V1 clause: how many of the checked
// accounts differ, plus its never-reconciled and investment-account extras.
func balancesDifferPhrase(bc store.BalanceCheck) string {
	return "DIFFER for " + xOfYPhrase(len(bc.Mismatched), bc.Checked, "account", "accounts") + balancesExtrasPhrase(bc)
}

// splitsDifferPhrase renders sc's V1 clause: how many of the checked
// transactions differ.
func splitsDifferPhrase(sc store.SplitCheck) string {
	return "DIFFER for " + xOfYPhrase(len(sc.Mismatched), sc.Checked, "transaction", "transactions")
}

// splitsPhrase renders sc's checked clause: no transactions checked,
// exactly one equalling its splits, or the plural count.
func splitsPhrase(sc store.SplitCheck) string {
	switch sc.Checked {
	case 0:
		return "no transactions to check"
	case 1:
		return "the 1 transaction equals the sum of its splits"
	default:
		return "all " + formatThousands(sc.Checked) + " transactions equal the sum of their splits"
	}
}

// rowsPhrase renders c as one comma-separated clause, each noun inflected
// on its own count, then n's investment-transaction clause when nonzero.
func rowsPhrase(c store.Counts, n store.NotImported) string {
	phrase := strings.Join([]string{
		nounPhrase(c.Transactions, "transaction", "transactions"),
		nounPhrase(c.Splits, "split", "splits"),
		nounPhrase(c.Transfers, "transfer", "transfers"),
		nounPhrase(c.Payees, "payee", "payees"),
		nounPhrase(c.Categories, "category", "categories"),
		nounPhrase(c.Tags, "tag", "tags"),
	}, ", ")
	if n.InvestmentTransactions > 0 {
		phrase += "; " + nounPhrase(n.InvestmentTransactions, "investment transaction", "investment transactions") + " not imported"
	}
	return phrase
}

// formatMoney renders cents as a thousands-grouped, 2-decimal amount with a
// leading "-" for a negative value.
func formatMoney(cents int64) string {
	negative := cents < 0
	if negative {
		cents = -cents
	}
	s := fmt.Sprintf("%s.%02d", formatThousands(int(cents/100)), cents%100)
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
	return fmt.Sprintf("%s (%s%s)", name, currency, suffix)
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
		labels[i] = accountLabel(m.Account, m.Currency, false, true)
		payees[i] = m.Payee
		if payees[i] == "" {
			payees[i] = "(no payee)"
		}
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

// widestLen returns the length of the longest of ss, 0 for an empty slice.
func widestLen(ss []string) int {
	n := 0
	for _, s := range ss {
		n = max(n, len(s))
	}
	return n
}

// renderStoreFailure renders the V1 block: Store, Rows, Transfers, and each
// of Balances/Splits in its DIFFER form only when that check itself failed.
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
		fmt.Fprintf(&b, "%-10s%s\n", "Balances", balancesPhrase(result.Validation.Balances))
	}

	if mismatched := result.Validation.Splits.Mismatched; len(mismatched) > 0 {
		fmt.Fprintf(&b, "%-10s%s\n", "Splits", splitsDifferPhrase(result.Validation.Splits))
		for _, row := range splitMismatchRows(mismatched) {
			fmt.Fprintln(&b, row)
		}
	} else {
		fmt.Fprintf(&b, "%-10s%s\n", "Splits", splitsPhrase(result.Validation.Splits))
	}
	fmt.Fprintf(&b, "%-10s%s\n", "Transfers", transfersPhrase(result.Validation.Transfers))
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
