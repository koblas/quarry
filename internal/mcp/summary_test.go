package mcp_test

import (
	"math/big"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/config"
	"github.com/koblas/quarry/internal/mcp"
	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/report/document"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	summaryLogPrefix = "quarry: mcp: monthly_summary: "
	summaryConfigLog = "cannot read quarry's config file; run quarry summary to see why"
	monthRefusedLine = "refused the call's month; details went to the client only"
	summaryAdvice    = "pass an earlier or later month"
)

// summaryToday is a day in October 2026, so the default month is September.
var summaryToday = time.Date(2026, time.October, 6, 12, 0, 0, 0, time.UTC)

// atSummaryToday is the clock option that fixes today at summaryToday.
func atSummaryToday() mcp.Option {
	return mcp.WithClock(func() time.Time { return summaryToday })
}

// coveredSummary is a store read whose snapshot was taken after September 2026 ended: it adds no warning.
func coveredSummary() store.Summary {
	st := statusFixture()
	st.Run.Snapshot.TakenAt = time.Date(2026, time.October, 1, 9, 5, 0, 0, time.UTC)
	return store.Summary{Status: st}
}

func Test_monthly_summary_reads_today_once_at_the_start_of_every_call(t *testing.T) {
	clock := &steppingClock{times: []time.Time{
		time.Date(2026, time.October, 31, 23, 59, 0, 0, time.UTC),
		time.Date(2026, time.November, 1, 0, 1, 0, 0, time.UTC),
	}}
	h := newHarness(t, &fakeStore{summary: coveredSummary()}, nil, withDefaultConfig(), mcp.WithClock(clock.now))

	first := decodeDoc[document.Summary](t, h.monthlySummary(t, map[string]any{}))
	second := decodeDoc[document.Summary](t, h.monthlySummary(t, map[string]any{}))

	assert.Equal(t, "2026-09", first.Month)
	assert.Equal(t, "2026-10", second.Month)
	assert.Equal(t, 2, clock.reads)
}

func Test_monthly_summary_defaults_to_last_month(t *testing.T) {
	h := newHarness(t, &fakeStore{summary: coveredSummary()}, nil, withDefaultConfig(), atSummaryToday())

	doc := decodeDoc[document.Summary](t, h.monthlySummary(t, map[string]any{}))

	assert.Equal(t, "2026-09", doc.Month)
	assert.Equal(t, "2026-09-01", doc.Since)
	assert.Equal(t, "2026-09-30", doc.Until)
}

func Test_monthly_summary_summarizes_the_month_it_is_given(t *testing.T) {
	cases := []struct {
		name  string
		month string
		until string
	}{
		{name: "an earlier month", month: "2026-08", until: "2026-08-31"},
		{name: "the last month that has ended", month: "2026-09", until: "2026-09-30"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t, &fakeStore{summary: coveredSummary()}, nil, withDefaultConfig(), atSummaryToday())

			doc := decodeDoc[document.Summary](t, h.monthlySummary(t, map[string]any{"month": c.month}))

			assert.Equal(t, c.month, doc.Month)
			assert.Equal(t, c.until, doc.Until)
		})
	}
}

func Test_monthly_summary_refuses_a_month_it_cannot_summarize_before_the_config_or_the_store(t *testing.T) {
	cases := []struct {
		name  string
		month string
		want  string
	}{
		{"a month without its leading zero", "2026-9", `month "2026-9" is not a month; use YYYY-MM, such as 2026-09`},
		{"an empty month", "", `month "" is not a month; use YYYY-MM, such as 2026-09`},
		{"year zero", "0000-01", `month "0000-01" is not a month; use YYYY-MM, such as 2026-09`},
		{"a thirteenth month", "2026-13", `month "2026-13" is not a month; use YYYY-MM, such as 2026-09`},
		{"a day instead of a month", "2026-09-15", `month "2026-09-15" is not a month; use YYYY-MM, such as 2026-09`},
		{"the current month", "2026-10", "month 2026-10 has not ended; monthly_summary covers whole months, so pass 2026-09 or earlier"},
		{"a future month", "2027-01", "month 2027-01 has not ended; monthly_summary covers whole months, so pass 2026-09 or earlier"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			stub := &configStub{}
			h := newHarness(t, &fakeStore{}, nil, mcp.WithConfig(stub.load), atSummaryToday())

			result := h.monthlySummary(t, map[string]any{"month": c.month})

			assert.True(t, result.IsError)
			assert.Equal(t, c.want, textOf(t, result))
			assert.Equal(t, summaryLogPrefix+monthRefusedLine+"\n", h.stderr.String())
			assert.Empty(t, stub.commands)
			assert.Zero(t, h.built)
		})
	}
}

func Test_monthly_summary_reads_the_store_once_for_the_month_and_the_one_before(t *testing.T) {
	fake := &fakeStore{summary: coveredSummary()}
	h := newHarness(t, fake, nil, withDefaultConfig(), atSummaryToday())

	decodeDoc[document.Summary](t, h.monthlySummary(t, map[string]any{}))

	assert.Equal(t, []store.SummaryParams{{
		Through: civilDay(2026, time.September, 30),
		Dates:   []time.Time{civilDay(2026, time.August, 31), civilDay(2026, time.September, 30)},
	}}, fake.summaryAsked)
	assert.Equal(t, 1, h.built)
	assert.Zero(t, fake.statusReads)
	assert.Empty(t, fake.through)
}

func Test_monthly_summary_loads_the_config_once_per_call(t *testing.T) {
	cases := []struct {
		name      string
		arguments map[string]any
	}{
		{name: "without a currency", arguments: map[string]any{}},
		{name: "with a currency", arguments: map[string]any{"currency": "USD"}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			stub := &configStub{}
			h := newHarness(t, &fakeStore{summary: coveredSummary()}, nil, mcp.WithConfig(stub.load), atSummaryToday())

			decodeDoc[document.Summary](t, h.monthlySummary(t, c.arguments))

			assert.Equal(t, []string{"mcp"}, stub.commands)
		})
	}
}

func Test_monthly_summary_counts_an_ignored_finding_when_currency_is_given(t *testing.T) {
	stub := &configStub{cfg: config.Config{Ignore: []string{duplicateID, fixedID}}}
	h := newHarness(t, &fakeStore{summary: coveredSummary()}, nil, mcp.WithConfig(stub.load), atSummaryToday())

	doc := decodeDoc[document.Summary](t, h.monthlySummary(t, map[string]any{"currency": "USD"}))

	require.NotNil(t, doc.Findings.Ignored)
	assert.Equal(t, 1, *doc.Findings.Ignored)
	assert.Equal(t, 0, doc.Findings.Open)
	assert.Equal(t, "USD", doc.Currency)
}

func Test_monthly_summary_takes_the_currency_from_the_config_when_the_call_names_none(t *testing.T) {
	stub := &configStub{cfg: config.Config{Currency: money.USD}}
	h := newHarness(t, &fakeStore{summary: coveredSummary()}, nil, mcp.WithConfig(stub.load), atSummaryToday())

	doc := decodeDoc[document.Summary](t, h.monthlySummary(t, map[string]any{}))

	assert.Equal(t, "USD", doc.Currency)
}

func Test_monthly_summary_keeps_config_warnings_when_currency_is_given(t *testing.T) {
	stub := &configStub{cfg: config.Config{WarningsAbsolute: []string{configUnknownKeyWarning}}}
	h := newHarness(t, &fakeStore{summary: coveredSummary()}, nil, mcp.WithConfig(stub.load), atSummaryToday())

	doc := decodeDoc[document.Summary](t, h.monthlySummary(t, map[string]any{"currency": "CAD"}))

	assert.Equal(t, []string{configUnknownKeyWarning}, doc.Warnings)
}

func Test_monthly_summary_refuses_an_unreadable_config_without_currency_before_building_the_report(t *testing.T) {
	stub := &configStub{err: errBadConfig}
	h := newHarness(t, &fakeStore{}, errFactoryBroke, mcp.WithConfig(stub.load), atSummaryToday())

	result := h.monthlySummary(t, map[string]any{})

	assert.True(t, result.IsError)
	assert.Equal(t, errBadConfig.Error(), textOf(t, result))
	assert.Equal(t, summaryLogPrefix+summaryConfigLog+"\n", h.stderr.String())
	assert.Zero(t, h.built)
}

func Test_monthly_summary_warns_it_cannot_tell_what_is_ignored_when_the_config_is_unreadable_with_currency(t *testing.T) {
	stub := &configStub{err: errBadConfig}
	h := newHarness(t, &fakeStore{summary: coveredSummary()}, nil, mcp.WithConfig(stub.load), atSummaryToday())

	doc := decodeDoc[document.Summary](t, h.monthlySummary(t, map[string]any{"currency": "USD"}))

	assert.Nil(t, doc.Findings.Ignored)
	assert.Equal(t, 1, doc.Findings.Open)
	assert.Equal(t, "USD", doc.Currency)
	assert.Equal(t, []string{document.CannotTellChoices(config.ProblemAbsolute(errBadConfig))}, doc.Warnings)
}

func Test_monthly_summary_answers_a_report_factory_failure_with_the_generic_log_line(t *testing.T) {
	h := newHarness(t, &fakeStore{}, errFactoryBroke, withDefaultConfig(), atSummaryToday())

	result := h.monthlySummary(t, map[string]any{})

	assert.True(t, result.IsError)
	assert.Equal(t, errFactoryBroke.Error(), textOf(t, result))
	assert.Equal(t, summaryLogPrefix+failedLogLine+"\n", h.stderr.String())
}

func Test_monthly_summary_answers_a_plain_store_fault_with_the_generic_log_line(t *testing.T) {
	h := newHarness(t, &fakeStore{err: errDiskOnFire}, nil, withDefaultConfig(), atSummaryToday())

	result := h.monthlySummary(t, map[string]any{})

	assert.True(t, result.IsError)
	assert.Equal(t, errDiskOnFire.Error(), textOf(t, result))
	assert.Equal(t, summaryLogPrefix+failedLogLine+"\n", h.stderr.String())
}

func Test_monthly_summary_sends_a_store_refusal_verbatim_to_the_client_and_stderr(t *testing.T) {
	h := newHarness(t, &fakeStore{err: &store.OpenError{Fault: store.OpenFaultMissing, Path: testStorePath}}, nil,
		withDefaultConfig(), atSummaryToday())

	result := h.monthlySummary(t, map[string]any{})

	assert.True(t, result.IsError)
	assert.Equal(t, missingStoreLine, textOf(t, result))
	assert.Equal(t, summaryLogPrefix+missingStoreLine+"\n", h.stderr.String())
}

func Test_monthly_summary_words_a_snapshot_taken_before_the_month_ended_with_the_tool_to_call_again(t *testing.T) {
	cases := []struct {
		name      string
		arguments map[string]any
	}{
		{name: "for the default month", arguments: map[string]any{}},
		{name: "for a month the call names", arguments: map[string]any{"month": "2026-09"}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			summary := coveredSummary()
			summary.Status.Run.Snapshot.TakenAt = time.Date(2026, time.September, 20, 9, 5, 0, 0, time.UTC)
			h := newHarness(t, &fakeStore{summary: summary}, nil, withDefaultConfig(), atSummaryToday())

			doc := decodeDoc[document.Summary](t, h.monthlySummary(t, c.arguments))

			assert.Equal(t, []string{"the store was built from a snapshot taken 2026-09-20 09:05 UTC, before September 2026 ended, " +
				"so transactions from the rest of the month are missing; open your Quicken file, run quarry sync, " +
				"then call monthly_summary again"}, doc.Warnings)
		})
	}
}

func Test_monthly_summary_gives_a_snapshot_with_no_time_no_call_again_tail(t *testing.T) {
	h := newHarness(t, &fakeStore{summary: store.Summary{Status: statusFixture()}}, nil, withDefaultConfig(), atSummaryToday())

	doc := decodeDoc[document.Summary](t, h.monthlySummary(t, map[string]any{}))

	assert.Equal(t, []string{"cannot tell whether the store holds all of September 2026: its snapshot's manifest does not record " +
		"when it was taken; open your Quicken file and run quarry sync to take a new snapshot"}, doc.Warnings)
}

func Test_monthly_summary_words_the_rate_advice_for_a_tool_parameter(t *testing.T) {
	summary := coveredSummary()
	summary.NetWorth = store.NetWorth{
		Rows:         []store.NetWorthRow{{Date: civilDay(2026, time.September, 30), Type: "checking", Currency: "USD", Accounts: 1, Balance: big.NewInt(5_000)}},
		FirstBalance: civilDay(2026, time.January, 5),
	}
	h := newHarness(t, &fakeStore{summary: summary}, nil, withDefaultConfig(), atSummaryToday())

	doc := decodeDoc[document.Summary](t, h.monthlySummary(t, map[string]any{"currency": "CAD"}))

	assert.Equal(t, []string{"the store has no exchange rates, so USD balances are not converted to CAD and are left out of the CAD total; " +
		"pass currency native to list them, or run quarry sync to fetch rates"}, doc.Warnings)
}

func Test_monthly_summary_caps_anomalies_charges_and_recurring_series_at_500(t *testing.T) {
	const (
		seriesFirst = 1000
		amount      = 1000
	)
	cases := []struct {
		name      string
		anomalies int
		series    int
		want      []string
	}{
		{name: "both lists at the cap", anomalies: 500, series: 500, want: []string{}},
		{name: "one charge over", anomalies: 501, series: 500, want: []string{"monthly_summary lists the first 500 charges of 501; " + summaryAdvice}},
		{name: "one series over", anomalies: 500, series: 501, want: []string{"monthly_summary lists the first 500 series of 501; " + summaryAdvice}},
		{name: "both over", anomalies: 501, series: 501, want: []string{
			"monthly_summary lists the first 500 charges of 501; " + summaryAdvice,
			"monthly_summary lists the first 500 series of 501; " + summaryAdvice,
		}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			summary := coveredSummary()
			summary.Charges = unusualChargesIn(time.September, 0, c.anomalies)
			summary.Charges.Rows = append(summary.Charges.Rows, monthlyChargesFrom(seriesFirst, c.series, amount).Rows...)
			stub := &configStub{cfg: config.Config{Currency: money.CAD}}
			h := newHarness(t, &fakeStore{summary: summary}, nil, mcp.WithConfig(stub.load), atSummaryToday())

			doc := decodeDoc[document.Summary](t, h.monthlySummary(t, map[string]any{}))

			assert.Len(t, doc.Anomalies.Charges, min(c.anomalies, 500))
			assert.Len(t, doc.Recurring.Series, min(c.series, 500))
			assert.Equal(t, c.want, doc.Warnings)
		})
	}
}

func Test_monthly_summary_lists_the_config_warning_before_the_snapshot_warning(t *testing.T) {
	const snapshotBefore = "the store was built from a snapshot taken 2026-09-20 09:05 UTC, before September 2026 ended, " +
		"so transactions from the rest of the month are missing; open your Quicken file, run quarry sync, " +
		"then call monthly_summary again"
	const snapshotUnknown = "cannot tell whether the store holds all of September 2026: its snapshot's manifest does not record " +
		"when it was taken; open your Quicken file and run quarry sync to take a new snapshot"
	cases := []struct {
		name  string
		stub  *configStub
		taken time.Time
		want  []string
	}{
		{
			name:  "a config warning, then a snapshot taken before the month ended",
			stub:  &configStub{cfg: config.Config{WarningsAbsolute: []string{configUnknownKeyWarning}}},
			taken: time.Date(2026, time.September, 20, 9, 5, 0, 0, time.UTC),
			want:  []string{configUnknownKeyWarning, snapshotBefore},
		},
		{
			name: "an unreadable config, then a snapshot with no recorded time",
			stub: &configStub{err: errBadConfig},
			want: []string{document.CannotTellChoices(config.ProblemAbsolute(errBadConfig)), snapshotUnknown},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			summary := coveredSummary()
			summary.Status.Run.Snapshot.TakenAt = c.taken
			h := newHarness(t, &fakeStore{summary: summary}, nil, mcp.WithConfig(c.stub.load), atSummaryToday())

			doc := decodeDoc[document.Summary](t, h.monthlySummary(t, map[string]any{"currency": "USD"}))

			assert.Equal(t, c.want, doc.Warnings)
		})
	}
}

func Test_monthly_summary_leaves_an_account_the_config_classifies_out_of_the_open_count(t *testing.T) {
	cases := []struct {
		name      string
		cfg       config.Config
		arguments map[string]any
		want      int
	}{
		{name: "no classification", cfg: config.Config{}, arguments: map[string]any{}, want: 2},
		{name: "no classification, with a currency", cfg: config.Config{}, arguments: map[string]any{"currency": "USD"}, want: 2},
		{name: "registered", cfg: config.Config{Registered: []string{"acct-1"}}, arguments: map[string]any{}, want: 1},
		{name: "non-registered", cfg: config.Config{NonRegistered: []string{"acct-1"}}, arguments: map[string]any{}, want: 1},
		{name: "registered, with a currency", cfg: config.Config{Registered: []string{"acct-1"}}, arguments: map[string]any{"currency": "USD"}, want: 1},
		{name: "non-registered, with a currency", cfg: config.Config{NonRegistered: []string{"acct-1"}}, arguments: map[string]any{"currency": "USD"}, want: 1},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			summary := coveredSummary()
			summary.Status.Accounts = statusWithBrokerage().Accounts
			stub := &configStub{cfg: c.cfg}
			h := newHarness(t, &fakeStore{summary: summary}, nil, mcp.WithConfig(stub.load), atSummaryToday())

			doc := decodeDoc[document.Summary](t, h.monthlySummary(t, c.arguments))

			assert.Equal(t, c.want, doc.Findings.Open)
		})
	}
}

// summaryInstant is 2026-10-01 00:30 UTC, which is still 2026-09-30 in New York.
func summaryInstant(zone *time.Location) func() time.Time {
	return func() time.Time { return time.Date(2026, time.September, 30, 20, 30, 0, 0, summaryNewYork).In(zone) }
}

var summaryNewYork = time.FixedZone("EDT", -4*60*60)

func Test_monthly_summary_defaults_to_the_month_before_the_callers_local_month(t *testing.T) {
	cases := []struct {
		name string
		zone *time.Location
		want string
	}{
		{name: "an instant that is still September in the local zone", zone: summaryNewYork, want: "2026-08"},
		{name: "the same instant read in UTC", zone: time.UTC, want: "2026-09"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t, &fakeStore{summary: coveredSummary()}, nil, withDefaultConfig(), mcp.WithClock(summaryInstant(c.zone)))

			doc := decodeDoc[document.Summary](t, h.monthlySummary(t, map[string]any{}))

			assert.Equal(t, c.want, doc.Month)
		})
	}
}

func Test_monthly_summary_refuses_the_month_that_has_not_ended_in_the_callers_local_zone(t *testing.T) {
	local := newHarness(t, &fakeStore{summary: coveredSummary()}, nil, withDefaultConfig(), mcp.WithClock(summaryInstant(summaryNewYork)))
	utc := newHarness(t, &fakeStore{summary: coveredSummary()}, nil, withDefaultConfig(), mcp.WithClock(summaryInstant(time.UTC)))

	refused := local.monthlySummary(t, map[string]any{"month": "2026-09"})
	control := decodeDoc[document.Summary](t, utc.monthlySummary(t, map[string]any{"month": "2026-09"}))

	assert.True(t, refused.IsError)
	assert.Equal(t, "month 2026-09 has not ended; monthly_summary covers whole months, so pass 2026-08 or earlier", textOf(t, refused))
	assert.Equal(t, "2026-09", control.Month)
}
