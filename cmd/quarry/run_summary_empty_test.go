package main

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// chequingHistoryLine is one converted history line over the chequing column alone.
func chequingHistoryLine(monthEnd, chequing, total string) string {
	return fmt.Sprintf("%-10s  %8s  %8s\n", monthEnd, chequing, total)
}

// firstMonthSummary is the September summary of a store whose first transactions fall in September.
func firstMonthSummary(snapshotRow string) string {
	return "Summary of September 2026 (2026-09-01 to 2026-09-30), amounts in CAD\n\n" +
		snapshotRow +
		"Dates     2026-09-05 to 2026-09-10\n" +
		"Findings  none open\n\n" +
		"Unusually large charges 2026-09-01 to 2026-09-30 in all accounts, amounts in CAD\n\n" +
		"No unusually large charges.\n\n" +
		"1 charge checked\n\n" +
		"Recurring charges new 2026-09-01 to 2026-09-30 in all accounts, amounts in CAD\n\n" +
		"No new recurring charges.\n\n" +
		"Net worth at each month end 2026-08-31 to 2026-09-30, amounts in CAD\n\n" +
		chequingHistoryLine("Month end", "chequing", "Total") +
		"2026-08-31\n" +
		chequingHistoryLine("2026-09-30", "9,980.00", "9,980.00") +
		"\nNo change shown: no account has a balance on 2026-08-31.\n"
}

// firstMonthRows holds a payday and a purchase in September 2026, the store's first month.
func firstMonthRows() store.Rows {
	chequing := chequingAccount("acct-cad", 1)
	return chargeRows([]store.Account{chequing},
		summaryTxn(chequing.ID, "Employer", "cat-fuel", day(2026, time.September, 5), 1_000_000),
		summaryTxn(chequing.ID, "Bakery", "cat-groceries", day(2026, time.September, 10), -2000))
}

func Test_run_summary_shows_no_change_in_the_first_month_of_data_and_warns_when_the_snapshot_time_is_unknown(t *testing.T) {
	home := newHome(t)
	replaceStore(t, home, firstMonthRows())

	exitCode, stdout, stderr := runSpendCaptureAt(context.Background(), []string{"summary", "--month", "2026-09"}, summaryClock)

	require.Equal(t, 0, exitCode)
	assert.Equal(t, septemberTimeUnknownWarning, stderr.String())
	assert.Equal(t, firstMonthSummary("Snapshot  20260929T000000Z, time taken not recorded in its manifest\n"), stdout.String())
}

func Test_run_summary_shows_no_change_in_the_first_month_of_data_without_a_warning_when_the_snapshot_covers_it(t *testing.T) {
	home := newHome(t)
	pinLocalZone(t)
	replaceStore(t, home, withSnapshotTaken(firstMonthRows(), summaryTakenAt))

	exitCode, stdout, stderr := runSpendCaptureAt(context.Background(), []string{"summary", "--month", "2026-09"}, summaryClock)

	require.Equal(t, 0, exitCode)
	assert.Empty(t, stderr.String())
	assert.Equal(t, firstMonthSummary("Snapshot  20260929T000000Z, taken 2026-10-01 09:05 EDT (4 days ago)\n"), stdout.String())
}

func Test_run_summary_says_so_in_a_month_with_no_unusual_charge_and_no_new_recurring_charge(t *testing.T) {
	home := newHome(t)
	chequing := chequingAccount("acct-cad", 1)
	txns := make([]chargeTxn, 0, 7)
	txns = append(txns, summaryTxn(chequing.ID, "Employer", "cat-fuel", day(2026, time.January, 2), 500_000))
	for i, d := range []time.Time{
		day(2026, time.January, 5), day(2026, time.February, 20), day(2026, time.April, 11), day(2026, time.June, 2), day(2026, time.July, 25),
	} {
		txns = append(txns, summaryTxn(chequing.ID, "Bell Canada", "cat-groceries", d, -[]int64{9000, 9300, 9605, 9900, 10200}[i]))
	}
	txns = append(txns, summaryTxn(chequing.ID, "Bell Canada", "cat-groceries", day(2026, time.September, 14), -10000))
	replaceStore(t, home, chargeRows([]store.Account{chequing}, txns...))

	exitCode, stdout, stderr := runSpendCaptureAt(context.Background(), []string{"summary"}, summaryClock)

	require.Equal(t, 0, exitCode)
	assert.Equal(t, septemberTimeUnknownWarning, stderr.String())
	assert.Equal(t, "Summary of September 2026 (2026-09-01 to 2026-09-30), amounts in CAD\n\n"+
		"Snapshot  20260929T000000Z, time taken not recorded in its manifest\n"+
		"Dates     2026-01-02 to 2026-09-14\n"+
		"Findings  none open\n\n"+
		"Unusually large charges 2026-09-01 to 2026-09-30 in all accounts, amounts in CAD\n\n"+
		"No unusually large charges.\n\n"+
		"1 charge checked\n\n"+
		"Recurring charges new 2026-09-01 to 2026-09-30 in all accounts, amounts in CAD\n\n"+
		"No new recurring charges.\n\n"+
		"Net worth at each month end 2026-08-31 to 2026-09-30, amounts in CAD\n\n"+
		chequingHistoryLine("Month end", "chequing", "Total")+
		chequingHistoryLine("2026-08-31", "4,519.95", "4,519.95")+
		chequingHistoryLine("2026-09-30", "4,419.95", "4,419.95")+
		chequingHistoryLine("Change", "-100.00", "-100.00"),
		stdout.String())
}

func Test_run_summary_counts_a_large_charge_with_too_little_history_as_not_judged_in_the_empty_section(t *testing.T) {
	home := newHome(t)
	chequing := chequingAccount("acct-cad", 1)
	replaceStore(t, home, chargeRows([]store.Account{chequing},
		summaryTxn(chequing.ID, "Appliance Store", "cat-groceries", day(2026, time.September, 12), -15000)))

	exitCode, stdout, stderr := runSpendCaptureAt(context.Background(), []string{"summary", "--month", "2026-09"}, summaryClock)

	require.Equal(t, 0, exitCode)
	assert.Equal(t, septemberTimeUnknownWarning, stderr.String())
	assert.Contains(t, stdout.String(), "\n\nNo unusually large charges.\n\n1 charge checked; 1 had too little history to judge\n\n")
}

// noTransactionsSeptemberRest is the body of a September summary below its Findings row, for a store without transactions.
const noTransactionsSeptemberRest = "Unusually large charges 2026-09-01 to 2026-09-30 in all accounts, amounts in CAD\n\n" +
	"No unusually large charges.\n\n" +
	"0 charges checked\n\n" +
	"Recurring charges new 2026-09-01 to 2026-09-30 in all accounts, amounts in CAD\n\n" +
	"No new recurring charges.\n\n" +
	"Net worth at each month end 2026-08-31 to 2026-09-30, amounts in CAD\n\n" +
	"No account has a balance on 2026-08-31 or 2026-09-30.\n"

func Test_run_summary_prints_the_ruled_empty_lines_for_a_store_without_transactions_and_a_month_before_all_data(t *testing.T) {
	cases := []struct {
		name string
		args []string
		seed func(t *testing.T)
		top  string
		rest string
		// stderr is empty unless the store records no snapshot time, as chargeRows leaves it.
		stderr string
	}{
		{
			name:   "store without transactions, snapshot time unknown",
			stderr: septemberTimeUnknownWarning,
			args:   []string{"summary"},
			seed: func(t *testing.T) {
				t.Helper()
				home := newHome(t)
				replaceStore(t, home, chargeRows([]store.Account{chequingAccount("acct-cad", 1)}))
			},
			top: "Summary of September 2026 (2026-09-01 to 2026-09-30), amounts in CAD\n\n" +
				"Snapshot  20260929T000000Z, time taken not recorded in its manifest\n" +
				"Dates     no transactions\n" +
				"Findings  none open\n\n",
			rest: noTransactionsSeptemberRest,
		},
		{
			name: "store without transactions, snapshot taken after the month ended",
			args: []string{"summary"},
			seed: func(t *testing.T) {
				t.Helper()
				home := newHome(t)
				pinLocalZone(t)
				replaceStore(t, home, withSnapshotTaken(chargeRows([]store.Account{chequingAccount("acct-cad", 1)}), summaryTakenAt))
			},
			top: "Summary of September 2026 (2026-09-01 to 2026-09-30), amounts in CAD\n\n" +
				"Snapshot  20260929T000000Z, taken 2026-10-01 09:05 EDT (4 days ago)\n" +
				"Dates     no transactions\n" +
				"Findings  none open\n\n",
			rest: noTransactionsSeptemberRest,
		},
		{
			name: "month before all data",
			args: []string{"summary", "--month", "2020-01"},
			seed: func(t *testing.T) {
				t.Helper()
				seedSummaryStore(t)
				pinLocalZone(t)
			},
			top: "Summary of January 2020 (2020-01-01 to 2020-01-31), amounts in CAD\n\n" +
				"Snapshot  20261001T130512Z, taken 2026-10-01 09:05 EDT (4 days ago)\n" +
				"Dates     2026-01-02 to 2026-09-15\n" +
				"Findings  2 open; the last sync found 1 new and 1 fixed; run quarry findings to list them\n\n",
			rest: "Unusually large charges 2020-01-01 to 2020-01-31 in all accounts, amounts in CAD\n\n" +
				"No unusually large charges.\n\n" +
				"0 charges checked\n\n" +
				"Recurring charges new 2020-01-01 to 2020-01-31 in all accounts, amounts in CAD\n\n" +
				"No new recurring charges.\n\n" +
				"Net worth at each month end 2019-12-31 to 2020-01-31, amounts in CAD\n\n" +
				"No account has a balance on 2019-12-31 or 2020-01-31.\n",
		},
		{
			name: "empty month between data",
			args: []string{"summary", "--month", "2026-03"},
			seed: func(t *testing.T) {
				t.Helper()
				seedSummaryStore(t)
				pinLocalZone(t)
			},
			top: "Summary of March 2026 (2026-03-01 to 2026-03-31), amounts in CAD\n\n" +
				"Snapshot  20261001T130512Z, taken 2026-10-01 09:05 EDT (4 days ago)\n" +
				"Dates     2026-01-02 to 2026-09-15\n" +
				"Findings  2 open; the last sync found 1 new and 1 fixed; run quarry findings to list them\n\n",
			rest: "Unusually large charges 2026-03-01 to 2026-03-31 in all accounts, amounts in CAD\n\n" +
				"No unusually large charges.\n\n" +
				"0 charges checked\n\n" +
				"Recurring charges new 2026-03-01 to 2026-03-31 in all accounts, amounts in CAD\n\n" +
				"No new recurring charges.\n\n" +
				"Net worth at each month end 2026-02-28 to 2026-03-31, amounts in CAD\n\n" +
				chequingHistoryLine("Month end", "chequing", "Total") +
				chequingHistoryLine("2026-02-28", "4,817.00", "4,817.00") +
				chequingHistoryLine("2026-03-31", "4,817.00", "4,817.00") +
				chequingHistoryLine("Change", "0.00", "0.00"),
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			c.seed(t)

			exitCode, stdout, stderr := runSpendCaptureAt(context.Background(), c.args, summaryClock)

			require.Equal(t, 0, exitCode)
			assert.Equal(t, c.stderr, stderr.String())
			assert.Equal(t, c.top+c.rest, stdout.String())
		})
	}
}

func Test_run_summary_shows_no_change_in_native_when_no_currency_has_a_balance_on_the_first_month_end(t *testing.T) {
	home := newHome(t)
	chequing, usd := chequingAccount("acct-cad", 1), usdChequingAccount("acct-usd", 2)
	usdDeposit := chargeTxn{
		id: "usd-deposit", account: usd.ID, payee: "Employer", currency: "USD", day: day(2026, time.September, 6),
		splits: []chargeSplit{{category: "cat-fuel", cents: 50_000}},
	}
	replaceStore(t, home, chargeRows([]store.Account{chequing, usd},
		summaryTxn(chequing.ID, "Employer", "cat-fuel", day(2026, time.September, 5), 100_000), usdDeposit))

	exitCode, stdout, stderr := runSpendCaptureAt(context.Background(), []string{"summary", "--month", "2026-09", "--currency", "native"}, summaryClock)

	require.Equal(t, 0, exitCode)
	assert.Equal(t, septemberTimeUnknownWarning, stderr.String())
	assert.Equal(t, "Summary of September 2026 (2026-09-01 to 2026-09-30)\n\n"+
		"Snapshot  20260929T000000Z, time taken not recorded in its manifest\n"+
		"Dates     2026-09-05 to 2026-09-06\n"+
		"Findings  none open\n\n"+
		"Unusually large charges 2026-09-01 to 2026-09-30 in all accounts\n\n"+
		"No unusually large charges.\n\n"+
		"0 charges checked\n\n"+
		"Recurring charges new 2026-09-01 to 2026-09-30 in all accounts\n\n"+
		"No new recurring charges.\n\n"+
		"Net worth at each month end 2026-08-31 to 2026-09-30\n\n"+
		"Month end   Currency  chequing     Total\n"+
		"2026-08-31\n"+
		"2026-09-30  CAD       1,000.00  1,000.00\n"+
		"2026-09-30  USD         500.00    500.00\n"+
		"\nNo change shown: no account has a balance on 2026-08-31.\n",
		stdout.String())
}
