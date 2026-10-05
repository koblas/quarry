package mcp_test

import (
	"encoding/json"
	"math/big"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/config"
	"github.com/koblas/quarry/internal/mcp"
	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/report/document"
	"github.com/koblas/quarry/internal/store"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	netWorthLogPrefix = "quarry: mcp: net_worth: "
	netWorthConfigLog = "cannot read quarry's config file; run quarry networth to see why"
	asOfConflictLine  = "as_of cannot be combined with since or until; pass as_of for one day, or since and until for month ends"
	netWorthCutNote   = "net_worth lists the first 500 month ends of 501; pass a later since, or query v_net_worth for the rest"
)

// atNetWorthToday is the clock option that makes today 2026-09-29.
func atNetWorthToday() mcp.Option {
	return mcp.WithClock(func() time.Time { return time.Date(2026, time.September, 29, 12, 0, 0, 0, time.UTC) })
}

// inTheYear2100 is the clock option that puts every month end the cap tests list before today.
func inTheYear2100() mcp.Option {
	return mcp.WithClock(func() time.Time { return time.Date(2100, time.January, 15, 0, 0, 0, 0, time.UTC) })
}

// civilDay is the day as the UTC midnight the store is asked for.
func civilDay(year int, month time.Month, day int) time.Time {
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

// decodeNetWorth is result's one text block decoded as the networth document.
func decodeNetWorth(t *testing.T, result *sdk.CallToolResult) document.NetWorth {
	t.Helper()
	require.False(t, result.IsError, textOf(t, result))
	var doc document.NetWorth
	require.NoError(t, json.Unmarshal([]byte(textOf(t, result)), &doc))
	return doc
}

// listedDates is the dates doc lists, as text.
func listedDates(doc document.NetWorth) []string {
	dates := make([]string, len(doc.Dates))
	for i, d := range doc.Dates {
		dates[i] = d.Date
	}
	return dates
}

func Test_net_worth_refuses_as_of_with_since_or_until(t *testing.T) {
	cases := []struct {
		name      string
		arguments map[string]any
	}{
		{name: "as_of and since", arguments: map[string]any{"as_of": "2026-03", "since": "2026-01"}},
		{name: "as_of and until", arguments: map[string]any{"as_of": "2026-03", "until": "2026-06"}},
		{name: "as_of and an empty since", arguments: map[string]any{"as_of": "2026-03", "since": ""}},
		{name: "a bad as_of and since: the conflict wins", arguments: map[string]any{"as_of": "2024-13", "since": "2026-01"}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			stub := &configStub{}
			fake := &fakeStore{}
			h := newHarness(t, fake, nil, mcp.WithConfig(stub.load), atNetWorthToday())

			result := h.netWorth(t, c.arguments)

			assert.True(t, result.IsError)
			assert.Equal(t, asOfConflictLine, textOf(t, result))
			assert.Empty(t, stub.commands)
			assert.Zero(t, h.built)
			assert.Empty(t, fake.netWorthAsked)
			assert.Equal(t, netWorthLogPrefix+asOfRefusedLine+"\n", h.stderr.String())
		})
	}
}

func Test_net_worth_refuses_an_as_of_it_cannot_use_with_the_class_line_before_the_config_or_the_store(t *testing.T) {
	cases := []struct {
		name string
		asOf string
		want string
	}{
		{name: "not a date", asOf: "2024-13", want: `as_of "2024-13" is not a date; use YYYY, YYYY-MM or YYYY-MM-DD`},
		{name: "empty", asOf: "", want: `as_of "" is not a date; use YYYY, YYYY-MM or YYYY-MM-DD`},
		{name: "after today", asOf: "2099", want: "as_of 2099 is after today; net worth is valued up to today only, so pass an earlier as_of"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			stub := &configStub{}
			fake := &fakeStore{}
			h := newHarness(t, fake, nil, mcp.WithConfig(stub.load), atNetWorthToday())

			result := h.netWorth(t, map[string]any{"as_of": c.asOf})

			assert.True(t, result.IsError)
			assert.Equal(t, c.want, textOf(t, result))
			assert.Empty(t, stub.commands)
			assert.Zero(t, h.built)
			assert.Empty(t, fake.netWorthAsked)
			assert.Equal(t, netWorthLogPrefix+asOfRefusedLine+"\n", h.stderr.String())
		})
	}
}

func Test_net_worth_refuses_a_window_it_cannot_list_with_the_class_line_before_the_config_or_the_store(t *testing.T) {
	cases := []struct {
		name      string
		arguments map[string]any
		want      string
	}{
		{name: "since after today", arguments: map[string]any{"since": "2099"}, want: "since 2099 is after today; net worth is valued up to today only, so pass an earlier since"},
		{name: "since after until", arguments: map[string]any{"since": "2026-05", "until": "2026-03"}, want: "since 2026-05 is after until 2026-03"},
		{name: "until before the default since", arguments: map[string]any{"until": "2025"}, want: "until 2025 is before the default since 2026-01-01; pass since too"},
		{name: "since not a date", arguments: map[string]any{"since": "2024-13"}, want: `since "2024-13" is not a date; use YYYY, YYYY-MM or YYYY-MM-DD`},
		{name: "until not a date", arguments: map[string]any{"until": "soon"}, want: `until "soon" is not a date; use YYYY, YYYY-MM or YYYY-MM-DD`},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			stub := &configStub{}
			fake := &fakeStore{}
			h := newHarness(t, fake, nil, mcp.WithConfig(stub.load), atNetWorthToday())

			result := h.netWorth(t, c.arguments)

			assert.True(t, result.IsError)
			assert.Equal(t, c.want, textOf(t, result))
			assert.Empty(t, stub.commands)
			assert.Zero(t, h.built)
			assert.Empty(t, fake.netWorthAsked)
			assert.Equal(t, netWorthLogPrefix+windowRefusedLine+"\n", h.stderr.String())
		})
	}
}

func Test_net_worth_clamps_an_until_in_the_future_to_today(t *testing.T) {
	h := newHarness(t, &fakeStore{}, nil, atNetWorthToday())

	doc := decodeNetWorth(t, h.netWorth(t, map[string]any{"until": "2099"}))

	assert.Equal(t, []string{
		"2026-01-31", "2026-02-28", "2026-03-31", "2026-04-30", "2026-05-31", "2026-06-30", "2026-07-31", "2026-08-31", "2026-09-29",
	}, listedDates(doc))
	require.NotNil(t, doc.Until)
	assert.Equal(t, "2026-09-29", *doc.Until)
}

func Test_net_worth_with_absent_null_or_empty_arguments_values_today(t *testing.T) {
	for name, args := range map[string]any{
		"omitted": nil,
		"null":    json.RawMessage("null"),
		"empty":   map[string]any{},
	} {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t, &fakeStore{}, nil, atNetWorthToday())

			doc := decodeNetWorth(t, h.netWorth(t, args))

			require.NotNil(t, doc.AsOf)
			assert.Equal(t, "2026-09-29", *doc.AsOf)
			assert.Nil(t, doc.Since)
			assert.Nil(t, doc.Until)
			assert.Equal(t, []string{"2026-09-29"}, listedDates(doc))
		})
	}
}

func Test_net_worth_asks_the_store_once_for_the_days_the_call_lists(t *testing.T) {
	cases := []struct {
		name      string
		arguments map[string]any
		want      []time.Time
	}{
		{name: "a snapshot asks the one day as_of names", arguments: map[string]any{"as_of": "2026-03"}, want: []time.Time{civilDay(2026, time.March, 31)}},
		{name: "a snapshot with no as_of asks today", arguments: map[string]any{}, want: []time.Time{civilDay(2026, time.September, 29)}},
		{
			name: "a history asks every month end and today when it is mid-month", arguments: map[string]any{"since": "2026-07"},
			want: []time.Time{civilDay(2026, time.July, 31), civilDay(2026, time.August, 31), civilDay(2026, time.September, 29)},
		},
		{
			name: "a history ending on a month end asks no extra day", arguments: map[string]any{"since": "2026-07", "until": "2026-08"},
			want: []time.Time{civilDay(2026, time.July, 31), civilDay(2026, time.August, 31)},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fake := &fakeStore{}
			h := newHarness(t, fake, nil, atNetWorthToday())

			decodeNetWorth(t, h.netWorth(t, c.arguments))

			assert.Equal(t, []store.NetWorthParams{{Dates: c.want}}, fake.netWorthAsked)
		})
	}
}

func Test_net_worth_reads_today_once_at_the_start_of_every_call(t *testing.T) {
	clock := &steppingClock{times: []time.Time{
		time.Date(2026, time.September, 29, 23, 59, 0, 0, time.UTC),
		time.Date(2026, time.September, 30, 0, 1, 0, 0, time.UTC),
	}}
	h := newHarness(t, &fakeStore{}, nil, mcp.WithClock(clock.now))

	first := decodeNetWorth(t, h.netWorth(t, map[string]any{"since": "2026-09"}))
	second := decodeNetWorth(t, h.netWorth(t, map[string]any{"since": "2026-09"}))

	assert.Equal(t, []string{"2026-09-29"}, listedDates(first))
	assert.Equal(t, []string{"2026-09-30"}, listedDates(second))
	assert.Equal(t, 2, clock.reads)
}

func Test_net_worth_does_not_read_the_config_when_the_call_names_a_currency(t *testing.T) {
	stub := &configStub{err: errBadConfig}
	h := newHarness(t, &fakeStore{}, nil, mcp.WithConfig(stub.load), atNetWorthToday())

	doc := decodeNetWorth(t, h.netWorth(t, map[string]any{"currency": "USD"}))

	assert.Equal(t, "USD", doc.Currency)
	assert.Empty(t, stub.commands)
}

func Test_net_worth_reads_the_config_as_the_mcp_command_and_lists_its_warnings_first(t *testing.T) {
	stub := &configStub{cfg: config.Config{WarningsAbsolute: []string{configUnknownKeyWarning}}}
	h := newHarness(t, &fakeStore{}, nil, mcp.WithConfig(stub.load), atNetWorthToday())

	doc := decodeNetWorth(t, h.netWorth(t, map[string]any{}))

	assert.Equal(t, []string{"mcp"}, stub.commands)
	require.Len(t, doc.Warnings, 2)
	assert.Equal(t, configUnknownKeyWarning, doc.Warnings[0])
	assert.Contains(t, doc.Warnings[1], "no account")
}

func Test_net_worth_returns_the_report_as_networth_does_with_totals_and_the_rate_warning_unchanged(t *testing.T) {
	asOf := civilDay(2026, time.March, 31)
	fake := &fakeStore{netWorth: store.NetWorth{
		Rows: []store.NetWorthRow{
			{Date: asOf, Type: "checking", Currency: "CAD", Accounts: 1, Balance: big.NewInt(100_000), BalanceCAD: big.NewInt(100_000)},
			{Date: asOf, Type: "checking", Currency: "USD", Accounts: 1, Balance: big.NewInt(5_000)},
		},
		FirstRate:    civilDay(2026, time.April, 1),
		FirstBalance: civilDay(2026, time.January, 5),
	}}
	h := newHarness(t, fake, nil, atNetWorthToday())
	listing, err := report.NewServer(report.WithStore(fake), report.WithHome(testHome)).
		NetWorth(t.Context(), report.NetWorthRequest{AsOf: asOf, Currency: money.CAD})
	require.NoError(t, err)

	result := h.netWorth(t, map[string]any{"as_of": "2026-03", "currency": "CAD"})

	assert.JSONEq(t, jsonOf(t, document.NewNetWorth(listing, document.NetWorthWarnings(listing, document.NativeParameter))), textOf(t, result))
	doc := decodeNetWorth(t, result)
	assert.Equal(t, []document.NetWorthTotal{{Currency: "CAD", Value: "1000.00"}, {Currency: "USD", Value: "50.00"}}, doc.Dates[0].Totals)
	require.Len(t, doc.Warnings, 1)
	assert.Equal(t, "USD balances on 2026-03-31, before 2026-04-01, the first exchange rate in the store, are not converted to CAD "+
		"and are left out of the CAD total; pass currency native to list them", doc.Warnings[0])
}

func Test_net_worth_names_the_currency_parameter_in_the_no_rates_warning(t *testing.T) {
	asOf := civilDay(2026, time.March, 31)
	h := newHarness(t, &fakeStore{netWorth: store.NetWorth{
		Rows:         []store.NetWorthRow{{Date: asOf, Type: "checking", Currency: "USD", Accounts: 1, Balance: big.NewInt(5_000)}},
		FirstBalance: civilDay(2026, time.January, 5),
	}}, nil, atNetWorthToday())

	doc := decodeNetWorth(t, h.netWorth(t, map[string]any{"as_of": "2026-03", "currency": "CAD"}))

	assert.Equal(t, []string{"the store has no exchange rates, so USD balances are not converted to CAD and are left out of the CAD total; " +
		"pass currency native to list them, or run quarry sync to fetch rates"}, doc.Warnings)
}

func Test_net_worth_warns_that_a_priced_holding_lacks_only_an_exchange_rate(t *testing.T) {
	asOf := civilDay(2026, time.March, 31)
	h := newHarness(t, &fakeStore{netWorth: store.NetWorth{
		Rows: []store.NetWorthRow{
			{Date: asOf, Type: "brokerage", Currency: "CAD", Accounts: 1, Balance: big.NewInt(100_000), BalanceCAD: big.NewInt(100_000)},
		},
		Unvalued: []store.UnvaluedHolding{{
			Date: asOf, AccountID: "acct-1", Account: "Brokerage", AccountCurrency: "CAD", SecurityID: "sec-1", Security: "Globex",
			Currency: new("USD"), Priced: true,
		}},
		FirstRate:    civilDay(2026, time.April, 1),
		FirstBalance: civilDay(2026, time.January, 5),
	}}, nil, atNetWorthToday())

	doc := decodeNetWorth(t, h.netWorth(t, map[string]any{"as_of": "2026-03", "currency": "CAD"}))

	assert.Equal(t, []string{`"Brokerage" holds 1 USD security valued on 2026-03-31, before 2026-04-01, ` +
		`the first exchange rate in the store, so its CAD balance leaves it out`}, doc.Warnings)
}

func Test_net_worth_refuses_an_unreadable_config_before_building_the_report(t *testing.T) {
	stub := &configStub{err: errBadConfig}
	h := newHarness(t, &fakeStore{}, errFactoryBroke, mcp.WithConfig(stub.load), atNetWorthToday())

	result := h.netWorth(t, map[string]any{})

	assert.True(t, result.IsError)
	assert.Equal(t, errBadConfig.Error(), textOf(t, result))
	assert.Zero(t, h.built)
	assert.Equal(t, netWorthLogPrefix+netWorthConfigLog+"\n", h.stderr.String())
}

func Test_net_worth_answers_a_report_factory_failure_with_the_generic_log_line(t *testing.T) {
	h := newHarness(t, &fakeStore{}, errFactoryBroke, atNetWorthToday())

	result := h.netWorth(t, map[string]any{})

	assert.True(t, result.IsError)
	assert.Equal(t, errFactoryBroke.Error(), textOf(t, result))
	assert.Equal(t, netWorthLogPrefix+failedLogLine+"\n", h.stderr.String())
}

func Test_net_worth_sends_a_store_refusal_verbatim_to_the_client_and_stderr(t *testing.T) {
	h := newHarness(t, &fakeStore{err: &store.OpenError{Fault: store.OpenFaultMissing, Path: testStorePath}}, nil, atNetWorthToday())

	result := h.netWorth(t, map[string]any{})

	assert.True(t, result.IsError)
	assert.Equal(t, missingStoreLine, textOf(t, result))
	assert.Equal(t, netWorthLogPrefix+missingStoreLine+"\n", h.stderr.String())
}

func Test_net_worth_answers_a_plain_store_fault_with_the_generic_log_line(t *testing.T) {
	h := newHarness(t, &fakeStore{err: errDiskOnFire}, nil, atNetWorthToday())

	result := h.netWorth(t, map[string]any{})

	assert.True(t, result.IsError)
	assert.Equal(t, errDiskOnFire.Error(), textOf(t, result))
	assert.Equal(t, netWorthLogPrefix+failedLogLine+"\n", h.stderr.String())
}

func Test_net_worth_refuses_arguments_the_schema_rejects_without_reading_the_config_or_the_store(t *testing.T) {
	cases := map[string]map[string]any{
		"an unknown property":   {"accounts": []string{"Brokerage"}},
		"a numeric as_of":       {"as_of": 2026},
		"a lower-case currency": {"currency": "cad"},
	}

	for name, arguments := range cases {
		t.Run(name, func(t *testing.T) {
			stub := &configStub{}
			fake := &fakeStore{}
			h := newHarness(t, fake, nil, mcp.WithConfig(stub.load), atNetWorthToday())

			result := h.netWorth(t, arguments)

			assert.True(t, result.IsError)
			assert.Empty(t, stub.commands)
			assert.Zero(t, h.built)
			assert.Empty(t, fake.netWorthAsked)
			assert.Equal(t, netWorthLogPrefix+argsRefusedLog+"\n", h.stderr.String())
		})
	}
}

func Test_net_worth_lists_the_first_500_month_ends_and_warns_on_501(t *testing.T) {
	stub := &configStub{cfg: config.Config{WarningsAbsolute: []string{configUnknownKeyWarning}}}
	h := newHarness(t, &fakeStore{}, nil, mcp.WithConfig(stub.load), inTheYear2100())

	doc := decodeNetWorth(t, h.netWorth(t, map[string]any{"since": "2000-01", "until": "2041-09"}))

	require.Len(t, doc.Dates, 500)
	assert.Equal(t, "2000-01-31", doc.Dates[0].Date)
	assert.Equal(t, "2041-08-31", doc.Dates[499].Date)
	assert.Equal(t, []string{
		configUnknownKeyWarning,
		"no account has a balance at any month end from 2000-01-31 to 2041-09-30; no account in Quicken's reports has transactions or holdings",
		netWorthCutNote,
	}, doc.Warnings)
}

func Test_net_worth_counts_every_month_end_in_the_rate_warning_before_the_cut(t *testing.T) {
	rows := make([]store.NetWorthRow, 0, 501)
	for i := range 501 {
		monthEnd := civilDay(2000+i/12, time.Month(i%12+2), 0)
		rows = append(rows, store.NetWorthRow{Date: monthEnd, Type: "checking", Currency: "USD", Accounts: 1, Balance: big.NewInt(5_000)})
	}
	fake := &fakeStore{netWorth: store.NetWorth{Rows: rows, FirstRate: civilDay(2042, time.January, 1)}}
	h := newHarness(t, fake, nil, inTheYear2100())

	doc := decodeNetWorth(t, h.netWorth(t, map[string]any{"since": "2000-01", "until": "2041-09", "currency": "CAD"}))

	require.Len(t, doc.Dates, 500)
	assert.Equal(t, []string{
		"USD balances on 501 month ends before 2042-01-01, the first exchange rate in the store, are not converted to CAD " +
			"and are left out of the CAD total; pass currency native to list them",
		netWorthCutNote,
	}, doc.Warnings)
}

func Test_net_worth_lists_exactly_500_month_ends_with_no_cut_note(t *testing.T) {
	stub := &configStub{cfg: config.Config{WarningsAbsolute: []string{configUnknownKeyWarning}}}
	h := newHarness(t, &fakeStore{}, nil, mcp.WithConfig(stub.load), inTheYear2100())

	doc := decodeNetWorth(t, h.netWorth(t, map[string]any{"since": "2000-01", "until": "2041-08"}))

	require.Len(t, doc.Dates, 500)
	assert.Equal(t, "2041-08-31", doc.Dates[499].Date)
	assert.Len(t, doc.Warnings, 2)
}
