package main

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
	var summary, anomalies, stderr bytes.Buffer

	require.Equal(t, 0, runWith(context.Background(), []string{"summary"}, spendEnvAt(&summary, &stderr, summaryClock)), stderr.String())
	require.Equal(t, 0, runWith(context.Background(), []string{"anomalies", "--since", "2026-09", "--until", "2026-09"},
		spendEnvAt(&anomalies, &stderr, summaryClock)), stderr.String())

	assert.Contains(t, summary.String(), "\n"+anomalies.String()+"\n")
}
