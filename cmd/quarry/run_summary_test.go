// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/snapshot"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// summaryTakenAt is when the summary fixture's last build read its snapshot: after September ended.
var summaryTakenAt = time.Date(2026, time.October, 1, 13, 5, 12, 0, time.UTC)

// summaryClock is the instant the summary tests run at: October 6, so the default month is September.
var summaryClock = time.Date(2026, time.October, 6, 12, 0, 0, 0, time.UTC)

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

// summaryRows is the store of September 2026 the summary tests read, as the sync that finds the findings builds it.
//
// Chequing holds a 5,000.00 deposit on January 2, five Bell Canada charges (90.00 to 102.00, 480.05 in all) a
// month and a half apart, a 412.00 Bell Canada charge on September 14 and a 1,000.00 deposit on September 15.
// Card holds Crave's 22.59 on the 3rd of July, August and September. Shell (10.00), Kiosk (11.00) and
// Pharmacy (12.00) are charges on August 10 and 11, of different amounts so none is a duplicate of another:
// uncategorized except Kiosk's when kioskFixed, and Pharmacy's exists only when kioskFixed.
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
// finds Pharmacy's while Shell's carries, and returns the HOME.
func seedSummaryStore(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
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
		return nil, errors.New("summary must not build a sync server")
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
