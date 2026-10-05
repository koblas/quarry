package report_test

import (
	"testing"

	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
)

// acbTickered is a CAD security with the given ticker, nil when ticker is nil.
func acbTickered(id, name string, ticker *string) store.Security {
	currency := "CAD"
	return store.Security{ID: id, Name: name, Ticker: ticker, Currency: &currency}
}

// acbBoughtIn is the walk of one CAD buy of each security, security i in accounts[i].
func acbBoughtIn(t *testing.T, securities []store.Security, accounts ...string) report.ACB {
	t.Helper()
	txs := make([]store.InvestmentTransaction, len(securities))
	for i, security := range securities {
		txs[i] = acbTx(t, int64(i+1), accounts[i], security.ID, "2024-01-05", store.ActionBuy, "CAD", acbMillion, -100)
	}

	return acbWalkRequest(t, securities, nil, nil, txs...)
}

// acbSharedNames is the names of each shared group, under its ticker.
func acbSharedNames(a report.ACB) map[string][]string {
	names := make(map[string][]string)
	for _, group := range a.SharedTickers() {
		for _, security := range group.Securities {
			names[group.Ticker] = append(names[group.Ticker], security.Name)
		}
	}

	return names
}

func Test_acb_shared_tickers_match_exactly_and_ignore_an_empty_ticker(t *testing.T) {
	cases := []struct {
		name       string
		securities []store.Security
		want       map[string][]string
	}{
		{
			name:       "tickers differing only by case are not shared",
			securities: []store.Security{acbTickered("sec-1", "Upper", new("VTI")), acbTickered("sec-2", "Lower", new("vti"))},
			want:       map[string][]string{},
		},
		{
			name:       "two securities with an empty ticker are not shared",
			securities: []store.Security{acbTickered("sec-1", "One", new("")), acbTickered("sec-2", "Two", new(""))},
			want:       map[string][]string{},
		},
		{
			name:       "two securities with no ticker are not shared",
			securities: []store.Security{acbTickered("sec-1", "One", nil), acbTickered("sec-2", "Two", nil)},
			want:       map[string][]string{},
		},
		{
			name:       "a ticker held by one security is not shared",
			securities: []store.Security{acbTickered("sec-1", "One", new("VTI")), acbTickered("sec-2", "Two", new("XEQT"))},
			want:       map[string][]string{},
		},
		{
			name: "two securities with one ticker are one group of two",
			securities: []store.Security{
				acbTickered("sec-1", "Alpha", new("VTI")), acbTickered("sec-2", "Beta", new("VTI")),
			},
			want: map[string][]string{"VTI": {"Alpha", "Beta"}},
		},
		{
			name: "three securities with one ticker are one group of three in security order",
			securities: []store.Security{
				acbTickered("sec-1", "Alpha", new("VTI")), acbTickered("sec-2", "Beta", new("VTI")), acbTickered("sec-3", "Gamma", new("VTI")),
			},
			want: map[string][]string{"VTI": {"Alpha", "Beta", "Gamma"}},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := acbBoughtIn(t, c.securities, "acct-1", "acct-1", "acct-1")

			assert.Equal(t, c.want, acbSharedNames(got))
		})
	}
}

func Test_acb_shared_tickers_order_groups_by_their_first_member(t *testing.T) {
	securities := []store.Security{
		acbTickered("sec-1", "Alpha", new("XEQT")), acbTickered("sec-2", "Beta", new("VTI")),
		acbTickered("sec-3", "Gamma", new("VTI")), acbTickered("sec-4", "Delta", new("XEQT")),
	}
	got := acbBoughtIn(t, securities, "acct-1", "acct-1", "acct-1", "acct-1")

	groups := got.SharedTickers()

	assert.Equal(t, []string{"XEQT", "VTI"}, []string{groups[0].Ticker, groups[1].Ticker})
	assert.Equal(t, []string{"Alpha", "Delta"}, []string{groups[0].Securities[0].Name, groups[0].Securities[1].Name})
}

func Test_acb_shared_tickers_leave_out_a_security_held_only_in_a_registered_account(t *testing.T) {
	securities := []store.Security{acbTickered("sec-1", "Alpha", new("VTI")), acbTickered("sec-2", "Beta", new("VTI"))}

	got := acbBoughtIn(t, securities, "acct-1", "acct-9")

	assert.Empty(t, got.SharedTickers())
}
