package mcp_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/config"
	"github.com/koblas/quarry/internal/mcp"
	"github.com/koblas/quarry/internal/report/document"
	"github.com/koblas/quarry/internal/store"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	acbNothingToShow = "no non-registered account has bought or sold a security; quarry acb has nothing to show"
	acbMapleOnly     = `"Maple Fund" is held only in registered accounts, so it has no ACB`
)

// atACBToday is the clock option that makes today 2026-09-29.
func atACBToday() mcp.Option {
	return mcp.WithClock(func() time.Time { return time.Date(2026, time.September, 29, 12, 0, 0, 0, time.UTC) })
}

// acbConfig lists acct-n as non-registered and acct-r as registered.
func acbConfig() config.Config {
	return config.Config{NonRegistered: []string{"acct-n"}, Registered: []string{"acct-r"}}
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

// decodeACB is result's one text block decoded as the acb document.
func decodeACB(t *testing.T, result *sdk.CallToolResult) document.ACB {
	t.Helper()
	require.False(t, result.IsError, textOf(t, result))
	var doc document.ACB
	require.NoError(t, json.Unmarshal([]byte(textOf(t, result)), &doc))
	return doc
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
	h := newHarness(t, fake, nil, mcp.WithConfig(stub.load), atACBToday())

	decodeACB(t, h.acb(t, map[string]any{"year": 2025, "security": []string{"sec-acme"}}))

	assert.Equal(t, 1, fake.historyReads)
}

func Test_acb_reads_today_once_per_call(t *testing.T) {
	clock := &steppingClock{times: []time.Time{
		time.Date(2026, time.September, 29, 23, 59, 0, 0, time.UTC),
		time.Date(2026, time.September, 30, 0, 1, 0, 0, time.UTC),
	}}
	h := newHarness(t, &fakeStore{}, nil, mcp.WithClock(clock.now))

	first := decodeACB(t, h.acb(t, map[string]any{"year": 2026}))
	second := decodeACB(t, h.acb(t, map[string]any{"year": 2026}))

	assert.Equal(t, "2026-09-29", first.AsOf)
	assert.Equal(t, "2026-09-30", second.AsOf)
	assert.Equal(t, 2, clock.reads)
}

func Test_acb_reads_the_config_as_the_mcp_command_and_lists_its_warnings_first(t *testing.T) {
	stub := &configStub{cfg: config.Config{WarningsAbsolute: []string{configUnknownKeyWarning}}}
	h := newHarness(t, &fakeStore{}, nil, mcp.WithConfig(stub.load), atACBToday())

	doc := decodeACB(t, h.acb(t, map[string]any{}))

	assert.Equal(t, []string{"mcp"}, stub.commands)
	assert.Equal(t, []string{configUnknownKeyWarning, acbNothingToShow}, doc.Warnings)
}

func Test_acb_lists_every_year_and_security_the_pool_traded_when_the_call_names_none(t *testing.T) {
	h := newHarness(t, &fakeStore{history: acbHistory()}, nil, mcp.WithConfig((&configStub{cfg: acbConfig()}).load), atACBToday())

	doc := decodeACB(t, h.acb(t, map[string]any{}))

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
			h := newHarness(t, &fakeStore{history: acbHistory()}, nil, mcp.WithConfig((&configStub{cfg: acbConfig()}).load), atACBToday())

			doc := decodeACB(t, h.acb(t, map[string]any{"year": c.year}))

			require.NotNil(t, doc.Year)
			assert.Equal(t, c.year, *doc.Year)
			assert.Equal(t, c.wantYears, yearsOf(doc))
			assert.Equal(t, c.wantIDs, securityIDs(doc))
		})
	}
}

func Test_acb_accepts_year_24_and_says_no_sale_was_in_it(t *testing.T) {
	h := newHarness(t, &fakeStore{history: acbHistory()}, nil, mcp.WithConfig((&configStub{cfg: acbConfig()}).load), atACBToday())

	doc := decodeACB(t, h.acb(t, map[string]any{"year": 24}))

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
			h := newHarness(t, &fakeStore{history: acbHistory()}, nil, mcp.WithConfig((&configStub{cfg: acbConfig()}).load), atACBToday())

			doc := decodeACB(t, h.acb(t, map[string]any{"security": []string{c.security}}))

			assert.Equal(t, []string{"sec-acme"}, securityIDs(doc))
			assert.Equal(t, []int{2025}, yearsOf(doc))
		})
	}
}

func Test_acb_lists_every_security_when_the_call_names_an_empty_list(t *testing.T) {
	h := newHarness(t, &fakeStore{history: acbHistory()}, nil, mcp.WithConfig((&configStub{cfg: acbConfig()}).load), atACBToday())

	doc := decodeACB(t, h.acb(t, map[string]any{"security": []string{}}))

	assert.Equal(t, []string{"sec-acme", "sec-beta"}, securityIDs(doc))
}

func Test_acb_applies_the_year_and_the_securities_together(t *testing.T) {
	h := newHarness(t, &fakeStore{history: acbHistory()}, nil, mcp.WithConfig((&configStub{cfg: acbConfig()}).load), atACBToday())

	doc := decodeACB(t, h.acb(t, map[string]any{"year": 2025, "security": []string{"sec-beta"}}))

	assert.Empty(t, securityIDs(doc))
	assert.Equal(t, []int{2025}, yearsOf(doc))
	assert.Equal(t, 0, doc.Years[0].SaleCount)
}

func Test_acb_warns_of_a_registered_only_security_the_call_names_beside_the_rest_of_the_report(t *testing.T) {
	h := newHarness(t, &fakeStore{history: acbHistory()}, nil, mcp.WithConfig((&configStub{cfg: acbConfig()}).load), atACBToday())

	doc := decodeACB(t, h.acb(t, map[string]any{"security": []string{"sec-maple"}}))

	assert.Empty(t, doc.Securities)
	assert.Equal(t, []string{acbMapleOnly}, doc.Warnings)
}

func Test_acb_maps_each_adjustment_the_config_lists_to_the_report(t *testing.T) {
	cfg := acbConfig()
	cfg.Adjustments = []config.Adjustment{{
		Security: "sec-acme", Date: time.Date(2025, time.March, 1, 0, 0, 0, 0, time.UTC), ReturnOfCapital: 10_000,
	}}
	h := newHarness(t, &fakeStore{history: acbHistory()}, nil, mcp.WithConfig((&configStub{cfg: cfg}).load), atACBToday())

	doc := decodeACB(t, h.acb(t, map[string]any{"security": []string{"sec-acme"}}))

	events := doc.Securities[0].Events
	require.Len(t, events, 3)
	assert.Equal(t, "return of capital", events[1].Action)
}

func Test_acb_refuses_with_the_stores_own_text_when_the_store_cannot_be_read(t *testing.T) {
	h := newHarness(t, &fakeStore{err: errDiskOnFire}, nil, mcp.WithConfig((&configStub{cfg: acbConfig()}).load), atACBToday())

	result := h.acb(t, map[string]any{})

	assert.True(t, result.IsError)
	assert.Equal(t, errDiskOnFire.Error(), textOf(t, result))
}

func Test_acb_refuses_when_the_report_cannot_be_built(t *testing.T) {
	h := newHarness(t, &fakeStore{}, errFactoryBroke, mcp.WithConfig((&configStub{cfg: acbConfig()}).load), atACBToday())

	result := h.acb(t, map[string]any{})

	assert.True(t, result.IsError)
	assert.Equal(t, errFactoryBroke.Error(), textOf(t, result))
}
