package duckstore_test

import (
	"math/big"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/store"
	"github.com/koblas/quarry/internal/store/duckstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	tierAccountA = "acct-a"
	tierAccountB = "acct-b"
)

// heldPair is an account and a security it holds one share of.
type heldPair struct{ account, security string }

func brokerage(id string, sourceID int64, name string) store.Account {
	return store.Account{ID: id, SourceID: sourceID, Name: name, Type: store.AccountTypeBrokerage, Currency: "CAD", Active: true}
}

func fund(id string, sourceID int64, name string) store.Security {
	return store.Security{ID: id, SourceID: sourceID, Name: name, Currency: new("CAD")}
}

// holdingsOn reads st's holdings on date.
func holdingsOn(t *testing.T, st *duckstore.Store, date time.Time) []store.Holding {
	t.Helper()
	got, err := st.Holdings(t.Context(), store.HoldingsParams{AsOf: date})
	require.NoError(t, err)
	return got.Holdings
}

// holdingOrder is "account/security" ids of the holdings of held on 2026-03-02, in the order Holdings returns them.
func holdingOrder(t *testing.T, accounts []store.Account, securities []store.Security, held []heldPair) []string {
	t.Helper()
	txns := make([]store.InvestmentTransaction, len(held))
	for i, h := range held {
		txns[i] = buy(h.account, h.security, int64(i+1), marchDay(1), oneShare)
	}
	rows := holdingRows(txns...)
	rows.Accounts, rows.Securities = accounts, securities
	st := newStoreWith(t, rows)
	listed := holdingsOn(t, st, marchDay(2))
	order := make([]string, 0, len(listed))
	for _, h := range listed {
		order = append(order, h.AccountID+"/"+h.SecurityID)
	}
	return order
}

func Test_holdings_reads_every_column_of_a_priced_converted_holding(t *testing.T) {
	t.Parallel()
	rows := holdingRows(buy(acctTwo, secUSD, 1, marchDay(1), 2_500_000))
	rows.Accounts[1].Closed = true
	rows.Prices = []store.Price{quote(secUSD, 1, marchDay(3), 12_345_678)}
	st := newStoreWithRates(t, rows, ratesOn(1, 1_250_000, "FXUSDCAD"))

	got := holdingsOn(t, st, marchDay(5))

	assert.Equal(t, []store.Holding{{
		AccountID: acctTwo, SecurityID: secUSD, Account: "Brokerage USD", AccountSourceID: 2, AccountClosed: true,
		Security: new("Globex Inc"), Ticker: new("GLBX"), Currency: new("USD"), SecuritySourceID: new(int64(2)),
		Shares: 2_500_000, Price: new(int64(12_345_678)), PriceDate: new(marchDay(3)),
		Value: big.NewInt(3086), ValueCAD: big.NewInt(3858), ValueUSD: big.NewInt(3086), USDCAD: money.Rate(1_250_000),
	}}, got)
}

func Test_holdings_leaves_nil_what_the_store_holds_as_null(t *testing.T) {
	t.Parallel()
	st := newStoreWith(t, holdingRows(buy(acctOne, secNoCurrency, 1, marchDay(1), oneShare)))

	got := holdingsOn(t, st, marchDay(2))

	assert.Equal(t, []store.Holding{{
		AccountID: acctOne, SecurityID: secNoCurrency, Account: "Chequing", AccountSourceID: 1,
		Security: new("Plain Fund"), SecuritySourceID: new(int64(4)), Shares: oneShare,
	}}, got)
}

func Test_holdings_lists_a_holding_whose_security_and_account_rows_are_missing(t *testing.T) {
	t.Parallel()
	st := newStoreWith(t, holdingRows(buy("acct-ghost", secGhost, 1, marchDay(1), oneShare)))

	got := holdingsOn(t, st, marchDay(2))

	assert.Equal(t, []store.Holding{{AccountID: "acct-ghost", SecurityID: secGhost, Shares: oneShare}}, got)
}

func Test_holdings_orders_by_account_then_security_and_breaks_each_tie_in_turn(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		accounts   []store.Account
		securities []store.Security
		held       []heldPair
		want       []string
	}{
		{
			name:       "account name before account source id",
			accounts:   []store.Account{brokerage(tierAccountA, 1, "Beta"), brokerage(tierAccountB, 2, "Alpha")},
			securities: []store.Security{fund(secAcme, 1, "Acme")},
			held:       []heldPair{{tierAccountA, secAcme}, {tierAccountB, secAcme}},
			want:       []string{tierAccountB + "/" + secAcme, tierAccountA + "/" + secAcme},
		},
		{
			name:       "account source id when two accounts share a name",
			accounts:   []store.Account{brokerage("acct-first-id", 2, "Same"), brokerage("acct-second-id", 1, "Same")},
			securities: []store.Security{fund(secAcme, 1, "Acme")},
			held:       []heldPair{{"acct-first-id", secAcme}, {"acct-second-id", secAcme}},
			want:       []string{"acct-second-id/" + secAcme, "acct-first-id/" + secAcme},
		},
		{
			name:       "an account's holdings stay together when accounts are missing",
			accounts:   nil,
			securities: []store.Security{fund(secAcme, 1, "Acme"), fund(secUSD, 2, "Beta")},
			held:       []heldPair{{"ghost-b", secAcme}, {"ghost-a", secUSD}, {"ghost-b", secUSD}, {"ghost-a", secAcme}},
			want:       []string{"ghost-a/" + secAcme, "ghost-a/" + secUSD, "ghost-b/" + secAcme, "ghost-b/" + secUSD},
		},
		{
			name:       "security name before security source id",
			accounts:   []store.Account{brokerage(tierAccountA, 1, "Brokerage")},
			securities: []store.Security{fund(secAcme, 1, "Zeta"), fund(secUSD, 2, "Alpha")},
			held:       []heldPair{{tierAccountA, secAcme}, {tierAccountA, secUSD}},
			want:       []string{tierAccountA + "/" + secUSD, tierAccountA + "/" + secAcme},
		},
		{
			name:       "security source id when two securities share a name",
			accounts:   []store.Account{brokerage(tierAccountA, 1, "Brokerage")},
			securities: []store.Security{fund("sec-first-id", 2, "Same"), fund("sec-second-id", 1, "Same")},
			held:       []heldPair{{tierAccountA, "sec-first-id"}, {tierAccountA, "sec-second-id"}},
			want:       []string{tierAccountA + "/sec-second-id", tierAccountA + "/sec-first-id"},
		},
		{
			name:       "security id when two securities are missing",
			accounts:   []store.Account{brokerage(tierAccountA, 1, "Brokerage")},
			securities: nil,
			held:       []heldPair{{tierAccountA, "sec-ghost-b"}, {tierAccountA, "sec-ghost-a"}},
			want:       []string{tierAccountA + "/sec-ghost-a", tierAccountA + "/sec-ghost-b"},
		},
		{
			name:       "names compare as plain text, capitals before lower case",
			accounts:   []store.Account{brokerage(tierAccountA, 1, "alpha"), brokerage(tierAccountB, 2, "Zulu")},
			securities: []store.Security{fund(secAcme, 1, "Acme")},
			held:       []heldPair{{tierAccountA, secAcme}, {tierAccountB, secAcme}},
			want:       []string{tierAccountB + "/" + secAcme, tierAccountA + "/" + secAcme},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			got := holdingOrder(t, c.accounts, c.securities, c.held)

			assert.Equal(t, c.want, got)
		})
	}
}

func Test_holdings_is_empty_on_a_day_nothing_is_held(t *testing.T) {
	t.Parallel()
	st := newStoreWith(t, holdingRows(
		buy(acctOne, secAcme, 1, marchDay(1), oneShare), buy(acctOne, secAcme, 2, marchDay(3), -oneShare)))

	held := holdingsOn(t, st, marchDay(2))
	sold := holdingsOn(t, st, marchDay(3))

	assert.Len(t, held, 1)
	assert.Empty(t, sold)
}

func Test_holdings_is_empty_on_a_day_after_today(t *testing.T) {
	t.Parallel()
	today := localToday()
	st := newStoreWith(t, holdingRows(buy(acctOne, secAcme, 1, today.AddDate(0, 0, -1), oneShare)))

	now := holdingsOn(t, st, today)
	tomorrow := holdingsOn(t, st, today.AddDate(0, 0, 1))

	assert.Len(t, now, 1)
	assert.Empty(t, tomorrow)
}

func Test_holdings_reads_the_largest_holding_in_exact_cents(t *testing.T) {
	t.Parallel()
	rows := holdingRows(
		buy(acctOne, secUSD, 1, marchDay(1), maxDecimal18x6), buy(acctTwo, secUSD, 2, marchDay(1), -maxDecimal18x6))
	rows.Prices = []store.Price{quote(secUSD, 1, marchDay(1), maxDecimal18x6)}
	st := newStoreWithRates(t, rows, ratesOn(1, 1_250_000, "FXUSDCAD"))
	cents := func(text string) *big.Int {
		n, _ := new(big.Int).SetString(text, 10)
		return n
	}

	got := holdingsOn(t, st, marchDay(1))

	require.Len(t, got, 2)
	// "Brokerage USD" (acct-2, negative) sorts before "Chequing" (acct-1).
	assert.Equal(t, []*big.Int{cents("99999999999999999800000000"), cents("99999999999999999800000000"), cents("124999999999999999750000000")},
		[]*big.Int{got[1].Value, got[1].ValueUSD, got[1].ValueCAD})
	assert.Equal(t, []*big.Int{cents("-99999999999999999800000000"), cents("-99999999999999999800000000"), cents("-124999999999999999750000000")},
		[]*big.Int{got[0].Value, got[0].ValueUSD, got[0].ValueCAD})
}
