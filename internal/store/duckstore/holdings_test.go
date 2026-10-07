package duckstore_test

import (
	"math/big"
	"strings"
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
	return holdingsRead(t, st, date).Holdings
}

// holdingOrder is "account/security" ids of the holdings of held on 2026-03-02, in the order Holdings returns them.
func holdingOrder(t *testing.T, accounts []store.Account, securities []store.Security, held []heldPair) []string {
	t.Helper()
	txns := make([]store.InvestmentTransaction, len(held))
	for i, h := range held {
		txns[i] = buy(h.account, h.security, int64(i+1), march(1), oneShare)
	}
	rows := holdingRows(txns...)
	rows.Accounts, rows.Securities = accounts, securities
	st := newStoreWith(t, rows)
	listed := holdingsOn(t, st, march(2))
	order := make([]string, 0, len(listed))
	for _, h := range listed {
		order = append(order, h.AccountID+"/"+h.SecurityID)
	}
	return order
}

func Test_holdings_reads_every_column_of_a_priced_converted_holding(t *testing.T) {
	t.Parallel()
	rows := holdingRows(buy(acctTwo, secUSD, 1, march(1), 2_500_000))
	rows.Accounts[1].Closed = true
	rows.Prices = []store.Price{quote(secUSD, 1, march(3), 12_345_678)}
	st := newStoreWithRates(t, rows, ratesOn(1, 1_250_000, "FXUSDCAD"))

	got := holdingsOn(t, st, march(5))

	assert.Equal(t, []store.Holding{{
		AccountID: acctTwo, SecurityID: secUSD, Account: "Brokerage USD", AccountSourceID: 2, AccountClosed: true,
		Security: new("Globex Inc"), Ticker: new("GLBX"), Currency: new("USD"), SecuritySourceID: new(int64(2)),
		Shares: 2_500_000, Price: new(int64(12_345_678)), PriceDate: new(march(3)),
		Value: big.NewInt(3086), ValueCAD: big.NewInt(3858), ValueUSD: big.NewInt(3086), USDCAD: money.Rate(1_250_000),
	}}, got)
}

func Test_holdings_leaves_nil_what_the_store_holds_as_null(t *testing.T) {
	t.Parallel()
	st := newStoreWith(t, holdingRows(buy(acctOne, secNoCurrency, 1, march(1), oneShare)))

	got := holdingsOn(t, st, march(2))

	assert.Equal(t, []store.Holding{{
		AccountID: acctOne, SecurityID: secNoCurrency, Account: "Chequing", AccountSourceID: 1,
		Security: new("Plain Fund"), SecuritySourceID: new(int64(4)), Shares: oneShare,
	}}, got)
}

func Test_holdings_reads_a_zero_price_as_a_price_with_a_value_of_zero(t *testing.T) {
	t.Parallel()
	rows := holdingRows(buy(acctOne, secAcme, 1, march(1), 2*oneShare))
	rows.Prices = []store.Price{quote(secAcme, 1, march(2), 0)}
	st := newStoreWithRates(t, rows, ratesOn(1, 1_250_000, "FXUSDCAD"))

	got := holdingsOn(t, st, march(5))

	require.Len(t, got, 1)
	assert.Equal(t, new(int64(0)), got[0].Price)
	assert.Equal(t, new(march(2)), got[0].PriceDate)
	assert.Equal(t, []string{"0", "0", "0"}, []string{got[0].Value.String(), got[0].ValueCAD.String(), got[0].ValueUSD.String()})
}

func Test_holdings_reads_the_placeholder_price_date_as_recorded(t *testing.T) {
	t.Parallel()
	placeholder := day(1899, time.December, 29)
	rows := holdingRows(buy(acctOne, secAcme, 1, march(1), 2*oneShare))
	rows.Prices = []store.Price{quote(secAcme, 1, placeholder, tenUnits)}
	st := newStoreWith(t, rows)

	got := holdingsOn(t, st, march(5))

	require.Len(t, got, 1)
	assert.Equal(t, new(placeholder), got[0].PriceDate)
	assert.Equal(t, big.NewInt(2000), got[0].Value)
}

func Test_holdings_lists_a_holding_whose_security_and_account_rows_are_missing(t *testing.T) {
	t.Parallel()
	st := newStoreWith(t, holdingRows(buy("acct-ghost", secGhost, 1, march(1), oneShare)))

	got := holdingsOn(t, st, march(2))

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
			name:       "account names ignoring case, so a lower-case name sorts among capitals",
			accounts:   []store.Account{brokerage(tierAccountA, 1, "Zulu"), brokerage(tierAccountB, 2, "alpha")},
			securities: []store.Security{fund(secAcme, 1, "Acme")},
			held:       []heldPair{{tierAccountA, secAcme}, {tierAccountB, secAcme}},
			want:       []string{tierAccountB + "/" + secAcme, tierAccountA + "/" + secAcme},
		},
		{
			name:       "account name as plain text when two names differ only in case",
			accounts:   []store.Account{brokerage(tierAccountA, 1, "alpha"), brokerage(tierAccountB, 2, "Alpha")},
			securities: []store.Security{fund(secAcme, 1, "Acme")},
			held:       []heldPair{{tierAccountA, secAcme}, {tierAccountB, secAcme}},
			want:       []string{tierAccountB + "/" + secAcme, tierAccountA + "/" + secAcme},
		},
		{
			name:       "security names ignoring case, so a lower-case name sorts among capitals",
			accounts:   []store.Account{brokerage(tierAccountA, 1, "Brokerage")},
			securities: []store.Security{fund(secAcme, 1, "Zeta"), fund(secUSD, 2, "alpha")},
			held:       []heldPair{{tierAccountA, secAcme}, {tierAccountA, secUSD}},
			want:       []string{tierAccountA + "/" + secUSD, tierAccountA + "/" + secAcme},
		},
		{
			name:       "security name as plain text when two names differ only in case",
			accounts:   []store.Account{brokerage(tierAccountA, 1, "Brokerage")},
			securities: []store.Security{fund(secAcme, 1, "alpha"), fund(secUSD, 2, "Alpha")},
			held:       []heldPair{{tierAccountA, secAcme}, {tierAccountA, secUSD}},
			want:       []string{tierAccountA + "/" + secUSD, tierAccountA + "/" + secAcme},
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
		buy(acctOne, secAcme, 1, march(1), oneShare), buy(acctOne, secAcme, 2, march(3), -oneShare)))

	held := holdingsOn(t, st, march(2))
	sold := holdingsOn(t, st, march(3))

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
		buy(acctOne, secUSD, 1, march(1), maxDecimal18x6), buy(acctTwo, secUSD, 2, march(1), -maxDecimal18x6))
	rows.Prices = []store.Price{quote(secUSD, 1, march(1), maxDecimal18x6)}
	st := newStoreWithRates(t, rows, ratesOn(1, 1_250_000, "FXUSDCAD"))

	got := holdingsOn(t, st, march(1))

	require.Len(t, got, 2)
	byAccount := map[string]store.Holding{got[0].AccountID: got[0], got[1].AccountID: got[1]}
	long, short := byAccount[acctOne], byAccount[acctTwo]
	assert.Equal(t, []*big.Int{cents("99999999999999999800000000"), cents("99999999999999999800000000"), cents("124999999999999999750000000")},
		[]*big.Int{long.Value, long.ValueUSD, long.ValueCAD})
	assert.Equal(t, []*big.Int{cents("-99999999999999999800000000"), cents("-99999999999999999800000000"), cents("-124999999999999999750000000")},
		[]*big.Int{short.Value, short.ValueUSD, short.ValueCAD})
}

func Test_holdings_gives_the_date_of_the_first_exchange_rate(t *testing.T) {
	t.Parallel()
	st := newStoreWithRates(t, holdingRows(buy(acctOne, secAcme, 1, march(1), oneShare)),
		ratesOn(12, 1_250_000, "FXUSDCAD"), ratesOn(10, 1_250_000, "FXUSDCAD"))

	got := holdingsRead(t, st, march(2))

	assert.Equal(t, march(10), got.FirstRate)
}

func Test_holdings_gives_no_first_rate_date_for_a_store_without_rates(t *testing.T) {
	t.Parallel()
	st := newStoreWith(t, holdingRows(buy(acctOne, secAcme, 1, march(1), oneShare)))

	got := holdingsRead(t, st, march(2))

	assert.True(t, got.FirstRate.IsZero())
	assert.Len(t, got.Holdings, 1)
}

func Test_holdings_returns_a_first_rate_read_fault_as_another_fault(t *testing.T) {
	t.Parallel()
	for _, c := range otherFaults("SELECT min", 1) {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			st := newBuiltStore(t, spyOpener(c.spy))

			_, err := st.Holdings(t.Context(), store.HoldingsParams{AsOf: march(2)})

			assertOtherFault(t, err, c.reason)
			assert.ErrorIs(t, err, c.fault)
		})
	}
}

func Test_holdings_lists_accounts_quicken_leaves_out_of_its_reports(t *testing.T) {
	t.Parallel()
	hidden := brokerage("acct-hidden", 1, "Hidden")
	hidden.NotInReports = true
	linked := brokerage("acct-linked", 2, "Linked")
	linked.LinkedTracking = true
	accounts := []store.Account{hidden, linked, brokerage("acct-plain", 3, "Plain")}
	held := []heldPair{{"acct-hidden", secAcme}, {"acct-linked", secAcme}, {"acct-plain", secAcme}}

	got := holdingOrder(t, accounts, []store.Security{fund(secAcme, 1, "Acme")}, held)

	assert.Equal(t, []string{"acct-hidden/" + secAcme, "acct-linked/" + secAcme, "acct-plain/" + secAcme}, got)
}

const acctNone = "acct-none"

// accountHoldingsStore holds Acme and Globex in acct-1 (Chequing) and Globex in acct-2 (Brokerage USD),
// on 2026-03-02.
func accountHoldingsStore(t *testing.T, mutate func(*store.Rows)) *duckstore.Store {
	t.Helper()
	rows := holdingRows(
		buy(acctOne, secAcme, 1, march(1), oneShare),
		buy(acctOne, secUSD, 2, march(1), oneShare),
		buy(acctTwo, secUSD, 3, march(1), oneShare))
	if mutate != nil {
		mutate(&rows)
	}
	return newStoreWith(t, rows)
}

// heldIn is "account/security" ids of the holdings accountHoldingsStore's st has on 2026-03-02 in the named accounts.
func heldIn(t *testing.T, st *duckstore.Store, ids ...string) []string {
	t.Helper()
	got := holdingsRead(t, st, march(2), ids...)
	order := make([]string, len(got.Holdings))
	for i, h := range got.Holdings {
		order[i] = h.AccountID + "/" + h.SecurityID
	}
	return order
}

func Test_holdings_reads_the_named_accounts_in_table_order(t *testing.T) {
	t.Parallel()
	st := accountHoldingsStore(t, nil)
	cases := []struct {
		name string
		ids  []string
		want []string
	}{
		{name: "one_account", ids: []string{acctOne}, want: []string{acctOne + "/" + secAcme, acctOne + "/" + secUSD}},
		{name: "another_account", ids: []string{acctTwo}, want: []string{acctTwo + "/" + secUSD}},
		{
			name: "every_account_when_none_is_named", ids: nil,
			want: []string{acctTwo + "/" + secUSD, acctOne + "/" + secAcme, acctOne + "/" + secUSD},
		},
		{
			name: "two_accounts_whatever_order_they_are_named", ids: []string{acctOne, acctTwo},
			want: []string{acctTwo + "/" + secUSD, acctOne + "/" + secAcme, acctOne + "/" + secUSD},
		},
		{name: "nothing_for_an_id_that_names_no_account", ids: []string{acctNone}, want: []string{}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			got := heldIn(t, st, c.ids...)

			assert.Equal(t, c.want, got)
		})
	}
}

func Test_holdings_reads_a_named_closed_account(t *testing.T) {
	t.Parallel()
	st := accountHoldingsStore(t, func(rows *store.Rows) { rows.Accounts[1].Closed = true })

	got := heldIn(t, st, acctTwo)

	assert.Equal(t, []string{acctTwo + "/" + secUSD}, got)
}

// holdingsRead reads st's holdings on date in the named accounts.
func holdingsRead(t *testing.T, st *duckstore.Store, date time.Time, ids ...string) store.Holdings {
	t.Helper()
	got, err := st.Holdings(t.Context(), store.HoldingsParams{AsOf: date, AccountIDs: ids})
	require.NoError(t, err)
	return got
}

func Test_holdings_reads_the_first_and_last_investment_transaction_dates(t *testing.T) {
	t.Parallel()
	cash := buy(acctOne, secAcme, 1, march(1), 0)
	cash.Action, cash.SecurityID, cash.Shares = "div", nil, nil
	future := localToday().AddDate(0, 0, 3)
	cases := []struct {
		name  string
		txns  []store.InvestmentTransaction
		asOf  time.Time
		ids   []string
		first time.Time
		last  time.Time
	}{
		{
			name: "of_every_account",
			txns: []store.InvestmentTransaction{
				buy(acctOne, secAcme, 1, march(3), oneShare), buy(acctTwo, secUSD, 2, march(1), oneShare), buy(acctOne, secAcme, 3, march(5), oneShare),
			},
			asOf: march(5), first: march(1), last: march(5),
		},
		{
			name: "of_only_the_named_accounts",
			txns: []store.InvestmentTransaction{
				buy(acctTwo, secUSD, 1, march(1), oneShare), buy(acctOne, secAcme, 2, march(3), oneShare),
				buy(acctOne, secAcme, 3, march(4), oneShare), buy(acctTwo, secUSD, 4, march(8), oneShare),
			},
			asOf: march(5), ids: []string{acctOne}, first: march(3), last: march(4),
		},
		{
			name: "counting_a_cash_only_and_a_future_dated_transaction",
			txns: []store.InvestmentTransaction{cash, buy(acctOne, secAcme, 2, march(2), oneShare), buy(acctOne, secAcme, 3, future, oneShare)},
			asOf: march(2), first: march(1), last: future,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			st := newStoreWith(t, holdingRows(c.txns...))

			got := holdingsRead(t, st, c.asOf, c.ids...)

			assert.Equal(t, []time.Time{c.first, c.last}, []time.Time{got.FirstTransaction, got.LastTransaction})
		})
	}
}

func Test_holdings_reads_no_transaction_dates_when_no_investment_transaction_is_in_scope(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		txns []store.InvestmentTransaction
		ids  []string
	}{
		{name: "a_store_without_investment_transactions"},
		{
			name: "an_id_that_names_no_account",
			txns: []store.InvestmentTransaction{buy(acctOne, secAcme, 1, march(3), oneShare)}, ids: []string{acctNone},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			st := newStoreWith(t, holdingRows(c.txns...))

			got := holdingsRead(t, st, march(5), c.ids...)

			assert.True(t, got.FirstTransaction.IsZero())
			assert.True(t, got.LastTransaction.IsZero())
		})
	}
}

func Test_holdings_reads_the_same_transaction_span_on_a_day_before_the_first_transaction(t *testing.T) {
	t.Parallel()
	st := newStoreWith(t, holdingRows(
		buy(acctOne, secAcme, 1, march(3), oneShare),
		buy(acctOne, secAcme, 2, march(5), oneShare)))

	got := holdingsRead(t, st, march(1))

	assert.Empty(t, got.Holdings)
	assert.Equal(t, []time.Time{march(3), march(5)}, []time.Time{got.FirstTransaction, got.LastTransaction})
}

func Test_holdings_returns_a_transaction_span_read_fault_as_another_fault(t *testing.T) {
	t.Parallel()
	for _, c := range otherFaults("SELECT min", 2) {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			st := newBuiltStore(t, spyOpener(c.spy))

			_, err := st.Holdings(t.Context(), store.HoldingsParams{AsOf: march(2)})

			assertOtherFault(t, err, c.reason)
			assert.ErrorIs(t, err, c.fault)
		})
	}
}

func Test_holdings_view_takes_the_latest_price_on_or_before_the_date(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		prices []store.Price
		want   []string
	}{
		{
			name:   "a price dated on the date is used",
			prices: []store.Price{quote(secAcme, 1, march(3), tenUnits), quote(secAcme, 2, march(5), 12_000_000)},
			want:   []string{"12.000000", "2026-03-05", "24.00"},
		},
		{
			name:   "the latest of two earlier prices is used",
			prices: []store.Price{quote(secAcme, 1, march(2), tenUnits), quote(secAcme, 2, march(4), 12_000_000)},
			want:   []string{"12.000000", "2026-03-04", "24.00"},
		},
		{
			name:   "a price dated after the date is not used",
			prices: []store.Price{quote(secAcme, 1, march(3), tenUnits), quote(secAcme, 2, march(6), 99_000_000)},
			want:   []string{"10.000000", "2026-03-03", "20.00"},
		},
		{
			name:   "an old price shows its own day",
			prices: []store.Price{quote(secAcme, 1, march(1), tenUnits)},
			want:   []string{"10.000000", "2026-03-01", "20.00"},
		},
		{
			name:   "another security's price is not used",
			prices: []store.Price{quote(secUSD, 1, march(4), tenUnits)},
			want:   []string{"NULL", "NULL", "NULL"},
		},
		{
			name:   "no price at all leaves price, price date and value NULL",
			prices: nil,
			want:   []string{"NULL", "NULL", "NULL"},
		},
		{
			name:   "a zero price is used as recorded",
			prices: []store.Price{quote(secAcme, 1, march(2), 0)},
			want:   []string{"0.000000", "2026-03-02", "0.00"},
		},
		{
			name:   "the placeholder price date is used as recorded",
			prices: []store.Price{quote(secAcme, 1, day(1899, time.December, 29), tenUnits)},
			want:   []string{"10.000000", placeholderPrice, "20.00"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			rows := holdingRows(buy(acctOne, secAcme, 1, march(1), 2*oneShare))
			rows.Prices = c.prices
			st := newStoreWithRates(t, rows)

			got := queryTexts(t, st, "SELECT price, price_date, value FROM v_holdings WHERE date = '2026-03-05'")

			assert.Equal(t, [][]string{c.want}, got)
		})
	}
}

func Test_holdings_view_rounds_a_half_cent_away_from_zero_and_below_half_down(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		shares int64
		price  int64
		want   string
	}{
		{name: "exactly half a cent rounds up", shares: 500_000, price: 10_000, want: "0.01"},
		{name: "exactly minus half a cent rounds down", shares: -500_000, price: 10_000, want: "-0.01"},
		{name: "just below half a cent rounds to zero", shares: oneShare, price: 4_999, want: "0.00"},
		{name: "a product that is not whole cents rounds to the nearest", shares: 3_500_000, price: 12_345_678, want: "43.21"},
		{name: "negative shares give a negative value", shares: -2 * oneShare, price: tenUnits, want: "-20.00"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			rows := holdingRows(buy(acctOne, secAcme, 1, march(1), c.shares))
			rows.Prices = []store.Price{quote(secAcme, 1, march(1), c.price)}
			st := newStoreWithRates(t, rows)

			got := queryTexts(t, st, "SELECT value FROM v_holdings WHERE date = '2026-03-01'")

			assert.Equal(t, [][]string{{c.want}}, got)
		})
	}
}

func Test_holdings_view_values_a_split_day_at_that_days_price(t *testing.T) {
	t.Parallel()
	rows := holdingRows(
		buy(acctOne, secAcme, 1, march(1), 10*oneShare),
		splitOf(acctOne, secAcme, 2, march(3), 2, 1))
	rows.Prices = []store.Price{quote(secAcme, 1, march(1), tenUnits), quote(secAcme, 2, march(3), 6_000_000)}
	st := newStoreWithRates(t, rows)

	got := queryTexts(t, st, "SELECT CAST(date AS VARCHAR), shares, price, value FROM v_holdings WHERE date IN ('2026-03-02', '2026-03-03') ORDER BY date")

	assert.Equal(t, [][]string{
		{"2026-03-02", "10.000000", "10.000000", "100.00"},
		{"2026-03-03", "20.000000", "6.000000", "120.00"},
	}, got)
}

func Test_holdings_view_values_the_largest_holding_without_overflow(t *testing.T) {
	t.Parallel()
	rows := holdingRows(
		buy(acctOne, secAcme, 1, march(1), maxDecimal18x6), buy(acctOne, secUSD, 2, march(1), maxDecimal18x6),
		buy(acctTwo, secAcme, 3, march(1), -maxDecimal18x6), buy(acctTwo, secUSD, 4, march(1), -maxDecimal18x6))
	rows.Prices = []store.Price{quote(secAcme, 1, march(1), maxDecimal18x6), quote(secUSD, 2, march(1), maxDecimal18x6)}
	st := newStoreWithRates(t, rows, ratesOn(1, 1_250_000, "FXUSDCAD"))

	got := queryTexts(t, st, "SELECT account_id, security_id, value, value_cad, value_usd FROM v_holdings WHERE date = '2026-03-01' ORDER BY account_id, security_id")

	assert.Equal(t, [][]string{
		{acctOne, secAcme, maxHoldingValue, maxHoldingValue, maxHoldingInUSD},
		{acctOne, secUSD, maxHoldingValue, maxHoldingInCAD, maxHoldingValue},
		{acctTwo, secAcme, "-" + maxHoldingValue, "-" + maxHoldingValue, "-" + maxHoldingInUSD},
		{acctTwo, secUSD, "-" + maxHoldingValue, "-" + maxHoldingInCAD, "-" + maxHoldingValue},
	}, got)
}

func Test_holdings_view_leaves_every_value_NULL_for_a_holding_with_no_price_even_with_a_rate_in_force(t *testing.T) {
	t.Parallel()
	cases := []struct{ name, security string }{{"a CAD holding", secAcme}, {"a USD holding", secUSD}}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			st := newStoreWithRates(t, holdingRows(buy(acctOne, c.security, 1, march(1), oneShare)), fridayAndMonday()...)

			got := queryTexts(t, st, "SELECT value, value_cad, value_usd, usd_cad FROM v_holdings WHERE date = '2026-03-13'")

			assert.Equal(t, [][]string{{"NULL", "NULL", "NULL", "1.250000"}}, got)
		})
	}
}

func Test_holdings_view_converts_a_value_at_the_rate_in_force_on_the_date(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		security string
		account  string
		date     string
		rates    []store.Rate
		want     []string
	}{
		{
			name: "a CAD holding keeps its value and converts to USD", security: secAcme, account: acctOne, date: "2026-03-13", rates: fridayAndMonday(),
			want: []string{"10.00", "10.00", "8.00", "1.250000"},
		},
		{
			name: "a USD holding keeps its value and converts to CAD", security: secUSD, account: acctOne, date: "2026-03-13", rates: fridayAndMonday(),
			want: []string{"10.00", "12.50", "10.00", "1.250000"},
		},
		{
			name: "a Saturday takes the Friday rate", security: secUSD, account: acctOne, date: "2026-03-14", rates: fridayAndMonday(),
			want: []string{"10.00", "12.50", "10.00", "1.250000"},
		},
		{
			name: "a day after the last rate takes the last rate", security: secUSD, account: acctOne, date: "2026-03-20", rates: fridayAndMonday(),
			want: []string{"10.00", "13.00", "10.00", "1.300000"},
		},
		{
			name: "a CAD holding before the first rate keeps its own currency only", security: secAcme, account: acctOne, date: "2026-03-12", rates: fridayAndMonday(),
			want: []string{"10.00", "10.00", "NULL", "NULL"},
		},
		{
			name: "a USD holding before the first rate keeps its own currency only", security: secUSD, account: acctOne, date: "2026-03-12", rates: fridayAndMonday(),
			want: []string{"10.00", "NULL", "10.00", "NULL"},
		},
		{
			name: "a CAD holding with no rates in the store keeps its own currency only", security: secAcme, account: acctOne, date: "2026-03-13", rates: nil,
			want: []string{"10.00", "10.00", "NULL", "NULL"},
		},
		{
			name: "a USD holding with no rates in the store keeps its own currency only", security: secUSD, account: acctOne, date: "2026-03-13", rates: nil,
			want: []string{"10.00", "NULL", "10.00", "NULL"},
		},
		{
			name: "a security with no currency converts to nothing and does not borrow its account's", security: secNoCurrency, account: acctOne, date: "2026-03-13", rates: fridayAndMonday(),
			want: []string{"10.00", "NULL", "NULL", "1.250000"},
		},
		{
			name: "a security in another currency converts to nothing", security: secEUR, account: acctOne, date: "2026-03-13", rates: fridayAndMonday(),
			want: []string{"10.00", "NULL", "NULL", "1.250000"},
		},
		{
			name: "the security's currency wins over its account's", security: secAcme, account: acctTwo, date: "2026-03-13", rates: fridayAndMonday(),
			want: []string{"10.00", "10.00", "8.00", "1.250000"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			rows := holdingRows(
				buy(acctOne, secAcme, 1, march(1), oneShare), buy(acctOne, secUSD, 2, march(1), oneShare),
				buy(acctOne, secEUR, 3, march(1), oneShare), buy(acctOne, secNoCurrency, 4, march(1), oneShare),
				buy(acctTwo, secAcme, 5, march(1), oneShare))
			rows.Prices = []store.Price{
				quote(secAcme, 1, march(1), tenUnits), quote(secUSD, 2, march(1), tenUnits),
				quote(secEUR, 3, march(1), tenUnits), quote(secNoCurrency, 4, march(1), tenUnits),
			}
			st := newStoreWithRates(t, rows, c.rates...)

			got := queryTexts(t, st, "SELECT value, value_cad, value_usd, usd_cad FROM v_holdings WHERE date = '"+c.date+
				"' AND security_id = '"+c.security+"' AND account_id = '"+c.account+"'")

			assert.Equal(t, [][]string{c.want}, got)
		})
	}
}

// dayList is dates joined with commas, as heldDaysQuery returns them.
func dayList(days ...time.Time) string {
	texts := make([]string, len(days))
	for i, d := range days {
		texts[i] = d.Format(time.DateOnly)
	}
	return strings.Join(texts, ",")
}

func Test_holdings_view_lists_each_day_a_holding_is_held_and_none_between_spans(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		txns []store.InvestmentTransaction
		want string
	}{
		{
			name: "each_day_of_a_closed_span_from_the_first_to_the_last",
			txns: []store.InvestmentTransaction{buy(acctOne, secAcme, 1, march(1), oneShare), buy(acctOne, secAcme, 2, march(4), -oneShare)},
			want: dayList(march(1), march(2), march(3)),
		},
		{
			name: "none_between_two_spans_of_one_holding",
			txns: []store.InvestmentTransaction{
				buy(acctOne, secAcme, 1, march(1), oneShare), buy(acctOne, secAcme, 2, march(3), -oneShare),
				buy(acctOne, secAcme, 3, march(5), oneShare), buy(acctOne, secAcme, 4, march(7), -oneShare),
			},
			want: dayList(march(1), march(2), march(5), march(6)),
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			st := newStoreWith(t, holdingRows(c.txns...))

			got := queryTexts(t, st, heldDaysQuery)

			assert.Equal(t, [][]string{{c.want}}, got)
		})
	}
}

func Test_holdings_view_stops_each_span_at_today(t *testing.T) {
	t.Parallel()
	today := localToday()
	daysAgo := func(n int) time.Time { return today.AddDate(0, 0, -n) }
	cases := []struct {
		name string
		txns []store.InvestmentTransaction
		want string
	}{
		{
			name: "an open span ends today",
			txns: []store.InvestmentTransaction{buy(acctOne, secAcme, 1, daysAgo(2), oneShare)},
			want: dayList(daysAgo(2), daysAgo(1), today),
		},
		{
			name: "a span closed after today ends today",
			txns: []store.InvestmentTransaction{
				buy(acctOne, secAcme, 1, daysAgo(1), oneShare), buy(acctOne, secAcme, 2, today.AddDate(0, 0, 2), -oneShare),
			},
			want: dayList(daysAgo(1), today),
		},
		{
			name: "a span that starts after today has no rows",
			txns: []store.InvestmentTransaction{buy(acctOne, secAcme, 1, today.AddDate(0, 0, 3), oneShare)},
			want: dayList(),
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			st := newStoreWith(t, holdingRows(c.txns...))

			got := queryTexts(t, st, heldDaysQuery)

			assert.Equal(t, [][]string{{c.want}}, got)
		})
	}
}

func Test_holdings_view_has_one_row_a_day_with_the_shares_held_that_day(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		txns []store.InvestmentTransaction
		want [][]string
	}{
		{
			name: "where_two_spans_meet",
			txns: []store.InvestmentTransaction{
				buy(acctOne, secAcme, 1, march(1), oneShare), buy(acctOne, secAcme, 2, march(3), oneShare),
				buy(acctOne, secAcme, 3, march(5), -2*oneShare),
			},
			want: [][]string{{"2026-03-01", "1.000000"}, {"2026-03-02", "1.000000"}, {"2026-03-03", "2.000000"}, {"2026-03-04", "2.000000"}},
		},
		{
			name: "across_a_span_of_negative_shares",
			txns: []store.InvestmentTransaction{buy(acctOne, secAcme, 1, march(1), -oneShare), buy(acctOne, secAcme, 2, march(3), oneShare)},
			want: [][]string{{"2026-03-01", "-1.000000"}, {"2026-03-02", "-1.000000"}},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			st := newStoreWith(t, holdingRows(c.txns...))

			got := queryTexts(t, st, "SELECT CAST(date AS VARCHAR), CAST(shares AS VARCHAR) FROM v_holdings ORDER BY date")

			assert.Equal(t, c.want, got)
		})
	}
}

func Test_holdings_view_lists_a_holding_whose_security_row_is_missing(t *testing.T) {
	t.Parallel()
	st := newStoreWith(t, holdingRows(
		buy(acctOne, secGhost, 1, march(1), oneShare), buy(acctOne, secGhost, 2, march(2), -oneShare)))

	got := queryTexts(t, st, "SELECT security_id, security, ticker, currency, price, value FROM v_holdings")

	assert.Equal(t, [][]string{{secGhost, "NULL", "NULL", "NULL", "NULL", "NULL"}}, got)
}

func Test_holdings_view_lists_accounts_left_out_of_reports(t *testing.T) {
	t.Parallel()
	rows := holdingRows(buy(acctOne, secAcme, 1, march(1), oneShare), buy(acctTwo, secAcme, 2, march(1), oneShare))
	rows.Accounts[0].NotInReports = true
	rows.Accounts[1].LinkedTracking = true
	st := newStoreWith(t, rows)

	got := queryTexts(t, st, "SELECT account_id FROM v_holdings WHERE date = '2026-03-01' ORDER BY account_id")

	assert.Equal(t, [][]string{{acctOne}, {acctTwo}}, got)
}

func Test_holdings_view_lists_its_columns_in_order(t *testing.T) {
	t.Parallel()
	st := newStoreWith(t, holdingRows())

	got, err := st.Query(t.Context(), "SELECT * FROM v_holdings", 0) //nolint:unqueryvet // every column is the point

	require.NoError(t, err)
	assert.Equal(t, []store.QueryColumn{
		{Name: "date", Type: "DATE"},
		{Name: "account_id", Type: "VARCHAR"},
		{Name: "security_id", Type: "VARCHAR"},
		{Name: "security", Type: "VARCHAR"},
		{Name: "ticker", Type: "VARCHAR"},
		{Name: "shares", Type: "DECIMAL(18,6)"},
		{Name: "price", Type: "DECIMAL(18,6)"},
		{Name: "price_date", Type: "DATE"},
		{Name: "currency", Type: "VARCHAR"},
		{Name: "value", Type: "DECIMAL(38,2)"},
		{Name: "value_cad", Type: "DECIMAL(38,2)"},
		{Name: "value_usd", Type: "DECIMAL(38,2)"},
		{Name: "usd_cad", Type: "DECIMAL(10,6)"},
	}, got.Columns)
}

func Test_holdings_view_carries_its_note(t *testing.T) {
	t.Parallel()
	st := newStoreWith(t, holdingRows())

	got := queryTexts(t, st, "SELECT comment FROM duckdb_views() WHERE view_name = 'v_holdings'")

	assert.Equal(t, [][]string{{"one row per holding per day it is held, through today, so filter by date; " +
		"value is shares times price rounded to the cent, value_cad and value_usd convert it at the rate for date as quarry holdings does; " +
		"cash in investment accounts is not included."}}, got)
}

// heldDaysQuery is the dates of every v_holdings row of Acme, oldest first, joined by commas.
const heldDaysQuery = `SELECT coalesce(string_agg(CAST(date AS VARCHAR), ',' ORDER BY date), '') FROM v_holdings WHERE security_id = 'sec-1'`
