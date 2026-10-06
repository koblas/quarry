package report_test

import (
	"errors"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func selectSecurity(id, name, ticker string) store.Security {
	currency := "CAD"
	return store.Security{ID: id, Name: name, Ticker: &ticker, Currency: &currency}
}

// selectHistory is securities bought in non-registered acct-1 (shared tickers and names, no or empty ticker), sec-3
// and sec-13 only in registered acct-9, sec-5 there the day after today, sec-9 only in acct-7, in neither list, sec-4 never.
func selectHistory(t *testing.T) store.InvestmentHistory {
	t.Helper()
	empty := ""
	cad := "CAD"
	blank := store.Security{ID: "sec-8", Name: "Blank", Ticker: &empty}
	tickerless := store.Security{ID: "sec-10", Name: "No Ticker", Currency: &cad}
	return store.InvestmentHistory{
		Accounts: append(acbAccounts(), store.Account{ID: "acct-7", Name: "Cash margin", Type: "chequing", Currency: "CAD"}),
		Securities: []store.Security{
			selectSecurity("sec-1", "Acme Corp", "ACME"),
			selectSecurity("sec-2", "Beta Inc", "BETA"),
			selectSecurity("sec-3", "Maple", "MPL"),
			selectSecurity("sec-4", "Adjusted", "ADJ"),
			selectSecurity("sec-5", "Future", "FUT"),
			selectSecurity("sec-6", "Acme Preferred", "acme"),
			selectSecurity("sec-7", "sec-2", "SEC7"),
			blank,
			selectSecurity("sec-9", "Cash only", "CSH"),
			tickerless,
			selectSecurity("sec-11", "beta Fund", "BFD"),
			selectSecurity("sec-b", "Twin", "TWB"),
			selectSecurity("sec-a", "Twin", "TWA"),
			selectSecurity("sec-13", "Today Fund", "TDY"),
		},
		Transactions: []store.InvestmentTransaction{
			acbTx(t, 1, "acct-1", "sec-1", "2024-01-02", store.ActionBuy, "CAD", 10*acbMillion, -10_000),
			acbTx(t, 2, "acct-1", "sec-2", "2024-01-02", store.ActionBuy, "CAD", 10*acbMillion, -10_000),
			acbTx(t, 3, "acct-9", "sec-3", "2024-01-02", store.ActionBuy, "CAD", 10*acbMillion, -10_000),
			acbTx(t, 4, "acct-9", "sec-5", "2026-10-06", store.ActionBuy, "CAD", 10*acbMillion, -10_000),
			acbTx(t, 5, "acct-1", "sec-6", "2024-01-02", store.ActionBuy, "CAD", 10*acbMillion, -10_000),
			acbTx(t, 6, "acct-1", "sec-7", "2024-01-02", store.ActionBuy, "CAD", 10*acbMillion, -10_000),
			acbTx(t, 7, "acct-1", "sec-8", "2024-01-02", store.ActionBuy, "CAD", 10*acbMillion, -10_000),
			acbTx(t, 8, "acct-7", "sec-9", "2024-01-02", store.ActionBuy, "CAD", 10*acbMillion, -10_000),
			acbTx(t, 9, "acct-1", "sec-10", "2024-01-02", store.ActionBuy, "CAD", 10*acbMillion, -10_000),
			acbTx(t, 10, "acct-1", "sec-11", "2024-01-02", store.ActionBuy, "CAD", 10*acbMillion, -10_000),
			acbTx(t, 11, "acct-1", "sec-b", "2024-01-02", store.ActionBuy, "CAD", 10*acbMillion, -10_000),
			acbTx(t, 12, "acct-1", "sec-a", "2024-01-02", store.ActionBuy, "CAD", 10*acbMillion, -10_000),
			acbTx(t, 13, "acct-9", "sec-13", acbToday.Format(time.DateOnly), store.ActionBuy, "CAD", 10*acbMillion, -10_000),
		},
	}
}

func selectACB(t *testing.T, history store.InvestmentHistory, selectors ...string) (report.ACB, error) {
	t.Helper()
	srv := report.NewServer(report.WithStore(fakeStore{history: history}))

	return srv.ACB(t.Context(), report.ACBRequest{Classification: acbClassification(), Today: acbToday, Securities: selectors})
}

func securityIDsOf(securities []store.Security) []string {
	ids := make([]string, 0, len(securities))
	for _, s := range securities {
		ids = append(ids, s.ID)
	}

	return ids
}

func Test_acb_selects_the_securities_a_selector_names(t *testing.T) {
	cases := []struct {
		name      string
		selectors []string
		want      []string
	}{
		{name: "an id", selectors: []string{"sec-1"}, want: []string{"sec-1"}},
		{name: "an id that is also another security's name names that security alone", selectors: []string{"sec-2"}, want: []string{"sec-2"}},
		{name: "a ticker ignoring case", selectors: []string{"beta"}, want: []string{"sec-2"}},
		{name: "a name ignoring case", selectors: []string{"beta inc"}, want: []string{"sec-2"}},
		{name: "a ticker two securities share names both in the walk's order", selectors: []string{"ACME"}, want: []string{"sec-1", "sec-6"}},
		{name: "a name ignoring case, though a security has no ticker", selectors: []string{"no ticker"}, want: []string{"sec-10"}},
		{name: "a ticker, though a security has no ticker", selectors: []string{"bfd"}, want: []string{"sec-11"}},
		{name: "a lowercase name sorts among the capitalised ones by its folded spelling", selectors: []string{"BFD", "BETA"}, want: []string{"sec-11", "sec-2"}},
		{name: "several selectors list the securities in the walk's order, not the request's", selectors: []string{"BETA", "sec-1"}, want: []string{"sec-1", "sec-2"}},
		{name: "a security named by two selectors is listed once", selectors: []string{"ACME", "sec-1"}, want: []string{"sec-1", "sec-6"}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := selectACB(t, selectHistory(t), c.selectors...)

			require.NoError(t, err)
			assert.True(t, got.Selected)
			assert.Equal(t, c.want, got.SelectedIDs)
		})
	}
}

func Test_acb_selects_nothing_when_the_request_names_no_security(t *testing.T) {
	got, err := selectACB(t, selectHistory(t))

	require.NoError(t, err)
	assert.False(t, got.Selected)
	assert.Empty(t, got.SelectedIDs)
	assert.Empty(t, got.RegisteredOnly)
}

func Test_acb_keeps_the_whole_walk_when_it_selects(t *testing.T) {
	history := selectHistory(t)
	all, err := selectACB(t, history)
	require.NoError(t, err)

	got, err := selectACB(t, history, "sec-1")

	require.NoError(t, err)
	assert.Equal(t, securityIDs(all), securityIDs(got))
	assert.Equal(t, all.Years, got.Years)
}

func Test_acb_refuses_a_selector_that_names_no_covered_security(t *testing.T) {
	cases := []struct {
		name     string
		selector string
	}{
		{name: "no id, ticker or name matches", selector: "XYZ"},
		{name: "an empty selector, though a security has an empty ticker", selector: ""},
		{name: "a security with no transaction", selector: "sec-4"},
		{name: "a security bought in a registered account the day after today", selector: "sec-5"},
		{name: "a security bought only in an account in neither list", selector: "sec-9"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := selectACB(t, selectHistory(t), c.selector)

			refusal, ok := errors.AsType[report.RefusalError](err)
			require.True(t, ok)
			assert.Equal(t, report.RefusalUnknownSecurity, refusal.Kind)
			assert.Equal(t, c.selector, refusal.Arg)
		})
	}
}

func Test_acb_refusal_words_the_security_the_way_the_command_prints_it(t *testing.T) {
	_, err := selectACB(t, selectHistory(t), "XYZ")

	assert.EqualError(t, err, `acb covers no security named "XYZ"; quarry acb --json lists every security it covers`)
}

func Test_acb_refuses_the_first_selector_in_request_order_that_names_nothing_though_others_match(t *testing.T) {
	got, err := selectACB(t, selectHistory(t), "sec-1", "NOPE", "ALSO", "sec-2")

	refusal, ok := errors.AsType[report.RefusalError](err)
	require.True(t, ok)
	assert.Equal(t, "NOPE", refusal.Arg)
	assert.Empty(t, got.Securities)
}

func Test_acb_lists_a_selected_security_held_only_in_registered_accounts(t *testing.T) {
	got, err := selectACB(t, selectHistory(t), "MPL")

	require.NoError(t, err)
	assert.Equal(t, []string{"sec-3"}, got.SelectedIDs)
	assert.Equal(t, []string{"sec-3"}, securityIDsOf(got.RegisteredOnly))
}

func Test_acb_counts_a_registered_account_transaction_dated_today_as_held(t *testing.T) {
	got, err := selectACB(t, selectHistory(t), "TDY")

	require.NoError(t, err)
	assert.Equal(t, []string{"sec-13"}, got.SelectedIDs)
	assert.Equal(t, []string{"sec-13"}, securityIDsOf(got.RegisteredOnly))
}

func Test_acb_lists_securities_sharing_a_name_by_id_whatever_order_the_request_names_them(t *testing.T) {
	for range 20 {
		for _, selectors := range [][]string{{"TWB", "TWA"}, {"TWA", "TWB"}} {
			got, err := selectACB(t, selectHistory(t), selectors...)

			require.NoError(t, err)
			assert.Equal(t, []string{"sec-a", "sec-b"}, got.SelectedIDs)
		}
	}
}

func Test_acb_ignores_a_registered_account_transaction_with_no_security(t *testing.T) {
	history := selectHistory(t)
	noSecurity := acbTx(t, 14, "acct-9", "sec-3", "2024-03-01", store.ActionBuy, "CAD", acbMillion, -1_000)
	noSecurity.SecurityID = nil
	history.Transactions = append(history.Transactions, noSecurity)

	got, err := selectACB(t, history, "MPL", "TDY")

	require.NoError(t, err)
	assert.Equal(t, []string{"sec-3", "sec-13"}, got.SelectedIDs)
	assert.Equal(t, []string{"sec-3", "sec-13"}, securityIDsOf(got.RegisteredOnly))
}

func Test_acb_does_not_list_a_selected_security_with_pool_events_as_registered_only(t *testing.T) {
	history := selectHistory(t)
	history.Transactions = append(history.Transactions, acbTx(t, 9, "acct-1", "sec-3", "2024-02-01", store.ActionBuy, "CAD", acbMillion, -1_000))

	got, err := selectACB(t, history, "MPL")

	require.NoError(t, err)
	assert.Equal(t, []string{"sec-3"}, got.SelectedIDs)
	assert.Empty(t, got.RegisteredOnly)
}

func Test_acb_lists_ids_of_pooled_and_registered_only_securities_together_in_the_walk_order(t *testing.T) {
	got, err := selectACB(t, selectHistory(t), "MPL", "sec-1")

	require.NoError(t, err)
	assert.Equal(t, []string{"sec-1", "sec-3"}, got.SelectedIDs)
	assert.Equal(t, []string{"sec-3"}, securityIDsOf(got.RegisteredOnly))
}
