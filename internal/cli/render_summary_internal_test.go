// White-box: summaryHeading, summaryFindingsPhrase and renderSummary are unexported layout rules; the ignored
// clause of an unread ignore list cannot be reached through the command, which counts no ignored finding then.
package cli

import (
	"testing"
	"time"

	"github.com/koblas/quarry/internal/finding"
	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/report/document"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
)

// septemberSummary is an empty summary of September 2026 in currency.
func septemberSummary(currency money.Currency) report.Summary {
	start := time.Date(2026, time.September, 1, 0, 0, 0, 0, time.UTC)
	return report.Summary{
		Month:    report.Month{Start: start, End: time.Date(2026, time.September, 30, 0, 0, 0, 0, time.UTC)},
		Currency: currency,
		NetWorth: report.NetWorth{Currency: currency},
	}
}

func Test_summaryHeading_names_the_currency_unless_amounts_stay_native(t *testing.T) {
	cases := []struct {
		name     string
		currency money.Currency
		want     string
	}{
		{name: "CAD", currency: money.CAD, want: "Summary of September 2026 (2026-09-01 to 2026-09-30), amounts in CAD"},
		{name: "USD", currency: money.USD, want: "Summary of September 2026 (2026-09-01 to 2026-09-30), amounts in USD"},
		{name: "native adds no currency", currency: money.Native, want: "Summary of September 2026 (2026-09-01 to 2026-09-30)"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, summaryHeading(septemberSummary(c.currency)))
		})
	}
}

func Test_summaryFindingsPhrase_says_ignored_only_when_the_ignore_list_was_read(t *testing.T) {
	cases := []struct {
		name        string
		ignoreKnown bool
		want        string
	}{
		{name: "ignore list read", ignoreKnown: true, want: "3 open, 2 ignored; run quarry findings to list them"},
		{name: "ignore list unread", ignoreKnown: false, want: "3 open; run quarry findings to list them"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tally := document.FindingsTally{Counts: finding.Counts{Open: 3, Ignored: 2}, IgnoreKnown: c.ignoreKnown}

			assert.Equal(t, c.want, summaryFindingsPhrase(tally))
		})
	}
}

func Test_renderSummary_says_when_the_snapshot_manifest_has_no_time(t *testing.T) {
	s := septemberSummary(money.CAD)
	s.Status.Run.Snapshot = store.SnapshotRef{Path: "/snapshots/20260928T140200Z.sqlite"}

	got := renderSummary(s, document.FindingsTally{IgnoreKnown: true}, time.Date(2026, time.October, 6, 12, 0, 0, 0, time.UTC))

	assert.Contains(t, got, "\nSnapshot  20260928T140200Z, time taken not recorded in its manifest\n")
}

func Test_renderSummary_says_when_the_store_holds_no_transactions(t *testing.T) {
	s := septemberSummary(money.CAD)

	got := renderSummary(s, document.FindingsTally{IgnoreKnown: true}, time.Date(2026, time.October, 6, 12, 0, 0, 0, time.UTC))

	assert.Contains(t, got, "\nDates     no transactions\nFindings  none open\n\n")
}
