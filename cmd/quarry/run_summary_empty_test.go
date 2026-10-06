package main

import (
	"bytes"
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

func Test_run_summary_shows_no_change_in_the_first_month_of_data(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	chequing := chequingAccount("acct-cad", 1)
	replaceStore(t, home, chargeRows([]store.Account{chequing},
		summaryTxn(chequing.ID, "Employer", "cat-fuel", day(2026, time.September, 5), 1_000_000),
		summaryTxn(chequing.ID, "Bakery", "cat-groceries", day(2026, time.September, 10), -2000)))
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"summary", "--month", "2026-09"}, spendEnvAt(&stdout, &stderr, summaryClock))

	require.Equal(t, 0, exitCode)
	assert.Empty(t, stderr.String())
	assert.Equal(t, "Summary of September 2026 (2026-09-01 to 2026-09-30), amounts in CAD\n\n"+
		"Snapshot  20260929T000000Z, time taken not recorded in its manifest\n"+
		"Dates     2026-09-05 to 2026-09-10\n"+
		"Findings  none open\n\n"+
		"Unusually large charges 2026-09-01 to 2026-09-30 in all accounts, amounts in CAD\n\n"+
		"No unusually large charges.\n\n"+
		"1 charge checked\n\n"+
		"Recurring charges new 2026-09-01 to 2026-09-30 in all accounts, amounts in CAD\n\n"+
		"No new recurring charges.\n\n"+
		"Net worth at each month end 2026-08-31 to 2026-09-30, amounts in CAD\n\n"+
		chequingHistoryLine("Month end", "chequing", "Total")+
		"2026-08-31\n"+
		chequingHistoryLine("2026-09-30", "9,980.00", "9,980.00")+
		"\nNo change shown: no account has a balance on 2026-08-31.\n",
		stdout.String())
}

func Test_run_summary_says_so_in_a_month_with_no_unusual_charge_and_no_new_recurring_charge(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
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
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"summary"}, spendEnvAt(&stdout, &stderr, summaryClock))

	require.Equal(t, 0, exitCode)
	assert.Empty(t, stderr.String())
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
