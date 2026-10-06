package document_test

import (
	"encoding/json"
	"math/big"
	"strconv"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/finding"
	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/report/document"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// summaryDocument is the document NewSummary builds, read back as a map, so a null and an absent key differ.
func summaryDocument(t *testing.T, s report.Summary, tally document.FindingsTally, warnings []string) map[string]any {
	t.Helper()
	var got map[string]any

	require.NoError(t, json.Unmarshal([]byte(indented(t, document.NewSummary(s, tally, warnings))), &got))
	return got
}

// object is the JSON object at path in doc.
func object(t *testing.T, doc map[string]any, path ...string) map[string]any {
	t.Helper()
	for _, key := range path {
		next, ok := doc[key].(map[string]any)
		require.True(t, ok, "%s is not an object", key)
		doc = next
	}
	return doc
}

// keysAt is the keys of the object at path in the NewSummary document, in document order; a numeric path step
// is an array index.
func keysAt(t *testing.T, s report.Summary, path ...string) []string {
	t.Helper()
	raw := []byte(indented(t, document.NewSummary(s, document.FindingsTally{}, nil)))
	for _, key := range path {
		if index, err := strconv.Atoi(key); err == nil {
			var elements []json.RawMessage
			require.NoError(t, json.Unmarshal(raw, &elements))
			raw = elements[index]
			continue
		}
		var fields map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(raw, &fields))
		raw = fields[key]
	}
	return topLevelKeys(t, raw)
}

// septemberSummary holds one of every section's entries, so every object of the document is written.
func septemberSummary() report.Summary {
	payee := "Bell Canada"
	perYear := int64(27_108)
	return report.Summary{
		Month:    report.Month{Start: civil(2026, time.September, 1), End: civil(2026, time.September, 30)},
		Currency: money.CAD,
		Status: store.Status{
			Run:       store.ImportRun{Snapshot: store.SnapshotRef{Path: "/snaps/20261001T130512Z.sqlite", TakenAt: time.Date(2026, time.October, 1, 13, 5, 12, 0, time.UTC)}},
			FirstDate: civil(2003, time.January, 2), LastDate: civil(2026, time.September, 30),
		},
		Anomalies: report.Anomalies{
			Checked: 412, NotJudged: 37,
			Listed: []report.Anomaly{{Payee: &payee, Amount: 41_200, Usual: 21_250}},
		},
		Recurring: report.Recurring{
			Series: []report.Series{{Payee: "Crave", Currency: "CAD", PerYear: &perYear}},
			Totals: []report.RecurringTotal{{Currency: "CAD", PerYear: 27_108}},
		},
		NetWorth: report.NetWorth{Dates: []report.NetWorthDate{{Date: civil(2026, time.August, 31)}, {Date: civil(2026, time.September, 30)}}},
		Change: &report.NetWorthChange{
			Types:  []report.NetWorthChangeType{{Type: "chequing", Currency: "CAD", Value: big.NewInt(65_433)}},
			Totals: []report.NetWorthChangeTotal{{Currency: "CAD", Value: big.NewInt(618_643)}},
		},
	}
}

func Test_NewSummary_writes_every_object_with_its_keys_in_the_ruled_order(t *testing.T) {
	cases := []struct {
		name string
		path []string
		want []string
	}{
		{name: "the document", want: []string{"month", "since", "until", "currency", "snapshot", "dates", "findings", "anomalies", "recurring", "net_worth", "warnings"}},
		{name: "snapshot", path: []string{"snapshot"}, want: []string{"id", "taken_at", "covers_month"}},
		{name: "dates", path: []string{"dates"}, want: []string{"first", "last"}},
		{name: "findings", path: []string{"findings"}, want: []string{"open", "ignored", "fixed", "new", "newly_fixed"}},
		{name: "anomalies", path: []string{"anomalies"}, want: []string{"checked", "not_judged", "charges"}},
		{name: "recurring", path: []string{"recurring"}, want: []string{"series", "totals"}},
		{name: "net worth", path: []string{"net_worth"}, want: []string{"dates", "changes"}},
		{name: "changes", path: []string{"net_worth", "changes"}, want: []string{"types", "totals"}},
		{name: "a change type", path: []string{"net_worth", "changes", "types", "0"}, want: []string{"type", "currency", "value"}},
		{name: "a change total", path: []string{"net_worth", "changes", "totals", "0"}, want: []string{"currency", "value"}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, keysAt(t, septemberSummary(), c.path...))
		})
	}
}

func Test_NewSummary_writes_the_month_and_its_first_and_last_day(t *testing.T) {
	got := summaryDocument(t, septemberSummary(), document.FindingsTally{}, nil)

	assert.Equal(t, "2026-09", got["month"])
	assert.Equal(t, "2026-09-01", got["since"])
	assert.Equal(t, "2026-09-30", got["until"])
	assert.Equal(t, "CAD", got["currency"])
}

func Test_NewSummary_writes_native_for_a_native_summary(t *testing.T) {
	s := septemberSummary()
	s.Currency = money.Native

	got := summaryDocument(t, s, document.FindingsTally{}, nil)

	assert.Equal(t, "native", got["currency"])
}

func Test_NewSummary_writes_the_snapshot_id_and_the_time_it_was_taken(t *testing.T) {
	s := septemberSummary()
	s.Coverage = report.SnapshotCovers

	got := summaryDocument(t, s, document.FindingsTally{}, nil)

	assert.Equal(t, map[string]any{"id": "20261001T130512Z", "taken_at": "2026-10-01T13:05:12Z", "covers_month": true}, got["snapshot"])
}

func Test_NewSummary_says_whether_the_snapshot_covers_the_month_and_leaves_it_null_when_the_time_is_unknown(t *testing.T) {
	cases := []struct {
		name        string
		coverage    report.SnapshotCoverage
		taken       time.Time
		wantCovers  any
		wantTakenAt any
	}{
		{name: "covers", coverage: report.SnapshotCovers, taken: civil(2026, time.October, 1), wantCovers: true, wantTakenAt: "2026-10-01T00:00:00Z"},
		{name: "predates the month end", coverage: report.SnapshotPredatesMonthEnd, taken: civil(2026, time.September, 28), wantCovers: false, wantTakenAt: "2026-09-28T00:00:00Z"},
		{name: "time unknown", coverage: report.SnapshotTimeUnknown, wantCovers: nil, wantTakenAt: nil},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := septemberSummary()
			s.Coverage = c.coverage
			s.Status.Run.Snapshot.TakenAt = c.taken

			snapshot := object(t, summaryDocument(t, s, document.FindingsTally{}, nil), "snapshot")

			assert.Equal(t, c.wantCovers, snapshot["covers_month"])
			assert.Equal(t, c.wantTakenAt, snapshot["taken_at"])
		})
	}
}

func Test_NewSummary_writes_a_coverage_the_report_does_not_define_as_null(t *testing.T) {
	s := septemberSummary()
	s.Coverage = report.SnapshotCoverage(99)

	snapshot := object(t, summaryDocument(t, s, document.FindingsTally{}, nil), "snapshot")

	assert.Nil(t, snapshot["covers_month"])
}

func Test_NewSummary_writes_the_findings_counts_with_ignored_null_only_when_the_ignore_list_was_unreadable(t *testing.T) {
	counts := finding.Counts{Open: 14, Ignored: 5, Fixed: 1, New: 3, NewlyFixed: 2}
	cases := []struct {
		name  string
		tally document.FindingsTally
		want  any
	}{
		{name: "ignore list read, five ignored", tally: document.FindingsTally{Counts: counts, IgnoreKnown: true}, want: float64(5)},
		{name: "ignore list read, none ignored", tally: document.FindingsTally{Counts: finding.Counts{}, IgnoreKnown: true}, want: float64(0)},
		{name: "ignore list unreadable", tally: document.FindingsTally{Counts: counts}, want: nil},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			findings := object(t, summaryDocument(t, septemberSummary(), c.tally, nil), "findings")

			assert.Equal(t, c.want, findings["ignored"])
		})
	}
}

func Test_NewSummary_writes_the_other_findings_counts_unchanged_when_the_ignore_list_is_unreadable(t *testing.T) {
	tally := document.FindingsTally{Counts: finding.Counts{Open: 14, Ignored: 5, Fixed: 1, New: 3, NewlyFixed: 2}}

	findings := object(t, summaryDocument(t, septemberSummary(), tally, nil), "findings")

	assert.Equal(t, map[string]any{"open": float64(14), "ignored": nil, "fixed": float64(1), "new": float64(3), "newly_fixed": float64(2)}, findings)
}

func Test_NewSummary_writes_the_first_and_last_transaction_days(t *testing.T) {
	got := summaryDocument(t, septemberSummary(), document.FindingsTally{}, nil)

	assert.Equal(t, map[string]any{"first": "2003-01-02", "last": "2026-09-30"}, got["dates"])
}

func Test_NewSummary_writes_null_dates_for_a_store_with_no_transactions(t *testing.T) {
	s := septemberSummary()
	s.Status.FirstDate, s.Status.LastDate = time.Time{}, time.Time{}

	got := summaryDocument(t, s, document.FindingsTally{}, nil)

	assert.Equal(t, map[string]any{"first": nil, "last": nil}, got["dates"])
}

func Test_NewSummary_writes_every_array_empty_not_null_for_a_summary_with_nothing_in_it(t *testing.T) {
	s := report.Summary{NetWorth: report.NetWorth{Dates: []report.NetWorthDate{{Date: civil(2026, time.August, 31)}}}}

	got := summaryDocument(t, s, document.FindingsTally{}, nil)

	assert.Equal(t, []any{}, object(t, got, "anomalies")["charges"])
	assert.Equal(t, []any{}, object(t, got, "recurring")["series"])
	assert.Equal(t, []any{}, object(t, got, "recurring")["totals"])
	assert.Equal(t, []any{map[string]any{"date": "2026-08-31", "balances": []any{}, "totals": []any{}}}, object(t, got, "net_worth")["dates"])
	assert.Equal(t, []any{}, got["warnings"])
}

func Test_NewSummary_writes_net_worth_dates_with_no_entries_as_an_empty_array(t *testing.T) {
	got := summaryDocument(t, report.Summary{}, document.FindingsTally{}, nil)

	assert.Equal(t, []any{}, object(t, got, "net_worth")["dates"])
}

func Test_NewSummary_writes_the_warnings_it_is_given_in_order(t *testing.T) {
	got := summaryDocument(t, septemberSummary(), document.FindingsTally{}, []string{"first", "second"})

	assert.Equal(t, []any{"first", "second"}, got["warnings"])
}

func Test_NewSummary_writes_the_anomaly_and_series_entries_as_their_own_commands_do(t *testing.T) {
	got := summaryDocument(t, septemberSummary(), document.FindingsTally{}, nil)

	summary := document.NewSummary(septemberSummary(), document.FindingsTally{}, nil)
	assert.Equal(t, 412, summary.Anomalies.Checked)
	assert.Equal(t, 37, summary.Anomalies.NotJudged)
	anomalies := object(t, got, "anomalies")
	charges, ok := anomalies["charges"].([]any)
	require.True(t, ok)
	require.Len(t, charges, 1)
	charge, ok := charges[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "Bell Canada", charge["payee"])
	assert.Equal(t, "412.00", charge["amount"])
	assert.Equal(t, "212.50", charge["usual"])
	recurring := object(t, got, "recurring")
	series, ok := recurring["series"].([]any)
	require.True(t, ok)
	require.Len(t, series, 1)
	assert.Equal(t, "Crave", object(t, map[string]any{"s": series[0]}, "s")["payee"])
	assert.Equal(t, []any{map[string]any{"currency": "CAD", "per_year": "271.08"}}, recurring["totals"])
}

func Test_NewSummary_writes_a_charge_with_no_payee_as_null_and_an_empty_payee_as_empty(t *testing.T) {
	empty := ""
	cases := []struct {
		name  string
		payee *string
		want  any
	}{
		{name: "no payee", payee: nil, want: nil},
		{name: "empty payee", payee: &empty, want: ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := septemberSummary()
			s.Anomalies.Listed = []report.Anomaly{{Payee: c.payee}}

			charges, ok := object(t, summaryDocument(t, s, document.FindingsTally{}, nil), "anomalies")["charges"].([]any)

			require.True(t, ok)
			assert.Equal(t, c.want, charges[0].(map[string]any)["payee"]) //nolint:forcetypeassert // the document's own entry
		})
	}
}

func Test_NewSummary_carries_a_payee_with_a_newline_and_an_escape_character_raw(t *testing.T) {
	payee := "Bell\nCanada\x1b[31m"
	s := septemberSummary()
	s.Anomalies.Listed = []report.Anomaly{{Payee: &payee}}

	charges, ok := object(t, summaryDocument(t, s, document.FindingsTally{}, nil), "anomalies")["charges"].([]any)

	require.True(t, ok)
	assert.Equal(t, payee, charges[0].(map[string]any)["payee"]) //nolint:forcetypeassert // the document's own entry
}

func Test_NewSummary_writes_a_converted_change_as_one_total_and_every_type_in_order(t *testing.T) {
	s := septemberSummary()
	s.Change = &report.NetWorthChange{
		Types: []report.NetWorthChangeType{
			{Type: "chequing", Currency: "CAD", Value: big.NewInt(65_433)},
			{Type: "credit_card", Currency: "CAD", Value: big.NewInt(-21_960)},
			{Type: "brokerage", Currency: "CAD", Value: big.NewInt(123_456_789_012_300)},
		},
		Totals: []report.NetWorthChangeTotal{{Currency: "CAD", Value: big.NewInt(618_643)}},
	}

	changes := object(t, summaryDocument(t, s, document.FindingsTally{}, nil), "net_worth", "changes")

	assert.Equal(t, []any{
		map[string]any{"type": "chequing", "currency": "CAD", "value": "654.33"},
		map[string]any{"type": "credit_card", "currency": "CAD", "value": "-219.60"},
		map[string]any{"type": "brokerage", "currency": "CAD", "value": "1234567890123.00"},
	}, changes["types"])
	assert.Equal(t, []any{map[string]any{"currency": "CAD", "value": "6186.43"}}, changes["totals"])
}

func Test_NewSummary_writes_a_native_change_per_currency(t *testing.T) {
	s := septemberSummary()
	s.Currency = money.Native
	s.Change = &report.NetWorthChange{
		Types: []report.NetWorthChangeType{
			{Type: "chequing", Currency: "CAD", Value: big.NewInt(10_000)},
			{Type: "chequing", Currency: "USD", Value: big.NewInt(20_000)},
			{Type: "brokerage", Currency: "USD", Value: big.NewInt(30_000)},
		},
		Totals: []report.NetWorthChangeTotal{
			{Currency: "CAD", Value: big.NewInt(10_000)},
			{Currency: "USD", Value: big.NewInt(50_000)},
		},
	}

	changes := object(t, summaryDocument(t, s, document.FindingsTally{}, nil), "net_worth", "changes")

	assert.Equal(t, []any{
		map[string]any{"type": "chequing", "currency": "CAD", "value": "100.00"},
		map[string]any{"type": "chequing", "currency": "USD", "value": "200.00"},
		map[string]any{"type": "brokerage", "currency": "USD", "value": "300.00"},
	}, changes["types"])
	assert.Equal(t, []any{
		map[string]any{"currency": "CAD", "value": "100.00"},
		map[string]any{"currency": "USD", "value": "500.00"},
	}, changes["totals"])
}

func Test_NewSummary_writes_a_change_with_no_value_as_null(t *testing.T) {
	s := septemberSummary()
	s.Change = &report.NetWorthChange{
		Types:  []report.NetWorthChangeType{{Type: "brokerage", Currency: "CAD"}},
		Totals: []report.NetWorthChangeTotal{{Currency: "CAD"}},
	}

	changes := object(t, summaryDocument(t, s, document.FindingsTally{}, nil), "net_worth", "changes")

	assert.Equal(t, []any{map[string]any{"type": "brokerage", "currency": "CAD", "value": nil}}, changes["types"])
	assert.Equal(t, []any{map[string]any{"currency": "CAD", "value": nil}}, changes["totals"])
}

func Test_NewSummary_writes_empty_changes_when_the_first_month_end_holds_no_balance(t *testing.T) {
	s := septemberSummary()
	s.Change = nil

	changes := object(t, summaryDocument(t, s, document.FindingsTally{}, nil), "net_worth", "changes")

	assert.Equal(t, map[string]any{"types": []any{}, "totals": []any{}}, changes)
}

func Test_NewSummary_writes_the_same_dates_as_networth_for_the_same_listing(t *testing.T) {
	n := report.NetWorth{
		Dates: []report.NetWorthDate{{
			Date: civil(2026, time.August, 31),
			Rows: []store.NetWorthRow{{Type: "chequing", Currency: "CAD", Balance: big.NewInt(100_000), BalanceCAD: big.NewInt(100_000)}},
			Totals: []report.NetWorthTotal{
				{Currency: "CAD", Value: big.NewInt(100_000)},
			},
		}},
		Currency: money.CAD,
	}
	s := septemberSummary()
	s.NetWorth = n

	got := object(t, summaryDocument(t, s, document.FindingsTally{}, nil), "net_worth")["dates"]

	assert.Equal(t, netWorthJSON(t, n, nil)["dates"], got)
}

func Test_SummaryWarnings_lists_the_snapshot_warning_for_a_snapshot_that_misses_part_of_the_month(t *testing.T) {
	taken := time.Date(2026, time.September, 28, 18, 2, 0, 0, time.UTC)
	cases := []struct {
		name     string
		coverage report.SnapshotCoverage
		want     []string
	}{
		{name: "covers", coverage: report.SnapshotCovers, want: []string{}},
		{name: "predates the month end", coverage: report.SnapshotPredatesMonthEnd, want: []string{
			"the store was built from a snapshot taken 2026-09-28 14:02 EDT, before September 2026 ended, so transactions " +
				"from the rest of the month are missing; open your Quicken file, run quarry sync, then run quarry summary again",
		}},
		{name: "time unknown", coverage: report.SnapshotTimeUnknown, want: []string{
			"cannot tell whether the store holds all of September 2026: its snapshot's manifest does not record when it was " +
				"taken; open your Quicken file and run quarry sync to take a new snapshot",
		}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			summary := summaryOf(civil(2026, time.September, 1), easternDaylight, taken, c.coverage)

			assert.Equal(t, c.want, document.SummaryWarnings(summary, "run quarry summary again"))
		})
	}
}
