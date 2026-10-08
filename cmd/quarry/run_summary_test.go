// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/koblas/quarry/internal/snapshot"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// summaryTakenAt is when the summary fixture's last build read its snapshot: after September ended.
var summaryTakenAt = time.Date(2026, time.October, 1, 13, 5, 12, 0, time.UTC)

// summaryClock is the instant the summary tests run at: October 6, so the default month is September.
var summaryClock = time.Date(2026, time.October, 6, 12, 0, 0, 0, time.UTC)

var errSummaryBuiltSyncServer = errors.New("summary must not build a sync server")

// summaryCardAccount is an active CAD credit card named "Card".
func summaryCardAccount(id string, sourceID int64) store.Account {
	return store.Account{ID: id, SourceID: sourceID, Name: "Card", Type: "credit_card", Currency: "CAD", Active: true}
}

// summaryTxn is a one-split transaction in account of cents (negative is money out) in category.
func summaryTxn(account, payee, category string, d time.Time, cents int64) chargeTxn {
	return chargeTxn{
		id: payee + d.Format(time.DateOnly), account: account, payee: payee, currency: "CAD", day: d,
		splits: []chargeSplit{{category: category, cents: cents}},
	}
}

// summaryRows is the store of September 2026 the summary tests read: Bell Canada's 412.00 charge is anomalous,
// Crave is new and recurring, and Kiosk's finding is fixed (Pharmacy's found) when kioskFixed.
func summaryRows(kioskFixed bool) store.Rows {
	chequing, card := chequingAccount("acct-cad", 1), summaryCardAccount("acct-card", 2)
	var txns []chargeTxn
	txns = append(txns, summaryTxn(chequing.ID, "Employer", "cat-fuel", day(2026, time.January, 2), 500_000))
	for i, d := range []time.Time{
		day(2026, time.January, 5), day(2026, time.February, 20), day(2026, time.April, 11), day(2026, time.June, 2), day(2026, time.July, 25),
	} {
		txns = append(txns, summaryTxn(chequing.ID, "Bell Canada", "cat-groceries", d, -[]int64{9000, 9300, 9605, 9900, 10200}[i]))
	}
	txns = append(txns,
		summaryTxn(chequing.ID, "Bell Canada", "cat-groceries", day(2026, time.September, 14), -41200),
		summaryTxn(chequing.ID, "Employer", "cat-fuel", day(2026, time.September, 15), 100_000),
		summaryTxn(card.ID, "Crave", "cat-groceries", day(2026, time.July, 3), -2259),
		summaryTxn(card.ID, "Crave", "cat-groceries", day(2026, time.August, 3), -2259),
		summaryTxn(card.ID, "Crave", "cat-groceries", day(2026, time.September, 3), -2259),
		summaryTxn(chequing.ID, "Shell", "", day(2026, time.August, 10), -1000),
	)
	if kioskFixed {
		txns = append(txns,
			summaryTxn(chequing.ID, "Kiosk", "cat-groceries", day(2026, time.August, 10), -1100),
			summaryTxn(chequing.ID, "Pharmacy", "", day(2026, time.August, 11), -1200))
	} else {
		txns = append(txns, summaryTxn(chequing.ID, "Kiosk", "", day(2026, time.August, 10), -1100))
	}
	rows := chargeRows([]store.Account{chequing, card}, txns...)
	rows.ImportRuns[0].Snapshot = store.SnapshotRef{
		Path: "/snapshots/20261001T130512Z.sqlite", SHA256: "9f86", SchemaFingerprint: "sha256:abc", TakenAt: summaryTakenAt,
	}
	return rows
}

// seedSummaryStore builds the store under a temp HOME twice, so the second build fixes Kiosk's finding and
// finds Pharmacy's while Shell's carries; it returns that HOME.
func seedSummaryStore(t *testing.T) string {
	t.Helper()
	home := newHome(t)
	replaceStore(t, home, summaryRows(false))
	replaceStore(t, home, summaryRows(true))
	return home
}

func Test_run_summary_prints_last_months_summary(t *testing.T) {
	seedSummaryStore(t)
	pinLocalZone(t)
	var stdout, stderr bytes.Buffer
	env := spendEnvAt(&stdout, &stderr, summaryClock)
	env.NewServer = func(context.Context, ...snapshot.Option) (*snapshot.Server, error) {
		t.Error("summary resolved a Quicken path")
		return nil, errSummaryBuiltSyncServer
	}

	exitCode := runWith(context.Background(), []string{"summary"}, env)

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, "Summary of September 2026 (2026-09-01 to 2026-09-30), amounts in CAD\n\n"+
		"Snapshot  20261001T130512Z, taken 2026-10-01 09:05 EDT (4 days ago)\n"+
		"Dates     2026-01-02 to 2026-09-15\n"+
		"Findings  2 open; the last sync found 1 new and 1 fixed; run quarry findings to list them\n\n"+
		anomaliesTable("Unusually large charges 2026-09-01 to 2026-09-30 in all accounts, amounts in CAD", "2 charges checked",
			[]string{"2026-09-14", "Chequing (CAD)", "Bell Canada", "Food:Groceries", "412.00", "96.05", "4.3x", "payee, 5 earlier"})+"\n"+
		recurringTable("Recurring charges new 2026-09-01 to 2026-09-30 in all accounts, amounts in CAD",
			[]string{"Crave", "CAD", "month", "22.59", "271.08", "2026-07-03", "2026-09-03", "active", ""},
			[]string{"Total", "CAD", "", "", "271.08", "", "", "", ""})+"\n"+
		"Net worth at each month end 2026-08-31 to 2026-09-30, amounts in CAD\n\n"+
		netWorthHistoryLine("Month end", "chequing", "credit_card", "Total")+
		netWorthHistoryLine("2026-08-31", "4,486.95", "-45.18", "4,441.77")+
		netWorthHistoryLine("2026-09-30", "5,074.95", "-67.77", "5,007.18")+
		netWorthHistoryLine("Change", "+588.00", "-22.59", "+565.41"),
		stdout.String())
}

func Test_run_summary_defaults_to_the_month_before_the_local_day_not_the_utc_day(t *testing.T) {
	seedSummaryStore(t)
	lateOnSeptember30 := time.Date(2026, time.September, 30, 21, 0, 0, 0, time.FixedZone("UTC-4", -4*60*60))

	exitCode, stdout, stderr := runSpendCaptureAt(context.Background(), []string{"summary"}, lateOnSeptember30)

	require.Equal(t, 0, exitCode, stderr.String())
	assert.True(t, strings.HasPrefix(stdout.String(), "Summary of August 2026 (2026-08-01 to 2026-08-31), amounts in CAD\n"), stdout.String())
}

// summaryNativeRows is summaryRows' CAD history plus US Chequing: five Hulu charges (20.00) a month and a half
// apart then 150.00 on September 14, Spotify's 9.99 on the 8th of July, August and September, and a 1,000.00 deposit.
func summaryNativeRows() store.Rows {
	chequing, card, usd := chequingAccount("acct-cad", 1), summaryCardAccount("acct-card", 2), usdChequingAccount("acct-usd", 3)
	usdTxn := func(payee string, d time.Time, cents int64) chargeTxn {
		return chargeTxn{
			id: "usd-" + payee + d.Format(time.DateOnly), account: usd.ID, payee: payee, currency: "USD", day: d,
			splits: []chargeSplit{{category: "cat-groceries", cents: cents}},
		}
	}
	txns := make([]chargeTxn, 0, 20)
	txns = append(txns,
		summaryTxn(chequing.ID, "Employer", "cat-fuel", day(2026, time.January, 2), 500_000),
		summaryTxn(chequing.ID, "Bell Canada", "cat-groceries", day(2026, time.September, 14), -41200),
		summaryTxn(card.ID, "Crave", "cat-groceries", day(2026, time.July, 3), -2259),
		summaryTxn(card.ID, "Crave", "cat-groceries", day(2026, time.August, 3), -2259),
		summaryTxn(card.ID, "Crave", "cat-groceries", day(2026, time.September, 3), -2259),
		usdTxn("Employer", day(2026, time.January, 2), 100_000),
		usdTxn("Hulu", day(2026, time.September, 14), -15000))
	for i, d := range []time.Time{
		day(2026, time.January, 5), day(2026, time.February, 20), day(2026, time.April, 11), day(2026, time.June, 2), day(2026, time.July, 25),
	} {
		txns = append(txns,
			summaryTxn(chequing.ID, "Bell Canada", "cat-groceries", d, -[]int64{9000, 9300, 9605, 9900, 10200}[i]),
			usdTxn("Hulu", d, -2000))
	}
	for _, month := range []time.Month{time.July, time.August, time.September} {
		txns = append(txns, usdTxn("Spotify", day(2026, month, 8), -999))
	}
	return chargeRows([]store.Account{chequing, card, usd}, txns...)
}

func Test_run_summary_lists_cad_and_usd_separately_with_native(t *testing.T) {
	home := newHome(t)
	replaceStore(t, home, summaryNativeRows())

	exitCode, stdout, stderr := runSpendCaptureAt(context.Background(), []string{"summary", "--currency", "native"}, summaryClock)

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Equal(t, "Summary of September 2026 (2026-09-01 to 2026-09-30)\n\n"+
		"Snapshot  20260929T000000Z, time taken not recorded in its manifest\n"+
		"Dates     2026-01-02 to 2026-09-14\n"+
		"Findings  none open\n\n"+
		anomaliesTable("Unusually large charges 2026-09-01 to 2026-09-30 in all accounts", "4 charges checked",
			[]string{"2026-09-14", "US Chequing (USD)", "Hulu", "Food:Groceries", "150.00", "20.00", "7.5x", "payee, 5 earlier"},
			[]string{"2026-09-14", "Chequing (CAD)", "Bell Canada", "Food:Groceries", "412.00", "96.05", "4.3x", "payee, 5 earlier"})+"\n"+
		recurringTable("Recurring charges new 2026-09-01 to 2026-09-30 in all accounts",
			[]string{"Crave", "CAD", "month", "22.59", "271.08", "2026-07-03", "2026-09-03", "active", ""},
			[]string{"Spotify", "USD", "month", "9.99", "119.88", "2026-07-08", "2026-09-08", "active", ""},
			[]string{"Total", "CAD", "", "", "271.08", "", "", "", ""},
			[]string{"Total", "USD", "", "", "119.88", "", "", "", ""})+"\n"+
		"Net worth at each month end 2026-08-31 to 2026-09-30\n\n"+
		"Month end   Currency  chequing  credit_card     Total\n"+
		"2026-08-31  CAD       4,519.95       -45.18  4,474.77\n"+
		"2026-08-31  USD         880.02                 880.02\n"+
		"2026-09-30  CAD       4,107.95       -67.77  4,040.18\n"+
		"2026-09-30  USD         720.03                 720.03\n"+
		"Change      CAD        -412.00       -22.59   -434.59\n"+
		"Change      USD        -159.99                -159.99\n",
		stdout.String())
}

func Test_run_summary_counts_a_type_without_a_balance_on_the_first_month_end_as_zero_in_the_change(t *testing.T) {
	home := newHome(t)
	card := store.Account{ID: "acct-card", SourceID: 2, Name: "Card", Type: "credit_card", Currency: "CAD", Active: true}
	replaceStore(t, home, spendRows([]store.Account{chequingAccount("acct-cad", 1), card},
		spendSplit{id: "cad-aug", account: "acct-cad", currency: "CAD", day: day(2026, time.August, 15), cents: 100_000},
		spendSplit{id: "card-sep", account: "acct-card", currency: "CAD", day: day(2026, time.September, 10), cents: -5_000},
	))

	exitCode, stdout, stderr := runSpendCaptureAt(context.Background(), []string{"summary"}, summaryClock)

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Contains(t, stdout.String(), "\nNet worth at each month end 2026-08-31 to 2026-09-30, amounts in CAD\n\n"+
		netWorthHistoryLine("Month end", "chequing", "credit_card", "Total")+
		netWorthHistoryLine("2026-08-31", "1,000.00", "", "1,000.00")+
		netWorthHistoryLine("2026-09-30", "1,000.00", "-50.00", "950.00")+
		netWorthHistoryLine("Change", "0.00", "-50.00", "-50.00"))
}

func Test_run_summary_anomalies_section_equals_quarry_anomalies_of_the_month(t *testing.T) {
	seedSummaryStore(t)
	pinLocalZone(t)

	summaryCode, summary, summaryErr := runSpendCaptureAt(context.Background(), []string{"summary"}, summaryClock)
	require.Equal(t, 0, summaryCode, summaryErr.String())
	anomaliesCode, anomalies, anomaliesErr := runSpendCaptureAt(context.Background(),
		[]string{"anomalies", "--since", "2026-09", "--until", "2026-09"}, summaryClock)
	require.Equal(t, 0, anomaliesCode, anomaliesErr.String())

	assert.Contains(t, summary.String(), "\n"+anomalies.String()+"\n")
}

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

// syncSummaryFindingsFixture syncs a file raising one duplicate pair, three uncategorized payees and one
// unclassified brokerage account under home, returning the duplicate's id and the brokerage account's.
func syncSummaryFindingsFixture(t *testing.T, home string) (string, string) {
	t.Helper()
	b := v9fixture.NewBuilder()
	chequingPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	brokeragePK := b.Account(v9fixture.AccountRow{Name: "Questrade TFSA", Type: "BROKERAGENORMAL", Currency: "CAD", Active: true})
	hydroPK := b.Payee(v9fixture.PayeeRow{Name: "Hydro One"})
	hydroNetworksPK := b.Payee(v9fixture.PayeeRow{Name: "HYDRO ONE NETWORKS"})
	billsPK := b.Category(v9fixture.TagRow{Name: "Bills", Type: new(int64(1))})
	first := categorizedPayeeTxn(b, chequingPK, hydroPK, billsPK, time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC), "-142.17")
	second := categorizedPayeeTxn(b, chequingPK, hydroNetworksPK, billsPK, time.Date(2026, 8, 5, 0, 0, 0, 0, time.UTC), "-142.17")
	for i, payee := range []string{"Amazon", "Costco", "Shell"} {
		amount := fmt.Sprintf("-%d0.00", i+1)
		day := time.Date(2026, 3, 1+i, 0, 0, 0, 0, time.UTC)
		txn := b.Transaction(v9fixture.TransactionRow{Account: chequingPK, Amount: amount, PostedDate: &day, Payee: b.Payee(v9fixture.PayeeRow{Name: payee})})
		b.Entry(v9fixture.EntryRow{Parent: txn, Amount: amount})
	}
	syncBundle(t, b.WriteBundle(t, filepath.Join(home, "Documents")))
	return fmt.Sprintf("duplicate:txn-%d+txn-%d", first, second), fmt.Sprintf("acct-%d", brokeragePK)
}

// summaryOf is quarry summary for January 2026, a month that has ended whatever the clock says.
func summaryOf(t *testing.T) string {
	t.Helper()
	exitCode, stdout, stderr := runCapture(context.Background(), []string{"summary", "--month", "2026-01"})
	require.Equal(t, 0, exitCode, stderr.String())
	return stdout.String()
}

func Test_run_summary_findings_count_a_classified_investment_account_and_an_ignored_id(t *testing.T) {
	home := newHome(t)
	duplicate, brokerage := syncSummaryFindingsFixture(t, home)

	before := summaryOf(t)
	writeConfig(t, home, fmt.Sprintf("[findings]\nignore = [%q]\n[accounts]\nnon-registered = [%q]\n", duplicate, brokerage))
	after := summaryOf(t)
	statusCode, status, statusErr := runCapture(context.Background(), []string{"status"})
	require.Equal(t, 0, statusCode, statusErr.String())

	assert.Contains(t, before, "\nFindings  5 open")
	assert.Contains(t, after, "\nFindings  3 open, 1 ignored")
	assert.Contains(t, status.String(), "\nFindings  3 open, 1 ignored")
}

func Test_run_summary_with_a_currency_warns_once_and_counts_every_finding_open_for_each_kind_of_bad_config(t *testing.T) {
	for _, c := range statusConfigRefusals() {
		t.Run(c.name, func(t *testing.T) {
			home := seedSummaryStore(t)
			c.setup(t, home)

			exitCode, stdout, stderr := runSpendCaptureAt(context.Background(), []string{"summary", "--currency", "CAD"}, summaryClock)

			require.Equal(t, 0, exitCode, stderr.String())
			assert.Equal(t, "quarry: warning: "+statusIgnoreWarningLead+c.problem+statusIgnoreWarningTail+"\n", stderr.String())
			assert.True(t, strings.HasPrefix(stdout.String(), "Summary of September 2026 (2026-09-01 to 2026-09-30), amounts in CAD\n"), stdout.String())
			assert.Contains(t, stdout.String(), "\nFindings  2 open; the last sync found 1 new and 1 fixed; run quarry findings to list them\n")
		})
	}
}

// runSummaryJSON runs quarry summary --json with args at now and returns its decoded document, stdout and stderr.
func runSummaryJSON(t *testing.T, now time.Time, args ...string) (summaryJSONDoc, string, string) {
	t.Helper()

	exitCode, stdout, stderr := runSpendCaptureAt(context.Background(), append([]string{"summary", "--json"}, args...), now)

	require.Equal(t, 0, exitCode, stderr.String())
	return decodeSummaryJSON(t, stdout.String()), stdout.String(), stderr.String()
}

// compactField is the value at path in the JSON object stdout, compacted, so two commands' copies compare byte for byte.
func compactField(t *testing.T, stdout string, path ...string) string {
	t.Helper()
	raw := json.RawMessage(stdout)
	for _, key := range path {
		var fields map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(raw, &fields))
		raw = fields[key]
	}
	var out bytes.Buffer

	require.NoError(t, json.Compact(&out, raw))
	return out.String()
}

// warningLine is the stderr line quarry prints for warning.
func warningLine(warning string) string { return "quarry: warning: " + warning + "\n" }

func Test_run_summary_json_writes_empty_arrays_and_nulls_for_the_first_month_of_data(t *testing.T) {
	home := newHome(t)
	replaceStore(t, home, firstMonthRows())

	doc, _, stderr := runSummaryJSON(t, summaryClock, "--month", "2026-09")

	assert.Equal(t, []anomalyJSON{}, doc.Anomalies.Charges)
	assert.Equal(t, []recurringSeriesJSON{}, doc.Recurring.Series)
	assert.Equal(t, []recurringTotalJSON{}, doc.Recurring.Totals)
	assert.Equal(t, summaryChangesJSON{Types: []summaryChangeTypeJSON{}, Totals: []summaryMoneyJSON{}}, doc.NetWorth.Changes)
	require.Len(t, doc.NetWorth.Dates, 2)
	assert.Equal(t, []summaryBalanceJSON{}, doc.NetWorth.Dates[0].Balances)
	assert.Equal(t, []summaryMoneyJSON{}, doc.NetWorth.Dates[0].Totals)
	assert.Len(t, doc.NetWorth.Dates[1].Balances, 1)
	assert.Equal(t, summarySnapshotJSON{ID: "20260929T000000Z"}, doc.Snapshot)
	assert.Equal(t, []string{
		"cannot tell whether the store holds all of September 2026: its snapshot's manifest does not record when it was taken; " +
			"open your Quicken file and run quarry sync to take a new snapshot",
	}, doc.Warnings)
	assert.Equal(t, septemberTimeUnknownWarning, stderr)
}

func Test_run_summary_json_writes_null_dates_for_a_store_without_transactions(t *testing.T) {
	home := newHome(t)
	replaceStore(t, home, chargeRows([]store.Account{chequingAccount("acct-cad", 1)}))

	doc, _, _ := runSummaryJSON(t, summaryClock)

	assert.Equal(t, summaryDatesJSON{}, doc.Dates)
	assert.Equal(t, summaryChangesJSON{Types: []summaryChangeTypeJSON{}, Totals: []summaryMoneyJSON{}}, doc.NetWorth.Changes)
}

func Test_run_summary_json_lists_each_currency_of_a_native_summary_separately(t *testing.T) {
	home := newHome(t)
	replaceStore(t, home, summaryNativeRows())

	doc, _, _ := runSummaryJSON(t, summaryClock, "--currency", "native")

	assert.Equal(t, "native", doc.Currency)
	assert.Equal(t, summaryChangesJSON{
		Types: []summaryChangeTypeJSON{
			{Type: "chequing", Currency: "CAD", Value: new("-412.00")},
			{Type: "credit_card", Currency: "CAD", Value: new("-22.59")},
			{Type: "chequing", Currency: "USD", Value: new("-159.99")},
		},
		Totals: []summaryMoneyJSON{
			{Currency: "CAD", Value: new("-434.59")},
			{Currency: "USD", Value: new("-159.99")},
		},
	}, doc.NetWorth.Changes)
}

func Test_run_summary_json_says_a_snapshot_taken_before_the_month_ended_does_not_cover_it(t *testing.T) {
	home := newHome(t)
	pinLocalZone(t)
	rows := summaryRows(true)
	rows.ImportRuns[0].Snapshot.TakenAt = time.Date(2026, time.September, 28, 14, 2, 0, 0, time.FixedZone("EDT", -4*60*60))
	replaceStore(t, home, rows)

	doc, _, stderr := runSummaryJSON(t, summaryClockEDT())

	assert.Equal(t, summarySnapshotJSON{ID: "20261001T130512Z", TakenAt: new("2026-09-28T18:02:00Z"), CoversMonth: new(false)}, doc.Snapshot)
	require.NotEmpty(t, doc.Warnings)
	assert.Equal(t, warningLine(doc.Warnings[len(doc.Warnings)-1]), stderr)
}

func Test_run_summary_json_names_the_config_by_its_absolute_path_while_stderr_shows_it_with_a_tilde(t *testing.T) {
	home := newHome(t)
	replaceStore(t, home, summaryRows(true))
	writeConfig(t, home, "[reporting]\ncurrency = \"EUR\"\n")

	doc, _, stderr := runSummaryJSON(t, summaryClock, "--currency", "CAD")

	assert.Equal(t, "CAD", doc.Currency)
	assert.Nil(t, doc.Findings.Ignored)
	require.NotEmpty(t, doc.Warnings)
	assert.Equal(t, "cannot tell which findings you ignored or how you classified your accounts: "+configPath(home)+
		": reporting.currency must be CAD, USD or native, got \"EUR\"; findings you ignored are counted as open, "+
		"and every investment account is counted as unclassified", doc.Warnings[0])
	assert.Equal(t, "quarry: warning: cannot tell which findings you ignored or how you classified your accounts: "+
		"~/Library/Application Support/quarry/config.toml: reporting.currency must be CAD, USD or native, got \"EUR\"; "+
		"findings you ignored are counted as open, and every investment account is counted as unclassified\n", stderr)
}

func Test_run_summary_json_names_a_config_warning_by_its_absolute_path_while_stderr_shows_it_with_a_tilde(t *testing.T) {
	home := newHome(t)
	replaceStore(t, home, summaryRows(true))
	writeConfig(t, home, "colour = \"red\"\n")

	doc, _, stderr := runSummaryJSON(t, summaryClock)

	require.NotEmpty(t, doc.Warnings)
	assert.Equal(t, configPath(home)+": unknown key colour; quarry ignores it", doc.Warnings[0])
	assert.Equal(t, "quarry: warning: ~/Library/Application Support/quarry/config.toml: unknown key colour; quarry ignores it\n", stderr)
}

func Test_run_summary_json_carries_the_charges_quarry_anomalies_lists_for_the_month(t *testing.T) {
	seedSummaryStore(t)
	pinLocalZone(t)
	_, summaryOut, _ := runSummaryJSON(t, summaryClock)

	exitCode, anomaliesOut, anomaliesErr := runSpendCaptureAt(context.Background(),
		[]string{"anomalies", "--json", "--since", "2026-09", "--until", "2026-09"}, summaryClock)

	require.Equal(t, 0, exitCode, anomaliesErr.String())
	assert.NotEqual(t, "[]", compactField(t, summaryOut, "anomalies", "charges"))
	assert.Equal(t, compactField(t, anomaliesOut.String(), "anomalies"), compactField(t, summaryOut, "anomalies", "charges"))
}

func Test_run_summary_json_carries_the_dates_quarry_networth_lists_for_the_two_month_ends(t *testing.T) {
	seedSummaryStore(t)
	pinLocalZone(t)
	_, summaryOut, _ := runSummaryJSON(t, summaryClock)

	exitCode, netWorthOut, netWorthErr := runSpendCaptureAt(context.Background(),
		[]string{"networth", "--json", "--since", "2026-08", "--until", "2026-09"}, summaryClock)

	require.Equal(t, 0, exitCode, netWorthErr.String())
	assert.Equal(t, compactField(t, netWorthOut.String(), "dates"), compactField(t, summaryOut, "net_worth", "dates"))
}

// summarySnapshotJSON is the document's "snapshot".
type summarySnapshotJSON struct {
	ID          string  `json:"id"`
	TakenAt     *string `json:"taken_at"`
	CoversMonth *bool   `json:"covers_month"`
}

// summaryDatesJSON is the document's "dates".
type summaryDatesJSON struct {
	First *string `json:"first"`
	Last  *string `json:"last"`
}

// summaryFindingsJSON is the document's "findings".
type summaryFindingsJSON struct {
	Open       int  `json:"open"`
	Ignored    *int `json:"ignored"`
	Fixed      int  `json:"fixed"`
	New        int  `json:"new"`
	NewlyFixed int  `json:"newly_fixed"`
}

// summaryAnomaliesJSON is the document's "anomalies".
type summaryAnomaliesJSON struct {
	Checked   int           `json:"checked"`
	NotJudged int           `json:"not_judged"`
	Charges   []anomalyJSON `json:"charges"`
}

// summaryRecurringJSON is the document's "recurring".
type summaryRecurringJSON struct {
	Series []recurringSeriesJSON `json:"series"`
	Totals []recurringTotalJSON  `json:"totals"`
}

// summaryBalanceJSON is one entry of a net-worth date's "balances".
type summaryBalanceJSON struct {
	Type             string  `json:"type"`
	Currency         string  `json:"currency"`
	Balance          string  `json:"balance"`
	ConvertedBalance *string `json:"converted_balance"`
}

// summaryMoneyJSON is one entry of "totals", in a net-worth date or in the changes.
type summaryMoneyJSON struct {
	Currency string  `json:"currency"`
	Value    *string `json:"value"`
}

// summaryNetWorthDateJSON is one entry of "net_worth.dates".
type summaryNetWorthDateJSON struct {
	Date     string               `json:"date"`
	Balances []summaryBalanceJSON `json:"balances"`
	Totals   []summaryMoneyJSON   `json:"totals"`
}

// summaryChangeTypeJSON is one entry of "net_worth.changes.types".
type summaryChangeTypeJSON struct {
	Type     string  `json:"type"`
	Currency string  `json:"currency"`
	Value    *string `json:"value"`
}

// summaryChangesJSON is "net_worth.changes".
type summaryChangesJSON struct {
	Types  []summaryChangeTypeJSON `json:"types"`
	Totals []summaryMoneyJSON      `json:"totals"`
}

// summaryNetWorthJSON is the document's "net_worth".
type summaryNetWorthJSON struct {
	Dates   []summaryNetWorthDateJSON `json:"dates"`
	Changes summaryChangesJSON        `json:"changes"`
}

// summaryJSONDoc is quarry summary --json's stdout.
type summaryJSONDoc struct {
	Month     string               `json:"month"`
	Since     string               `json:"since"`
	Until     string               `json:"until"`
	Currency  string               `json:"currency"`
	Snapshot  summarySnapshotJSON  `json:"snapshot"`
	Dates     summaryDatesJSON     `json:"dates"`
	Findings  summaryFindingsJSON  `json:"findings"`
	Anomalies summaryAnomaliesJSON `json:"anomalies"`
	Recurring summaryRecurringJSON `json:"recurring"`
	NetWorth  summaryNetWorthJSON  `json:"net_worth"`
	Warnings  []string             `json:"warnings"`
}

// decodeSummaryJSON reads stdout as the summary document, refusing keys the document does not rule.
func decodeSummaryJSON(t *testing.T, stdout string) summaryJSONDoc {
	t.Helper()
	decoder := json.NewDecoder(strings.NewReader(stdout))
	decoder.DisallowUnknownFields()
	var doc summaryJSONDoc

	require.NoError(t, decoder.Decode(&doc), stdout)
	return doc
}

func Test_run_summary_json_prints_the_ruled_document(t *testing.T) {
	seedSummaryStore(t)
	pinLocalZone(t)

	exitCode, stdout, stderr := runSpendCaptureAt(context.Background(), []string{"summary", "--json"}, summaryClock)

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, []string{
		"month", "since", "until", "currency", "snapshot", "dates", "findings", "anomalies", "recurring", "net_worth", "warnings",
	}, topLevelKeys(t, stdout.String()))
	doc := decodeSummaryJSON(t, stdout.String())
	assert.Equal(t, "2026-09", doc.Month)
	assert.Equal(t, "2026-09-01", doc.Since)
	assert.Equal(t, "2026-09-30", doc.Until)
	assert.Equal(t, "CAD", doc.Currency)
	assert.Equal(t, summarySnapshotJSON{ID: "20261001T130512Z", TakenAt: new("2026-10-01T13:05:12Z"), CoversMonth: new(true)}, doc.Snapshot)
	assert.Equal(t, summaryDatesJSON{First: new("2026-01-02"), Last: new("2026-09-15")}, doc.Dates)
	assert.Equal(t, summaryFindingsJSON{Open: 2, Ignored: new(0), Fixed: 1, New: 1, NewlyFixed: 1}, doc.Findings)
	assert.Equal(t, 2, doc.Anomalies.Checked)
	assert.Equal(t, 0, doc.Anomalies.NotJudged)
	require.Len(t, doc.Anomalies.Charges, 1)
	assert.Equal(t, new("Bell Canada"), doc.Anomalies.Charges[0].Payee)
	assert.Equal(t, "412.00", doc.Anomalies.Charges[0].Amount)
	require.Len(t, doc.Recurring.Series, 1)
	assert.Equal(t, "Crave", doc.Recurring.Series[0].Payee)
	assert.Equal(t, []recurringTotalJSON{{Currency: "CAD", PerYear: "271.08"}}, doc.Recurring.Totals)
	require.Len(t, doc.NetWorth.Dates, 2)
	assert.Equal(t, "2026-08-31", doc.NetWorth.Dates[0].Date)
	assert.Equal(t, "2026-09-30", doc.NetWorth.Dates[1].Date)
	assert.Equal(t, summaryChangesJSON{
		Types: []summaryChangeTypeJSON{
			{Type: "chequing", Currency: "CAD", Value: new("588.00")},
			{Type: "credit_card", Currency: "CAD", Value: new("-22.59")},
		},
		Totals: []summaryMoneyJSON{{Currency: "CAD", Value: new("565.41")}},
	}, doc.NetWorth.Changes)
	assert.Equal(t, []string{}, doc.Warnings)
}

const (
	summaryNotAMonthLine = "quarry: --month %q is not a month; use YYYY-MM, such as 2026-09\n"
	summaryNotEndedLine  = "quarry: --month %s has not ended; summary covers whole months, so pass 2026-09 or earlier\n"
)

func Test_run_summary_refuses_a_month_it_cannot_summarize(t *testing.T) {
	cases := []struct {
		name       string
		month      string
		wantStderr string
	}{
		{name: "a month without its leading zero", month: "2026-9", wantStderr: fmt.Sprintf(summaryNotAMonthLine, "2026-9")},
		{name: "a day", month: "2026-09-15", wantStderr: fmt.Sprintf(summaryNotAMonthLine, "2026-09-15")},
		{name: "year zero", month: "0000-01", wantStderr: fmt.Sprintf(summaryNotAMonthLine, "0000-01")},
		{name: "a month past twelve", month: "2026-13", wantStderr: fmt.Sprintf(summaryNotAMonthLine, "2026-13")},
		{name: "a year alone", month: "2026", wantStderr: fmt.Sprintf(summaryNotAMonthLine, "2026")},
		{name: "an empty value", month: "", wantStderr: fmt.Sprintf(summaryNotAMonthLine, "")},
		{name: "the current month", month: "2026-10", wantStderr: fmt.Sprintf(summaryNotEndedLine, "2026-10")},
		{name: "a later month", month: "2027-01", wantStderr: fmt.Sprintf(summaryNotEndedLine, "2027-01")},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())

			exitCode, stdout, stderr := runSpendCaptureAt(context.Background(), []string{"summary", "--month", c.month}, summaryClock)

			assert.Equal(t, 2, exitCode)
			assert.Empty(t, stdout.String())
			assert.Equal(t, c.wantStderr, stderr.String())
		})
	}
}

func Test_run_summary_accepts_a_month_that_has_ended(t *testing.T) {
	home := newHome(t)

	exitCode, _, stderr := runSpendCaptureAt(context.Background(), []string{"summary", "--month", "2026-09"}, summaryClock)

	assert.Equal(t, 1, exitCode)
	assert.Equal(t, "quarry: no store at "+abbreviated(t, storePathUnder(home), home)+" yet; run quarry sync to build it\n", stderr.String())
}

func Test_run_summary_checks_arguments_then_month_then_config_then_store(t *testing.T) {
	const malformedConfig = "[snapshots\nkeep = 24\n"
	cases := []struct {
		name       string
		config     string
		args       []string
		wantExit   int
		wantStderr string
	}{
		{
			name: "a bad currency before a bad month", args: []string{"summary", "--currency", "EUR", "--month", "2026-9"},
			config: malformedConfig, wantExit: 2, wantStderr: badCurrencyFlag,
		},
		{
			name: "a bad month before a malformed config", args: []string{"summary", "--month", "2026-10"},
			config: malformedConfig, wantExit: 2, wantStderr: fmt.Sprintf(summaryNotEndedLine, "2026-10"),
		},
		{
			name: "a missing month value before a malformed config", args: []string{"summary", "--month"},
			config: malformedConfig, wantExit: 2, wantStderr: "quarry: flag needs an argument: --month; Run 'quarry summary --help' for usage.\n",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := newHome(t)
			writeConfig(t, home, c.config)

			exitCode, stdout, stderr := runSpendCaptureAt(context.Background(), c.args, summaryClock)

			assert.Equal(t, c.wantExit, exitCode)
			assert.Empty(t, stdout.String())
			assert.Equal(t, c.wantStderr, stderr.String())
		})
	}
}

func Test_run_summary_reads_the_config_before_looking_for_a_store(t *testing.T) {
	home := newHome(t)
	writeConfig(t, home, "[snapshots\nkeep = 24\n")

	exitCode, stdout, stderr := runSpendCaptureAt(context.Background(), []string{"summary"}, summaryClock)

	require.Equal(t, 1, exitCode, stderr.String())
	assert.Empty(t, stdout.String())
	assert.Regexp(t, "^"+regexp.QuoteMeta("quarry: cannot read "+configShown+": line 1: ")+"[^\n]+"+regexp.QuoteMeta(configFix)+"\n$", stderr.String())
}

func Test_run_summary_with_a_currency_refuses_when_the_home_directory_is_unknown(t *testing.T) {
	t.Setenv("HOME", "")

	exitCode, stdout, stderr := runSpendCaptureAt(context.Background(), []string{"summary", "--currency", "USD"}, summaryClock)

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout.String())
	assert.Equal(t, "quarry: cannot find your home directory ($HOME is not set); set HOME, then run quarry summary again\n", stderr.String())
}

func Test_run_summary_with_a_currency_prints_only_the_missing_store_when_the_config_is_malformed(t *testing.T) {
	home := newHome(t)
	writeConfig(t, home, "[snapshots\nkeep = 24\n")

	exitCode, stdout, stderr := runSpendCaptureAt(context.Background(), []string{"summary", "--currency", "USD"}, summaryClock)

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout.String())
	assert.Equal(t, "quarry: no store at "+abbreviated(t, storePathUnder(home), home)+" yet; run quarry sync to build it\n", stderr.String())
}

func Test_run_summary_refuses_a_store_built_by_an_older_quarry(t *testing.T) {
	assertRefusesAnOlderStore(t, "summary")
}

// summaryClockEDT is October 6 noon in the pinned local zone: summaryClock is UTC, so it would print the
// warning's time in UTC while the Snapshot row prints EDT.
func summaryClockEDT() time.Time {
	return time.Date(2026, time.October, 6, 12, 0, 0, 0, time.FixedZone("EDT", -4*60*60))
}

// septemberTimeUnknownWarning is what a September summary says about a snapshot whose manifest records no time.
const septemberTimeUnknownWarning = "quarry: warning: cannot tell whether the store holds all of September 2026: " +
	"its snapshot's manifest does not record when it was taken; " +
	"open your Quicken file and run quarry sync to take a new snapshot\n"

// withSnapshotTaken is rows with the snapshot's manifest time set; chargeRows leaves it unrecorded.
func withSnapshotTaken(rows store.Rows, taken time.Time) store.Rows {
	rows.ImportRuns[0].Snapshot.TakenAt = taken
	return rows
}

func Test_run_summary_warns_when_the_snapshot_was_taken_before_the_month_ended(t *testing.T) {
	home := newHome(t)
	pinLocalZone(t)
	rows := summaryRows(true)
	rows.ImportRuns[0].Snapshot.TakenAt = time.Date(2026, time.September, 28, 14, 2, 0, 0, time.FixedZone("EDT", -4*60*60))
	replaceStore(t, home, rows)

	exitCode, stdout, stderr := runSpendCaptureAt(context.Background(), []string{"summary"}, summaryClockEDT())

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Contains(t, stdout.String(), "Summary of September 2026 (2026-09-01 to 2026-09-30), amounts in CAD\n\n"+
		"Snapshot  20261001T130512Z, taken 2026-09-28 14:02 EDT (7 days ago)\n")
	assert.Equal(t, "quarry: warning: the store was built from a snapshot taken 2026-09-28 14:02 EDT, before September 2026 ended, "+
		"so transactions from the rest of the month are missing; open your Quicken file, run quarry sync, "+
		"then run quarry summary again\n", stderr.String())
}

func Test_run_summary_warns_when_the_snapshot_records_no_time(t *testing.T) {
	home := newHome(t)
	pinLocalZone(t)
	rows := summaryRows(true)
	rows.ImportRuns[0].Snapshot.TakenAt = time.Time{}
	replaceStore(t, home, rows)

	exitCode, stdout, stderr := runSpendCaptureAt(context.Background(), []string{"summary"}, summaryClockEDT())

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Contains(t, stdout.String(), "Snapshot  20261001T130512Z, time taken not recorded in its manifest\n")
	assert.Equal(t, septemberTimeUnknownWarning, stderr.String())
}

// netWorthNoRatesLine is the net-worth warning for a store with no exchange rates, in a CAD summary.
const netWorthNoRatesLine = "the store has no exchange rates, so USD balances are not converted to CAD and are left out of the CAD total; " +
	"pass --currency native to list them, or run quarry sync to fetch rates"

func Test_run_summary_warns_once_per_kind_when_the_store_has_no_exchange_rates(t *testing.T) {
	home := newHome(t)
	replaceStore(t, home, summaryNativeRows())

	exitCode, stdout, stderr := runSpendCaptureAt(context.Background(), []string{"summary"}, summaryClock)

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Contains(t, stdout.String(), anomaliesTable("Unusually large charges 2026-09-01 to 2026-09-30 in all accounts, amounts in CAD", "4 charges checked",
		[]string{"2026-09-14", "US Chequing (USD)", "Hulu", "Food:Groceries", "USD 150.00", "USD 20.00", "7.5x", "payee, 5 earlier"},
		[]string{"2026-09-14", "Chequing (CAD)", "Bell Canada", "Food:Groceries", "412.00", "96.05", "4.3x", "payee, 5 earlier"}))
	assert.Contains(t, stdout.String(), recurringTable("Recurring charges new 2026-09-01 to 2026-09-30 in all accounts, amounts in CAD",
		[]string{"Crave", "CAD", "month", "22.59", "271.08", "2026-07-03", "2026-09-03", "active", ""},
		[]string{"Spotify", "USD", "month", "9.99", "119.88", "2026-07-08", "2026-09-08", "active", ""},
		[]string{"Total", "CAD", "", "", "271.08", "", "", "", ""},
		[]string{"Total", "USD", "", "", "119.88", "", "", "", ""}))
	assert.Contains(t, stdout.String(), netWorthHistoryLine("Change", "no rate", "-22.59", "no rate"))
	assert.Equal(t, septemberTimeUnknownWarning+warningLine(noRatesLine)+warningLine(netWorthNoRatesLine), stderr.String())
}

const (
	chargeBeforeFirstRateLine = "1 charge dated before 2026-10-02, the first exchange rate in the store, is listed in USD, not converted to CAD"
	seriesBeforeFirstRateLine = "1 series with a charge dated before 2026-10-02, the first exchange rate in the store, " +
		"is listed in USD, not converted to CAD"
	monthEndsBeforeFirstRateLine = "USD balances on 2 month ends before 2026-10-02, the first exchange rate in the store, " +
		"are not converted to CAD and are left out of the CAD total; pass --currency native to list them"
	colourWarningTilde = "~/Library/Application Support/quarry/config.toml: unknown key colour; quarry ignores it"
	unreadableConfigW2 = "cannot tell which findings you ignored or how you classified your accounts: " +
		"~/Library/Application Support/quarry/config.toml: reporting.currency must be CAD, USD or native, got \"EUR\"; " +
		"findings you ignored are counted as open, and every investment account is counted as unclassified"
)

// septemberTimeUnknownText is septemberTimeUnknownWarning as --json's warnings carry it: without the stderr prefix and newline.
func septemberTimeUnknownText() string {
	return strings.TrimSuffix(strings.TrimPrefix(septemberTimeUnknownWarning, "quarry: warning: "), "\n")
}

// runSummaryText runs quarry summary with args at summaryClock and returns its stdout and stderr.
func runSummaryText(t *testing.T, args ...string) (string, string) {
	t.Helper()

	exitCode, stdout, stderr := runSpendCaptureAt(context.Background(), append([]string{"summary"}, args...), summaryClock)

	require.Equal(t, 0, exitCode, stderr.String())
	return stdout.String(), stderr.String()
}

// seedNativeSummaryStore stores summaryNativeRows, which holds no exchange rates, under a fresh HOME.
func seedNativeSummaryStore(t *testing.T) string {
	t.Helper()
	home := newHome(t)
	replaceStore(t, home, summaryNativeRows())
	return home
}

// seedRatedNativeSummaryStore stores summaryNativeRows with a USD rate first dated October 2, after both month ends.
func seedRatedNativeSummaryStore(t *testing.T) {
	t.Helper()
	home := newHome(t)
	replaceStoreWithRates(t, home, summaryNativeRows(), usdRate(day(2026, time.October, 2), 1_350_000))
}

// usdBalanceOnlyRows holds one USD deposit in January: a USD balance on both month ends, no charge, no series.
func usdBalanceOnlyRows() store.Rows {
	usd := usdChequingAccount("acct-usd", 1)
	return chargeRows([]store.Account{usd}, chargeTxn{
		id: "usd-deposit", account: usd.ID, payee: "Employer", currency: "USD", day: day(2026, time.January, 2),
		splits: []chargeSplit{{category: "cat-fuel", cents: 100_000}},
	})
}

func Test_run_summary_json_lists_the_missing_rate_warnings_after_the_snapshot_warning_and_leaves_the_cells_null(t *testing.T) {
	seedNativeSummaryStore(t)

	doc, _, stderr := runSummaryJSON(t, summaryClock)

	assert.Equal(t, []string{septemberTimeUnknownText(), noRatesLine, netWorthNoRatesLine}, doc.Warnings)
	assert.Equal(t, []summaryChangeTypeJSON{
		{Type: "chequing", Currency: "CAD"},
		{Type: "credit_card", Currency: "CAD", Value: new("-22.59")},
	}, doc.NetWorth.Changes.Types)
	assert.Equal(t, []summaryMoneyJSON{{Currency: "CAD"}}, doc.NetWorth.Changes.Totals)
	assert.Equal(t, warningLines(doc.Warnings), stderr)
}

func Test_run_summary_prints_the_same_warnings_on_stderr_with_and_without_json(t *testing.T) {
	seedNativeSummaryStore(t)
	_, plain := runSummaryText(t)

	_, _, flagged := runSummaryJSON(t, summaryClock)

	assert.Equal(t, septemberTimeUnknownWarning+warningLine(noRatesLine)+warningLine(netWorthNoRatesLine), plain)
	assert.Equal(t, plain, flagged)
}

func Test_run_summary_warns_about_each_kind_dated_before_the_first_rate(t *testing.T) {
	seedRatedNativeSummaryStore(t)

	stdout, stderr := runSummaryText(t)

	assert.Contains(t, stdout, netWorthHistoryLine("Change", "no rate", "-22.59", "no rate"))
	assert.Equal(t, septemberTimeUnknownWarning+warningLine(chargeBeforeFirstRateLine)+warningLine(seriesBeforeFirstRateLine)+
		warningLine(monthEndsBeforeFirstRateLine), stderr)
}

func Test_run_summary_json_lists_each_kind_dated_before_the_first_rate_and_leaves_the_cells_null(t *testing.T) {
	seedRatedNativeSummaryStore(t)

	doc, _, stderr := runSummaryJSON(t, summaryClock)

	assert.Equal(t, []string{septemberTimeUnknownText(), chargeBeforeFirstRateLine, seriesBeforeFirstRateLine, monthEndsBeforeFirstRateLine}, doc.Warnings)
	assert.Equal(t, []summaryMoneyJSON{{Currency: "CAD"}}, doc.NetWorth.Changes.Totals)
	assert.Equal(t, warningLines(doc.Warnings), stderr)
}

func Test_run_summary_warns_only_about_the_month_ends_when_no_charge_or_series_needs_a_rate(t *testing.T) {
	home := newHome(t)
	replaceStoreWithRates(t, home, usdBalanceOnlyRows(), usdRate(day(2026, time.October, 2), 1_350_000))

	stdout, stderr := runSummaryText(t)

	assert.Contains(t, stdout, "No unusually large charges.")
	assert.Contains(t, stdout, "No new recurring charges.")
	assert.Contains(t, stdout, "Change       no rate  no rate\n")
	assert.Equal(t, septemberTimeUnknownWarning+warningLine(monthEndsBeforeFirstRateLine), stderr)
}

func Test_run_summary_json_lists_only_the_month_end_warning_when_no_charge_or_series_needs_a_rate(t *testing.T) {
	home := newHome(t)
	replaceStoreWithRates(t, home, usdBalanceOnlyRows(), usdRate(day(2026, time.October, 2), 1_350_000))

	doc, _, _ := runSummaryJSON(t, summaryClock)

	assert.Equal(t, []string{septemberTimeUnknownText(), monthEndsBeforeFirstRateLine}, doc.Warnings)
	assert.Equal(t, []summaryChangeTypeJSON{{Type: "chequing", Currency: "CAD"}}, doc.NetWorth.Changes.Types)
	assert.Equal(t, []summaryMoneyJSON{{Currency: "CAD"}}, doc.NetWorth.Changes.Totals)
}

func Test_run_summary_prints_a_config_warning_before_the_snapshot_and_rate_warnings(t *testing.T) {
	home := seedNativeSummaryStore(t)
	writeConfig(t, home, "colour = \"red\"\n")

	_, stderr := runSummaryText(t)

	assert.Equal(t, warningLine(colourWarningTilde)+septemberTimeUnknownWarning+warningLine(noRatesLine)+warningLine(netWorthNoRatesLine), stderr)
}

func Test_run_summary_json_lists_a_config_warning_before_the_snapshot_and_rate_warnings(t *testing.T) {
	home := seedNativeSummaryStore(t)
	writeConfig(t, home, "colour = \"red\"\n")

	doc, _, _ := runSummaryJSON(t, summaryClock)

	assert.Equal(t, []string{
		configPath(home) + ": unknown key colour; quarry ignores it", septemberTimeUnknownText(), noRatesLine, netWorthNoRatesLine,
	}, doc.Warnings)
}

func Test_run_summary_prints_the_cannot_tell_warning_before_the_snapshot_and_rate_warnings(t *testing.T) {
	home := seedNativeSummaryStore(t)
	writeConfig(t, home, "[reporting]\ncurrency = \"EUR\"\n")

	_, stderr := runSummaryText(t, "--currency", "CAD")

	assert.Equal(t, warningLine(unreadableConfigW2)+septemberTimeUnknownWarning+warningLine(noRatesLine)+warningLine(netWorthNoRatesLine), stderr)
}

func Test_run_summary_json_lists_the_cannot_tell_warning_before_the_snapshot_and_rate_warnings(t *testing.T) {
	home := seedNativeSummaryStore(t)
	writeConfig(t, home, "[reporting]\ncurrency = \"EUR\"\n")

	doc, _, _ := runSummaryJSON(t, summaryClock, "--currency", "CAD")

	require.Len(t, doc.Warnings, 4)
	assert.Equal(t, "cannot tell which findings you ignored or how you classified your accounts: "+configPath(home)+
		": reporting.currency must be CAD, USD or native, got \"EUR\"; findings you ignored are counted as open, "+
		"and every investment account is counted as unclassified", doc.Warnings[0])
	assert.Equal(t, []string{septemberTimeUnknownText(), noRatesLine, netWorthNoRatesLine}, doc.Warnings[1:])
}

func Test_run_summary_native_prints_no_rate_warning(t *testing.T) {
	seedNativeSummaryStore(t)

	_, stderr := runSummaryText(t, "--currency", "native")

	assert.Equal(t, septemberTimeUnknownWarning, stderr)
}

func Test_run_summary_json_native_lists_no_rate_warning(t *testing.T) {
	seedNativeSummaryStore(t)

	doc, _, _ := runSummaryJSON(t, summaryClock, "--currency", "native")

	assert.Equal(t, []string{septemberTimeUnknownText()}, doc.Warnings)
}

// siblingStderr is the stderr of quarry args at summaryClock.
func siblingStderr(t *testing.T, args ...string) string {
	t.Helper()

	exitCode, _, stderr := runSpendCaptureAt(context.Background(), args, summaryClock)

	require.Equal(t, 0, exitCode, stderr.String())
	return stderr.String()
}

func Test_run_summary_in_usd_prints_the_lines_anomalies_and_networth_print_for_the_same_store(t *testing.T) {
	seedNativeSummaryStore(t)
	anomalies := siblingStderr(t, "anomalies", "--since", "2026-09", "--until", "2026-09", "--currency", "USD")
	netWorth := siblingStderr(t, "networth", "--since", "2026-08", "--until", "2026-09", "--currency", "USD")
	require.Contains(t, anomalies, "listed in each account's own currency")
	require.Contains(t, netWorth, "CAD balances are not converted to USD")

	_, stderr := runSummaryText(t, "--currency", "USD")

	assert.Equal(t, septemberTimeUnknownWarning+anomalies+netWorth, stderr)
}

func Test_run_summary_json_in_usd_lists_the_lines_stderr_prints(t *testing.T) {
	seedNativeSummaryStore(t)

	doc, _, stderr := runSummaryJSON(t, summaryClock, "--currency", "USD")

	assert.Equal(t, "USD", doc.Currency)
	require.Len(t, doc.Warnings, 3)
	assert.Equal(t, warningLines(doc.Warnings), stderr)
}
