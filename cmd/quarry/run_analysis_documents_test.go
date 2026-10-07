// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"context"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// analysisRun is one command line over a store and the exact bytes it prints.
type analysisRun struct {
	name   string
	store  func(t *testing.T, home string)
	args   []string
	stdout string
	stderr string
}

// Byte-for-byte on purpose: the recurring and anomalies pins decode their JSON and
// ignore key order, and these four reports are rendered by more than one surface.
func Test_run_prints_spend_cashflow_recurring_and_anomalies_byte_for_byte(t *testing.T) {
	for _, c := range analysisRuns() {
		t.Run(c.name, func(t *testing.T) {
			home := newHome(t)
			c.store(t, home)

			exitCode, stdout, stderr := runSpendCapture(context.Background(), c.args)

			require.Equal(t, 0, exitCode, stderr.String())
			assert.Equal(t, c.stdout, stdout.String())
			assert.Equal(t, c.stderr, stderr.String())
		})
	}
}

// analysisRuns is the table of command lines over stores and the bytes each must print.
func analysisRuns() []analysisRun {
	populatedAccounts := []string{"--account", "Chequing", "--account", "US Chequing", "--account", "Linked", "--account", "Old Card"}
	args := func(command string, more ...string) []string {
		return append([]string{command}, more...)
	}
	cases := []analysisRun{
		{
			name: "spend --json over named accounts, a left-out pair and a USD span across the first rate", store: populatedAnalysisStore,
			args: args("spend", append([]string{"--json"}, populatedAccounts...)...), stdout: goldenSpendPopulatedJSONStdout, stderr: goldenSpendPopulatedJSONStderr,
		},
		{
			name: "spend text over named accounts, a left-out pair and a USD span across the first rate", store: populatedAnalysisStore,
			args: args("spend", populatedAccounts...), stdout: goldenSpendPopulatedTextStdout, stderr: goldenSpendPopulatedTextStderr,
		},
		{
			name: "cashflow --json over named accounts, a left-out pair and a USD span across the first rate", store: populatedAnalysisStore,
			args: args("cashflow", append([]string{"--json"}, populatedAccounts...)...), stdout: goldenCashflowPopulatedJSONStdout, stderr: goldenCashflowPopulatedJSONStderr,
		},
		{
			name: "cashflow text over named accounts, a left-out pair and a USD span across the first rate", store: populatedAnalysisStore,
			args: args("cashflow", populatedAccounts...), stdout: goldenCashflowPopulatedTextStdout, stderr: goldenCashflowPopulatedTextStderr,
		},
		{
			name: "recurring --json over named accounts, a left-out pair and a USD span across the first rate", store: populatedAnalysisStore,
			args: args("recurring", append([]string{"--json"}, populatedAccounts...)...), stdout: goldenRecurringPopulatedJSONStdout, stderr: goldenRecurringPopulatedJSONStderr,
		},
		{
			name: "recurring text over named accounts, a left-out pair and a USD span across the first rate", store: populatedAnalysisStore,
			args: args("recurring", populatedAccounts...), stdout: goldenRecurringPopulatedTextStdout, stderr: goldenRecurringPopulatedTextStderr,
		},
		{
			name: "anomalies --json over named accounts, a left-out pair and a USD span across the first rate", store: populatedAnalysisStore,
			args: args("anomalies", append([]string{"--json"}, populatedAccounts...)...), stdout: goldenAnomaliesPopulatedJSONStdout, stderr: goldenAnomaliesPopulatedJSONStderr,
		},
		{
			name: "anomalies text over named accounts, a left-out pair and a USD span across the first rate", store: populatedAnalysisStore,
			args: args("anomalies", populatedAccounts...), stdout: goldenAnomaliesPopulatedTextStdout, stderr: goldenAnomaliesPopulatedTextStderr,
		},

		{
			name: "spend --json over an empty window", store: emptyWindowAnalysisStore,
			args: args("spend", "--json"), stdout: goldenSpendEmptyJSONStdout, stderr: goldenSpendEmptyJSONStderr,
		},
		{
			name: "spend text over an empty window", store: emptyWindowAnalysisStore,
			args: args("spend"), stdout: goldenSpendEmptyTextStdout, stderr: goldenSpendEmptyTextStderr,
		},
		{
			name: "cashflow --json over an empty window", store: emptyWindowAnalysisStore,
			args: args("cashflow", "--json"), stdout: goldenCashflowEmptyJSONStdout, stderr: goldenCashflowEmptyJSONStderr,
		},
		{
			name: "cashflow text over an empty window", store: emptyWindowAnalysisStore,
			args: args("cashflow"), stdout: goldenCashflowEmptyTextStdout, stderr: goldenCashflowEmptyTextStderr,
		},
		{
			name: "recurring --json over an empty window", store: emptyWindowAnalysisStore,
			args: args("recurring", "--json"), stdout: goldenRecurringEmptyJSONStdout, stderr: goldenRecurringEmptyJSONStderr,
		},
		{
			name: "recurring text over an empty window", store: emptyWindowAnalysisStore,
			args: args("recurring"), stdout: goldenRecurringEmptyTextStdout, stderr: goldenRecurringEmptyTextStderr,
		},
		{
			name: "anomalies --json over an empty window", store: emptyWindowAnalysisStore,
			args: args("anomalies", "--json"), stdout: goldenAnomaliesEmptyJSONStdout, stderr: goldenAnomaliesEmptyJSONStderr,
		},
		{
			name: "anomalies text over an empty window", store: emptyWindowAnalysisStore,
			args: args("anomalies"), stdout: goldenAnomaliesEmptyTextStdout, stderr: goldenAnomaliesEmptyTextStderr,
		},

		{
			name: "spend --by tag --json with a split carrying two tags", store: multiTagAnalysisStore,
			args: args("spend", "--by", "tag", "--json"), stdout: goldenSpendMultiTagJSONStdout, stderr: goldenSpendMultiTagJSONStderr,
		},
		{
			name: "spend --by tag text with a split carrying two tags", store: multiTagAnalysisStore,
			args: args("spend", "--by", "tag"), stdout: goldenSpendMultiTagTextStdout, stderr: goldenSpendMultiTagTextStderr,
		},
	}

	return cases
}

// populatedAnalysisStore holds CAD and USD accounts plus one linked-tracking and one
// not-in-reports account; USD charges run on both sides of the first rate, 2026-03-01.
func populatedAnalysisStore(t *testing.T, home string) {
	t.Helper()
	accounts := []store.Account{
		chequingAccount("acct-cad", 1),
		usdChequingAccount("acct-usd", 2),
		{ID: "acct-linked", SourceID: 3, Name: "Linked", Type: "retirement", Currency: "CAD", Active: true, LinkedTracking: true},
		{ID: "acct-old", SourceID: 4, Name: "Old Card", Type: "credit_card", Currency: "CAD", Active: true, NotInReports: true},
	}
	charges := monthlySeries("Netflix.com", 2026, time.February, 999, 999, 999, 999, 1199, 1199, 1199, 1199)
	charges = append(charges, hardwareHistory()...)
	charges = append(charges, bigHardware())
	charges = append(charges, inUSD(lumberHistoryWithBigCharge())...)
	charges = append(charges, inUSD(monthlySeries("Spotify", 2025, time.November, 1099, 1099, 1099, 1099, 1099, 1099, 1099))...)
	charges = append(charges,
		fuelCharge("acct-cad", day(2026, time.May, 2), 4500),
		salary("pay-march", day(2026, time.March, 1), 500000),
		salary("pay-april", day(2026, time.April, 1), 500000),
		onAccount(groceryCharge("Whole Foods", day(2026, time.March, 11), 90000), "acct-linked"),
		onAccount(groceryCharge("Costco", day(2026, time.March, 12), 40000), "acct-old"),
	)
	rows := chargeRows(accounts, charges...)
	rows.Categories = append(rows.Categories,
		store.Category{ID: "cat-salary", SourceID: 3, Name: "Salary", FullPath: "Income:Salary", Kind: "income"})
	replaceStoreWithRates(t, home, rows, usdRate(day(2026, time.March, 1), 1_300_000))
}

// lumberHistoryWithBigCharge is five 2025 Lumber charges of 38.00 to 42.00, then a 250.00 one on 2026-02-02.
func lumberHistoryWithBigCharge() []chargeTxn {
	history := make([]chargeTxn, 0, 6)
	for i, cents := range []int64{3800, 3900, 4000, 4100, 4200} {
		history = append(history, groceryCharge("Lumber", day(2025, time.March, 3+7*i), cents))
	}
	return append(history, groceryCharge("Lumber", day(2026, time.February, 2), 25000))
}

// emptyWindowAnalysisStore holds charges only before 2026, so the default window is empty.
func emptyWindowAnalysisStore(t *testing.T, home string) {
	t.Helper()
	replaceStore(t, home, chargeRows([]store.Account{chequingAccount("acct-cad", 1)}, bakeryHistory()...))
}

// multiTagAnalysisStore holds a grocery split tagged vacation and alpha beside a fuel split tagged alpha.
func multiTagAnalysisStore(t *testing.T, home string) {
	t.Helper()
	replaceStore(t, home, spendRows([]store.Account{chequingAccount("acct-cad", 1)},
		spendSplit{
			id: "s01", account: "acct-cad", category: "cat-groceries", currency: "CAD", day: day(2026, time.March, 10), cents: -12000,
			tags: []string{"tag-vacation", "tag-alpha"},
		},
		spendSplit{
			id: "s02", account: "acct-cad", category: "cat-fuel", currency: "CAD", day: day(2026, time.April, 2), cents: -3000,
			tags: []string{"tag-alpha"},
		},
	))
}

// fuelCharge is a fuel charge of cents on account, with no payee.
func fuelCharge(account string, date time.Time, cents int64) chargeTxn {
	return chargeTxn{
		id: "fuel" + date.Format(time.DateOnly), account: account, currency: "CAD", day: date,
		splits: []chargeSplit{{category: "cat-fuel", cents: -cents}},
	}
}

// salary is an income deposit of cents on the CAD chequing account.
func salary(id string, date time.Time, cents int64) chargeTxn {
	return chargeTxn{
		id: id, account: "acct-cad", payee: "Employer", currency: "CAD", day: date,
		splits: []chargeSplit{{category: "cat-salary", cents: cents}},
	}
}

// onAccount is charge moved to account.
func onAccount(charge chargeTxn, account string) chargeTxn {
	charge.account = account
	return charge
}
