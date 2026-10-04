// White-box: snapshotAge, snapshotTakenPhrase and renderStatus are unexported
// formatting rules whose bucket edges and NULL forms are best driven directly.
package cli

import (
	"testing"
	"time"

	"github.com/koblas/quarry/internal/finding"
	"github.com/koblas/quarry/internal/report/document"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
)

// useZone makes zone the process-local zone for the test.
func useZone(t *testing.T, zone *time.Location) {
	t.Helper()
	//nolint:gosmopolitan // the test swaps the process-local zone to pin the local rendering; Cleanup restores it
	previous := time.Local
	time.Local = zone                           //nolint:gosmopolitan // restored by Cleanup
	t.Cleanup(func() { time.Local = previous }) //nolint:gosmopolitan // restores the zone swapped above
}

func Test_snapshotAge(t *testing.T) {
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		age  time.Duration
		want string
	}{
		{name: "taken this instant", age: 0, want: "just now"},
		{name: "one second short of a minute", age: 59 * time.Second, want: "just now"},
		{name: "exactly a minute, singular", age: time.Minute, want: "1 minute ago"},
		{name: "minutes round down", age: 2*time.Minute + 59*time.Second, want: "2 minutes ago"},
		{name: "one second short of an hour", age: time.Hour - time.Second, want: "59 minutes ago"},
		{name: "exactly an hour, singular", age: time.Hour, want: "1 hour ago"},
		{name: "one minute short of 48 hours", age: 48*time.Hour - time.Minute, want: "47 hours ago"},
		{name: "exactly 48 hours is days", age: 48 * time.Hour, want: "2 days ago"},
		{name: "days round down", age: 3*24*time.Hour + 23*time.Hour, want: "3 days ago"},
		{name: "taken ahead of the clock", age: -5 * time.Minute, want: "just now"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, snapshotAge(now, now.Add(-c.age)))
		})
	}
}

func Test_snapshotTakenPhrase(t *testing.T) {
	useZone(t, time.FixedZone("EDT", -4*60*60))
	takenAt := time.Date(2026, 9, 27, 14, 30, 5, 0, time.UTC)

	got := snapshotTakenPhrase(takenAt.Add(48*time.Hour), takenAt)

	assert.Equal(t, "2026-09-27 10:30 EDT (2 days ago)", got)
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

func Test_renderStatus(t *testing.T) {
	useZone(t, time.FixedZone("EDT", -4*60*60))
	home := "/Users/dave"
	now := time.Date(2026, 9, 29, 14, 30, 5, 0, time.UTC)

	t.Run("full block", func(t *testing.T) {
		want := "Store     ~/Library/Application Support/quarry/quarry.duckdb\n" +
			"Snapshot  20260927T143005Z, taken 2026-09-27 10:30 EDT (2 days ago)\n" +
			"Source    ~/Documents/Home.quicken\n" +
			"Dates     2003-01-04 to 2026-09-26\n" +
			"Rows      18,204 transactions, 21,977 splits, 3,141 transfers, 1,873 payees, 312 categories, 14 tags; " +
			"1,605 investment transactions, 84 securities, 99,352 prices\n" +
			"Balances  35 accounts match Quicken's last reconciled balance; 3 never reconciled and 4 investment accounts not checked\n" +
			"Splits    all 18,204 transactions equal the sum of their splits\n" +
			"Shares    7 holdings match Quicken's share counts\n" +
			"Transfers 3,112 paired, 29 one-sided\n" +
			"Findings  12 open, 4 ignored; run quarry findings to list them\n" +
			"Rates     USD/CAD from the Bank of Canada, 2003-01-04 to 2026-09-28 (1 day ago)\n"

		assert.Equal(t, want, renderStatus(statusFixture(), findingsFixture(), home, now))
	})

	t.Run("no transactions", func(t *testing.T) {
		st := statusFixture()
		st.FirstDate, st.LastDate = time.Time{}, time.Time{}
		st.Run.Counts = store.Counts{}

		got := renderStatus(st, findingsFixture(), home, now)

		assert.Contains(t, got, "Dates     no transactions\n")
		assert.Contains(t, got, "Splits    no transactions to check\n")
	})

	t.Run("the Shares line follows the share check count", func(t *testing.T) {
		cases := []struct {
			name    string
			checked int
			want    string
		}{
			{name: "no holdings", checked: 0, want: "Shares    no holdings to check\n"},
			{name: "one holding, singular", checked: 1, want: "Shares    1 holding matches Quicken's share count\n"},
			{name: "many holdings, thousands-grouped", checked: 1234, want: "Shares    1,234 holdings match Quicken's share counts\n"},
		}

		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				st := statusFixture()
				st.Run.SharesChecked = c.checked

				assert.Contains(t, renderStatus(st, findingsFixture(), home, now), c.want)
			})
		}
	})

	t.Run("no one-sided transfers", func(t *testing.T) {
		st := statusFixture()
		st.Run.TransfersOneSided = 0

		assert.Contains(t, renderStatus(st, findingsFixture(), home, now), "Transfers 3,112 paired\n")
	})

	t.Run("no transfers at all", func(t *testing.T) {
		st := statusFixture()
		st.Run.TransfersPaired, st.Run.TransfersOneSided = 0, 0

		assert.Contains(t, renderStatus(st, findingsFixture(), home, now), "Transfers none\n")
	})

	t.Run("snapshot time not recorded", func(t *testing.T) {
		st := statusFixture()
		st.Run.Snapshot.TakenAt = time.Time{}

		got := renderStatus(st, findingsFixture(), home, now)

		assert.Contains(t, got, "Snapshot  20260927T143005Z, time taken not recorded in its manifest\n")
	})

	t.Run("source not recorded", func(t *testing.T) {
		st := statusFixture()
		st.Run.Snapshot.Source = ""

		got := renderStatus(st, findingsFixture(), home, now)

		assert.Contains(t, got, "Source    not recorded in the snapshot's manifest\n")
	})

	t.Run("whitespace-only source prints as it is", func(t *testing.T) {
		st := statusFixture()
		st.Run.Snapshot.Source = " "

		assert.Contains(t, renderStatus(st, findingsFixture(), home, now), "Source     \n")
	})

	t.Run("a store dated west of UTC keeps its calendar days", func(t *testing.T) {
		useZone(t, time.FixedZone("PST", -8*60*60))

		assert.Contains(t, renderStatus(statusFixture(), findingsFixture(), home, now), "Dates     2003-01-04 to 2026-09-26\n")
	})

	t.Run("findings row forms", func(t *testing.T) {
		cases := []struct {
			name     string
			findings document.FindingsTally
			want     string
		}{
			{
				name: "open and ignored", findings: document.FindingsTally{Counts: finding.Counts{Open: 12, Ignored: 4}, IgnoreKnown: true},
				want: "Findings  12 open, 4 ignored; run quarry findings to list them\n",
			},
			{
				name: "none open", findings: document.FindingsTally{IgnoreKnown: true},
				want: "Findings  none open\n",
			},
			{
				name: "none open, some ignored", findings: document.FindingsTally{Counts: finding.Counts{Ignored: 4}, IgnoreKnown: true},
				want: "Findings  none open, 4 ignored\n",
			},
			{
				name: "counts are thousands-grouped", findings: document.FindingsTally{Counts: finding.Counts{Open: 1234}, IgnoreKnown: true},
				want: "Findings  1,234 open; run quarry findings to list them\n",
			},
			{
				name: "new and newly fixed findings add no clause", findings: document.FindingsTally{Counts: finding.Counts{Open: 3, New: 3, NewlyFixed: 2}, IgnoreKnown: true},
				want: "Findings  3 open; run quarry findings to list them\n",
			},
			{
				name: "an unreadable ignore list drops the ignored clause", findings: document.FindingsTally{Counts: finding.Counts{Open: 4, Ignored: 1}},
				want: "Findings  4 open; run quarry findings to list them\n",
			},
		}

		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				assert.Contains(t, renderStatus(statusFixture(), c.findings, home, now), c.want)
			})
		}
	})
}
