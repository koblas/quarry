package mcp_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/config"
	"github.com/koblas/quarry/internal/mcp"
	"github.com/koblas/quarry/internal/report/document"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	acbNothingToShow = "no non-registered account has bought or sold a security; quarry acb has nothing to show"
	acbMapleOnly     = `"Maple Fund" is held only in registered accounts, so it has no ACB`
)

// acbConfig lists acct-n as non-registered and acct-r as registered.
func acbConfig() config.Config {
	return config.Config{NonRegistered: []string{"acct-n"}, Registered: []string{"acct-r"}}
}

// acbOptions is the options every ACB call runs under: acbConfig, and today fixed at 2026-09-29.
func acbOptions() []mcp.Option {
	return []mcp.Option{mcp.WithConfig((&configStub{cfg: acbConfig()}).load), atSeptember29()}
}

// acbBuy is a buy of millionths shares of security in account on date for cents.
func acbBuy(id string, sourceID int64, account, security string, date time.Time, millionths, cents int64) store.InvestmentTransaction {
	return store.InvestmentTransaction{
		ID: id, SourceID: sourceID, AccountID: account, SecurityID: &security, Date: date,
		Action: store.ActionBuy, Shares: &millionths, Amount: -cents, Currency: "CAD",
	}
}

// acbSell is a sell of millionths shares of security in account on date for cents.
func acbSell(id string, sourceID int64, account, security string, date time.Time, millionths, cents int64) store.InvestmentTransaction {
	sold := -millionths
	return store.InvestmentTransaction{
		ID: id, SourceID: sourceID, AccountID: account, SecurityID: &security, Date: date,
		Action: store.ActionSell, Shares: &sold, Amount: cents, Currency: "CAD",
	}
}

// acbHistory is Acme and Beta, bought and half sold in 2025 and 2026 in the non-registered acct-n, and Maple,
// traded only in the registered acct-r.
func acbHistory() store.InvestmentHistory {
	return store.InvestmentHistory{
		Accounts: []store.Account{
			{ID: "acct-n", Name: "Taxable", Type: store.AccountTypeBrokerage, Currency: "CAD"},
			{ID: "acct-r", Name: "RRSP", Type: store.AccountTypeBrokerage, Currency: "CAD"},
		},
		Securities: []store.Security{
			{ID: "sec-acme", Name: "Acme Corp", Ticker: new("ACME"), Currency: new("CAD")},
			{ID: "sec-beta", Name: "Beta Fund", Ticker: new("BETA"), Currency: new("CAD")},
			{ID: "sec-maple", Name: "Maple Fund", Ticker: new("MPL"), Currency: new("CAD")},
		},
		Transactions: []store.InvestmentTransaction{
			acbBuy("inv-1", 1, "acct-n", "sec-acme", time.Date(2025, time.February, 3, 0, 0, 0, 0, time.UTC), 10_000_000, 100_000),
			acbBuy("inv-2", 2, "acct-n", "sec-beta", time.Date(2025, time.February, 4, 0, 0, 0, 0, time.UTC), 10_000_000, 100_000),
			acbBuy("inv-3", 3, "acct-r", "sec-maple", time.Date(2025, time.February, 5, 0, 0, 0, 0, time.UTC), 10_000_000, 100_000),
			acbSell("inv-4", 4, "acct-n", "sec-acme", time.Date(2025, time.June, 2, 0, 0, 0, 0, time.UTC), 5_000_000, 70_000),
			acbSell("inv-5", 5, "acct-n", "sec-beta", time.Date(2026, time.June, 2, 0, 0, 0, 0, time.UTC), 5_000_000, 70_000),
		},
	}
}

// securityIDs is the ids of the securities doc lists, in order.
func securityIDs(doc document.ACB) []string {
	ids := make([]string, len(doc.Securities))
	for i, s := range doc.Securities {
		ids[i] = s.SecurityID
	}
	return ids
}

// yearsOf is the years doc lists, in order.
func yearsOf(doc document.ACB) []int {
	years := make([]int, len(doc.Years))
	for i, y := range doc.Years {
		years[i] = y.Year
	}
	return years
}

func Test_acb_reads_the_store_once_per_call(t *testing.T) {
	fake := &fakeStore{history: acbHistory()}
	stub := &configStub{cfg: acbConfig()}
	h := newHarness(t, fake, nil, mcp.WithConfig(stub.load), atSeptember29())

	decodeDoc[document.ACB](t, h.acb(t, map[string]any{"year": 2025, "security": []string{"sec-acme"}}))

	assert.Equal(t, 1, fake.historyReads)
}

func Test_acb_reads_today_once_per_call(t *testing.T) {
	clock := newMidnightClock()
	h := newHarness(t, &fakeStore{}, nil, mcp.WithClock(clock.now))

	first := decodeDoc[document.ACB](t, h.acb(t, map[string]any{"year": 2026}))
	second := decodeDoc[document.ACB](t, h.acb(t, map[string]any{"year": 2026}))

	assert.Equal(t, "2026-09-29", first.AsOf)
	assert.Equal(t, "2026-09-30", second.AsOf)
	assert.Equal(t, 2, clock.reads)
}

func Test_acb_reads_the_config_as_the_mcp_command_and_lists_its_warnings_first(t *testing.T) {
	stub := &configStub{cfg: config.Config{WarningsAbsolute: []string{configUnknownKeyWarning}}}
	h := newHarness(t, &fakeStore{}, nil, mcp.WithConfig(stub.load), atSeptember29())

	doc := decodeDoc[document.ACB](t, h.acb(t, map[string]any{}))

	assert.Equal(t, []string{"mcp"}, stub.commands)
	assert.Equal(t, []string{configUnknownKeyWarning, acbNothingToShow}, doc.Warnings)
}

func Test_acb_lists_every_year_and_security_the_pool_traded_when_the_call_names_none(t *testing.T) {
	h := newHarness(t, &fakeStore{history: acbHistory()}, nil, acbOptions()...)

	doc := decodeDoc[document.ACB](t, h.acb(t, map[string]any{}))

	assert.Nil(t, doc.Year)
	assert.Equal(t, []int{2025, 2026}, yearsOf(doc))
	assert.Equal(t, []string{"sec-acme", "sec-beta"}, securityIDs(doc))
}

func Test_acb_lists_only_the_year_the_call_names_this_year_included(t *testing.T) {
	cases := []struct {
		name      string
		year      int
		wantYears []int
		wantIDs   []string
	}{
		{name: "an earlier year", year: 2025, wantYears: []int{2025}, wantIDs: []string{"sec-acme"}},
		{name: "this year", year: 2026, wantYears: []int{2026}, wantIDs: []string{"sec-beta"}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t, &fakeStore{history: acbHistory()}, nil, acbOptions()...)

			doc := decodeDoc[document.ACB](t, h.acb(t, map[string]any{"year": c.year}))

			require.NotNil(t, doc.Year)
			assert.Equal(t, c.year, *doc.Year)
			assert.Equal(t, c.wantYears, yearsOf(doc))
			assert.Equal(t, c.wantIDs, securityIDs(doc))
		})
	}
}

func Test_acb_accepts_year_24_and_says_no_sale_was_in_it(t *testing.T) {
	h := newHarness(t, &fakeStore{history: acbHistory()}, nil, acbOptions()...)

	doc := decodeDoc[document.ACB](t, h.acb(t, map[string]any{"year": 24}))

	require.NotNil(t, doc.Year)
	assert.Equal(t, 24, *doc.Year)
	assert.Equal(t, []string{"no sales in 24 in non-registered accounts; the sales are in 2025–2026"}, doc.Warnings)
}

func Test_acb_lists_only_the_securities_the_call_names_by_id_ticker_or_name_in_any_case(t *testing.T) {
	cases := []struct {
		name     string
		security string
	}{
		{name: "id", security: "sec-acme"},
		{name: "ticker in lower case", security: "acme"},
		{name: "name in upper case", security: "ACME CORP"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t, &fakeStore{history: acbHistory()}, nil, acbOptions()...)

			doc := decodeDoc[document.ACB](t, h.acb(t, map[string]any{"security": []string{c.security}}))

			assert.Equal(t, []string{"sec-acme"}, securityIDs(doc))
			assert.Equal(t, []int{2025}, yearsOf(doc))
		})
	}
}

func Test_acb_lists_every_security_when_the_call_names_an_empty_list(t *testing.T) {
	h := newHarness(t, &fakeStore{history: acbHistory()}, nil, acbOptions()...)

	doc := decodeDoc[document.ACB](t, h.acb(t, map[string]any{"security": []string{}}))

	assert.Equal(t, []string{"sec-acme", "sec-beta"}, securityIDs(doc))
}

func Test_acb_applies_the_year_and_the_securities_together(t *testing.T) {
	h := newHarness(t, &fakeStore{history: acbHistory()}, nil, acbOptions()...)

	doc := decodeDoc[document.ACB](t, h.acb(t, map[string]any{"year": 2025, "security": []string{"sec-beta"}}))

	assert.Empty(t, securityIDs(doc))
	assert.Equal(t, []int{2025}, yearsOf(doc))
	assert.Equal(t, 0, doc.Years[0].SaleCount)
}

func Test_acb_warns_of_a_registered_only_security_the_call_names_beside_the_rest_of_the_report(t *testing.T) {
	h := newHarness(t, &fakeStore{history: acbHistory()}, nil, acbOptions()...)

	doc := decodeDoc[document.ACB](t, h.acb(t, map[string]any{"security": []string{"sec-maple"}}))

	assert.Empty(t, doc.Securities)
	assert.Equal(t, []string{acbMapleOnly}, doc.Warnings)
}

func Test_acb_maps_each_adjustment_the_config_lists_to_the_report(t *testing.T) {
	cfg := acbConfig()
	cfg.Adjustments = []config.Adjustment{{
		Security: "sec-acme", Date: time.Date(2025, time.March, 1, 0, 0, 0, 0, time.UTC), ReturnOfCapital: 10_000,
	}}
	h := newHarness(t, &fakeStore{history: acbHistory()}, nil, mcp.WithConfig((&configStub{cfg: cfg}).load), atSeptember29())

	doc := decodeDoc[document.ACB](t, h.acb(t, map[string]any{"security": []string{"sec-acme"}}))

	events := doc.Securities[0].Events
	require.Len(t, events, 3)
	assert.Equal(t, "return of capital", events[1].Action)
}

func Test_acb_refuses_with_the_stores_own_text_when_the_store_cannot_be_read(t *testing.T) {
	h := newHarness(t, &fakeStore{err: errDiskOnFire}, nil, acbOptions()...)

	result := h.acb(t, map[string]any{})

	assert.True(t, result.IsError)
	assert.Equal(t, errDiskOnFire.Error(), textOf(t, result))
}

func Test_acb_refuses_when_the_report_cannot_be_built(t *testing.T) {
	h := newHarness(t, &fakeStore{}, errFactoryBroke, acbOptions()...)

	result := h.acb(t, map[string]any{})

	assert.True(t, result.IsError)
	assert.Equal(t, errFactoryBroke.Error(), textOf(t, result))
}

const (
	acbLogPrefix       = "quarry: mcp: acb: "
	acbConfigLog       = "cannot read quarry's config file; run quarry acb to see why"
	acbSecurityLog     = "refused the call's security; details went to the client only"
	acbYearLog         = "refused the call's year; details went to the client only"
	acbArgumentsLog    = "refused the call's arguments; details went to the client only"
	acbUnclassifiedOne = "acb needs every brokerage and retirement account classified; 1 account is " +
		"in neither accounts.registered nor accounts.non-registered in ~/Library/Application Support/quarry/config.toml; " +
		"data_quality with type unclassified-account and status all lists it"
	acbUnclassifiedTwo = "acb needs every brokerage and retirement account classified; 2 accounts are " +
		"in neither accounts.registered nor accounts.non-registered in ~/Library/Application Support/quarry/config.toml; " +
		"data_quality with type unclassified-account and status all lists them"
)

func Test_acb_words_the_unclassified_refusal_for_a_tool_with_its_count(t *testing.T) {
	cases := []struct {
		name string
		cfg  config.Config
		want string
	}{
		{name: "one account lists it", cfg: config.Config{Registered: []string{"acct-r"}}, want: acbUnclassifiedOne},
		{name: "two accounts list them", cfg: config.Config{}, want: acbUnclassifiedTwo},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t, &fakeStore{history: acbHistory()}, nil, mcp.WithConfig((&configStub{cfg: c.cfg}).load), atSeptember29())

			result := h.acb(t, map[string]any{})

			assert.True(t, result.IsError)
			assert.Equal(t, c.want, textOf(t, result))
			assert.Equal(t, acbLogPrefix+c.want+"\n", h.stderr.String())
		})
	}
}

func Test_acb_refuses_unclassified_accounts_whose_finding_the_config_ignores(t *testing.T) {
	cfg := config.Config{Registered: []string{"acct-r"}, Ignore: []string{"unclassified-account:acct-n"}}
	h := newHarness(t, &fakeStore{history: acbHistory()}, nil, mcp.WithConfig((&configStub{cfg: cfg}).load), atSeptember29())

	result := h.acb(t, map[string]any{})

	assert.True(t, result.IsError)
	assert.Equal(t, acbUnclassifiedOne, textOf(t, result))
}

func Test_acb_names_the_first_security_that_names_none_without_it_reaching_stderr(t *testing.T) {
	cases := []struct {
		name       string
		securities []string
		want       string
	}{
		{
			name: "the first in argument order", securities: []string{"sec-acme", "XYZ", "ABC"},
			want: `acb covers no security named "XYZ"; acb with no arguments lists every security it covers`,
		},
		{
			name: "an empty name", securities: []string{""},
			want: `acb covers no security named ""; acb with no arguments lists every security it covers`,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t, &fakeStore{history: acbHistory()}, nil, acbOptions()...)

			result := h.acb(t, map[string]any{"security": c.securities})

			assert.True(t, result.IsError)
			assert.Equal(t, c.want, textOf(t, result))
			assert.Equal(t, acbLogPrefix+acbSecurityLog+"\n", h.stderr.String())
		})
	}
}

func Test_acb_refuses_a_year_after_this_one_without_it_reaching_stderr(t *testing.T) {
	h := newHarness(t, &fakeStore{history: acbHistory()}, nil, acbOptions()...)

	result := h.acb(t, map[string]any{"year": 2027})

	assert.True(t, result.IsError)
	assert.Equal(t, "year 2027 is after this year; pass this year or an earlier one", textOf(t, result))
	assert.Equal(t, acbLogPrefix+acbYearLog+"\n", h.stderr.String())
}

func Test_acb_refuses_a_year_before_it_reads_the_config_or_the_store(t *testing.T) {
	stub := &configStub{err: errBadConfig}
	fake := &fakeStore{history: acbHistory()}
	h := newHarness(t, fake, nil, mcp.WithConfig(stub.load), atSeptember29())

	result := h.acb(t, map[string]any{"year": 2027})

	assert.True(t, result.IsError)
	assert.Equal(t, acbLogPrefix+acbYearLog+"\n", h.stderr.String())
	assert.Empty(t, stub.commands)
	assert.Zero(t, h.built)
}

func Test_acb_refuses_an_unreadable_config_before_building_the_report(t *testing.T) {
	stub := &configStub{err: errBadConfig}
	h := newHarness(t, &fakeStore{}, errFactoryBroke, mcp.WithConfig(stub.load), atSeptember29())

	result := h.acb(t, map[string]any{})

	assert.True(t, result.IsError)
	assert.Equal(t, errBadConfig.Error(), textOf(t, result))
	assert.Zero(t, h.built)
	assert.Equal(t, acbLogPrefix+acbConfigLog+"\n", h.stderr.String())
}

func Test_acb_refuses_arguments_its_schema_does_not_allow(t *testing.T) {
	cases := []struct {
		name      string
		arguments map[string]any
	}{
		{name: "a year given as text", arguments: map[string]any{"year": "2024"}},
		{name: "year 0", arguments: map[string]any{"year": 0}},
		{name: "year 10000", arguments: map[string]any{"year": 10000}},
		{name: "a currency", arguments: map[string]any{"currency": "USD"}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			stub := &configStub{cfg: acbConfig()}
			h := newHarness(t, &fakeStore{history: acbHistory()}, nil, mcp.WithConfig(stub.load), atSeptember29())

			result := h.acb(t, c.arguments)

			assert.True(t, result.IsError)
			assert.Equal(t, acbLogPrefix+acbArgumentsLog+"\n", h.stderr.String())
			assert.Empty(t, stub.commands)
		})
	}
}

// acbBuys is count one-share buys of security in acct-n, one a day from the first of 2025, ids numbered from 1.
func acbBuys(security string, count int) []store.InvestmentTransaction {
	buys := make([]store.InvestmentTransaction, count)
	for i := range buys {
		day := time.Date(2025, time.January, 1+i, 0, 0, 0, 0, time.UTC)
		buys[i] = acbBuy(fmt.Sprintf("inv-%s-%d", security, i+1), int64(i+1), "acct-n", security, day, 1_000_000, 100)
	}
	return buys
}

// acbManyEvents is acbHistory's accounts with Acme and Beta, bought acme and beta times.
func acbManyEvents(acme, beta int) store.InvestmentHistory {
	history := acbHistory()
	history.Transactions = append(acbBuys("sec-acme", acme), acbBuys("sec-beta", beta)...)
	return history
}

func Test_acb_caps_events_at_500_across_securities(t *testing.T) {
	const capNote = "acb lists the first 500 events of %s; pass security to narrow"
	cases := []struct {
		name         string
		acme, beta   int
		wantCounts   []int
		wantWarnings []string
	}{
		{name: "500 events are listed whole", acme: 300, beta: 200, wantCounts: []int{300, 200}, wantWarnings: []string{}},
		{
			name: "501 events lose the last one", acme: 300, beta: 201, wantCounts: []int{300, 200},
			wantWarnings: []string{fmt.Sprintf(capNote, "501")},
		},
		{
			name: "a security past the cap leaves the next with its header and no events", acme: 600, beta: 634, wantCounts: []int{500, 0},
			wantWarnings: []string{fmt.Sprintf(capNote, "1,234")},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t, &fakeStore{history: acbManyEvents(c.acme, c.beta)}, nil, acbOptions()...)

			doc := decodeDoc[document.ACB](t, h.acb(t, map[string]any{}))

			require.Len(t, doc.Securities, 2)
			assert.Equal(t, c.wantCounts, []int{len(doc.Securities[0].Events), len(doc.Securities[1].Events)})
			assert.NotNil(t, doc.Securities[0].Events)
			assert.NotNil(t, doc.Securities[1].Events)
			assert.Equal(t, c.wantWarnings, doc.Warnings)
		})
	}
}

func Test_acb_keeps_the_header_of_a_security_whose_events_the_cap_cut_all(t *testing.T) {
	h := newHarness(t, &fakeStore{history: acbManyEvents(500, 1)}, nil, acbOptions()...)

	doc := decodeDoc[document.ACB](t, h.acb(t, map[string]any{}))

	assert.Equal(t, []string{"sec-acme", "sec-beta"}, securityIDs(doc))
	assert.Equal(t, []document.ACBEvent{}, doc.Securities[1].Events)
	assert.Equal(t, "Beta Fund", doc.Securities[1].Security)
	assert.Equal(t, "1.000000", doc.Securities[1].Shares)
}

func Test_acb_ends_its_warnings_with_the_cap_line(t *testing.T) {
	cfg := acbConfig()
	cfg.WarningsAbsolute = []string{configUnknownKeyWarning}
	h := newHarness(t, &fakeStore{history: acbManyEvents(300, 201)}, nil, mcp.WithConfig((&configStub{cfg: cfg}).load), atSeptember29())

	doc := decodeDoc[document.ACB](t, h.acb(t, map[string]any{}))

	assert.Equal(t, []string{configUnknownKeyWarning, "acb lists the first 500 events of 501; pass security to narrow"}, doc.Warnings)
}

func Test_acb_counts_only_the_securities_the_call_names_against_the_cap(t *testing.T) {
	h := newHarness(t, &fakeStore{history: acbManyEvents(300, 201)}, nil, acbOptions()...)

	doc := decodeDoc[document.ACB](t, h.acb(t, map[string]any{"security": []string{"sec-acme"}}))

	require.Len(t, doc.Securities, 1)
	assert.Len(t, doc.Securities[0].Events, 300)
	assert.Equal(t, []string{}, doc.Warnings)
}
