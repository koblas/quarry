// White-box helpers shared by the internal (package cli) test files: dates, zones and report fixtures.
package cli

import (
	"math/big"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/finding"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/report/document"
	"github.com/koblas/quarry/internal/store"
)

// utcDay is midnight UTC on the given date.
func utcDay(year int, month time.Month, day int) time.Time {
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

func sqlRow(texts ...string) []store.QueryValue {
	row := make([]store.QueryValue, len(texts))
	for i, text := range texts {
		row[i] = store.QueryValue{Text: text}
	}
	return row
}

// anomalyOf is a payee-baseline anomaly of the given payee, category and splits: 412.00 against a
// usual 96.05, 4.3 times, from 38 earlier charges, on 2026-03-02 in Chequing (CAD).
func anomalyOf(payee *string, category *store.ChargeCategory, splits int) report.Anomaly {
	return report.Anomaly{
		Date:    time.Date(2026, time.March, 2, 0, 0, 0, 0, time.UTC),
		Account: store.Account{ID: "acct-1", Name: "Chequing", Currency: "CAD", Active: true},
		Payee:   payee, Currency: "CAD", Amount: 41200, Category: category, ExpenseSplits: splits,
		Baseline: report.BaselinePayee, Usual: 9605, Earlier: 38, TimesTenths: 43,
	}
}

func listed(anomalies ...report.Anomaly) report.Anomalies {
	return report.Anomalies{Window: spendingWindow(), Listed: anomalies, Checked: len(anomalies)}
}

func statusFixture() store.Status {
	return store.Status{
		Path: "/Users/dave/Library/Application Support/quarry/quarry.duckdb",
		Run: store.ImportRun{
			Snapshot: store.SnapshotRef{
				Path:    "/Users/dave/Library/Application Support/quarry/snapshots/20260927T143005Z.sqlite",
				TakenAt: time.Date(2026, 9, 27, 14, 30, 5, 0, time.UTC),
				Source:  "/Users/dave/Documents/Home.quicken",
			},
			Counts: store.Counts{
				Transactions: 18204, Splits: 21977, Transfers: 3141, Payees: 1873, Categories: 312, Tags: 14,
				InvestmentTransactions: 1605, Securities: 84, Prices: 99352,
			},
			BalancesChecked: 35, BalancesNeverReconciled: 3, InvestmentAccounts: 4,
			TransfersPaired: 3112, TransfersOneSided: 29,
			SharesChecked: 7,
		},
		FirstDate: time.Date(2003, 1, 4, 0, 0, 0, 0, time.UTC),
		LastDate:  time.Date(2026, 9, 26, 0, 0, 0, 0, time.UTC),
		Rates: store.StatusRates{
			First: time.Date(2003, 1, 4, 0, 0, 0, 0, time.UTC),
			Last:  time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC),
		},
	}
}

func findingsFixture() document.FindingsTally {
	return document.FindingsTally{Counts: finding.Counts{Open: 12, Ignored: 4}, IgnoreKnown: true}
}

// useZone makes zone the process-local zone for the test.
func useZone(tb testing.TB, zone *time.Location) {
	tb.Helper()
	//nolint:gosmopolitan // the test swaps the process-local zone to pin the local rendering; Cleanup restores it
	previous := time.Local
	time.Local = zone                            //nolint:gosmopolitan // restored by Cleanup
	tb.Cleanup(func() { time.Local = previous }) //nolint:gosmopolitan // restores the zone swapped above
}

func cents(text string) *big.Int {
	n, _ := new(big.Int).SetString(text, 10)
	return n
}

// spendWindow is January 1 through September 29, 2026, the window the JSON document tests report over.
func spendWindow() store.Window {
	return store.Window{
		Since: time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC),
		Until: time.Date(2026, time.September, 29, 0, 0, 0, 0, time.UTC),
	}
}

func spendingWindow() store.Window {
	return store.Window{
		Since: time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC),
		Until: time.Date(2026, time.March, 9, 0, 0, 0, 0, time.UTC),
	}
}
