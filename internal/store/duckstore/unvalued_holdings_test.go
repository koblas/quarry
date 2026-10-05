package duckstore_test

import (
	"context"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/store"
	"github.com/koblas/quarry/internal/store/duckstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// unvaluedOn reads the unvalued holdings st's net worth read returns for dates.
func unvaluedOn(t *testing.T, st *duckstore.Store, dates ...time.Time) []store.UnvaluedHolding {
	t.Helper()
	got, err := st.NetWorth(t.Context(), store.NetWorthParams{Dates: dates})
	require.NoError(t, err)
	return got.Unvalued
}

// unvaluedIDs is the account id of each of held.
func unvaluedIDs(held []store.UnvaluedHolding) []string {
	ids := make([]string, len(held))
	for i, h := range held {
		ids[i] = h.AccountID
	}
	return ids
}

// unvaluedKeys is "date account security" of each of held.
func unvaluedKeys(held []store.UnvaluedHolding) []string {
	keys := make([]string, len(held))
	for i, h := range held {
		keys[i] = fmt.Sprintf("%s %s %s", h.Date.Format(time.DateOnly), h.Account, h.Security)
	}
	return keys
}

// unvaluedReads are the two reads that carry unvalued holdings.
func unvaluedReads() []readOp {
	return []readOp{
		{name: "NetWorth", call: func(ctx context.Context, st *duckstore.Store) error {
			_, err := st.NetWorth(ctx, store.NetWorthParams{Dates: []time.Time{day(2026, 9, 29)}})
			return err
		}},
		{name: "Accounts", call: func(ctx context.Context, st *duckstore.Store) error {
			_, err := st.Accounts(ctx)
			return err
		}},
	}
}

func Test_net_worth_names_each_holding_its_balance_leaves_out(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		prices  []store.Price
		rateDay int
		want    store.UnvaluedHolding
	}{
		{
			name: "a holding with no price", rateDay: 1,
			want: store.UnvaluedHolding{SecurityID: secAcme, Security: "Acme Corp", Currency: new("CAD"), Priced: false},
		},
		{
			name: "a priced holding with no currency", rateDay: 1, prices: []store.Price{quote(secNoCurrency, 2, marchDay(1), tenUnits)},
			want: store.UnvaluedHolding{SecurityID: secNoCurrency, Security: "Plain Fund", Priced: true},
		},
		{
			name: "a priced holding in a currency other than CAD and USD", rateDay: 1, prices: []store.Price{quote(secEUR, 2, marchDay(1), tenUnits)},
			want: store.UnvaluedHolding{SecurityID: secEUR, Security: "Euro Fund", Currency: new("EUR"), Priced: true},
		},
		{
			name: "a holding priced at zero with no currency", rateDay: 1, prices: []store.Price{quote(secNoCurrency, 2, marchDay(1), 0)},
			want: store.UnvaluedHolding{SecurityID: secNoCurrency, Security: "Plain Fund", Priced: true},
		},
		{
			name: "a priced USD holding before the first rate", rateDay: 10, prices: []store.Price{quote(secUSD, 2, marchDay(1), tenUnits)},
			want: store.UnvaluedHolding{SecurityID: secUSD, Security: "Globex Inc", Currency: new("USD"), Priced: true},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			rows := balanceRows(buy(acctOne, secControl, 1, marchDay(1), oneShare), buy(acctOne, c.want.SecurityID, 2, marchDay(1), oneShare))
			rows.Prices = append([]store.Price{quote(secControl, 1, marchDay(1), tenUnits)}, c.prices...)
			st := newStoreWithRates(t, rows, ratesOn(c.rateDay, 1_250_000, "FXUSDCAD"))

			got := unvaluedOn(t, st, marchDay(2))

			c.want.Date, c.want.AccountID, c.want.Account, c.want.AccountCurrency = marchDay(2), acctOne, "Chequing", "CAD"
			assert.Equal(t, []store.UnvaluedHolding{c.want}, got)
		})
	}
}

func Test_unvalued_holdings_carry_the_currency_of_their_account(t *testing.T) {
	t.Parallel()
	rows := balanceRows(buy(acctTwo, secAcme, 1, marchDay(1), oneShare))
	rows.Prices = []store.Price{quote(secAcme, 1, marchDay(1), tenUnits)}
	st := newStoreWith(t, rows)

	got := unvaluedOn(t, st, marchDay(2))

	assert.Equal(t, []store.UnvaluedHolding{{
		Date: marchDay(2), AccountID: acctTwo, Account: "Brokerage USD", AccountCurrency: "USD",
		SecurityID: secAcme, Security: "Acme Corp", Currency: new("CAD"), Priced: true,
	}}, got)
}

func Test_unvalued_holdings_match_the_view_count_per_account_and_day(t *testing.T) {
	t.Parallel()
	rows := balanceRows(
		buy(acctOne, secControl, 1, marchDay(1), oneShare), buy(acctOne, secAcme, 2, marchDay(1), oneShare),
		buy(acctOne, secEUR, 3, marchDay(3), oneShare), buy(acctOne, secNoCurrency, 4, marchDay(4), oneShare),
		buy(acctTwo, secControl, 5, marchDay(2), oneShare), buy(acctTwo, secUSD, 6, marchDay(2), oneShare),
		buy(acctEUR, secAcme, 7, marchDay(3), oneShare), buy(acctEUR, secEUR, 8, marchDay(3), oneShare))
	rows.Prices = []store.Price{
		quote(secControl, 1, marchDay(1), tenUnits), quote(secEUR, 2, marchDay(1), tenUnits),
		quote(secNoCurrency, 3, marchDay(1), tenUnits), quote(secUSD, 4, marchDay(1), tenUnits),
	}
	st := newStoreWithRates(t, rows, ratesOn(4, 1_250_000, "FXUSDCAD"))

	view := queryTexts(t, st, "SELECT account_id, CAST(date AS VARCHAR), CAST(holdings_unvalued AS VARCHAR) FROM v_balances_daily "+
		"WHERE holdings_unvalued > 0 AND date IN ('2026-03-01', '2026-03-02', '2026-03-03', '2026-03-04', '2026-03-05')")
	got := unvaluedOn(t, st, marchDay(1), marchDay(2), marchDay(3), marchDay(4), marchDay(5))

	fromView := map[string]int{}
	for _, r := range view {
		n, err := strconv.Atoi(r[2])
		require.NoError(t, err)
		fromView[r[0]+" "+r[1]] = n
	}
	fromRows := map[string]int{}
	for _, h := range got {
		fromRows[h.AccountID+" "+h.Date.Format(time.DateOnly)]++
	}
	assert.Equal(t, fromView, fromRows)
	assert.Greater(t, len(fromView), 6, "fixture must leave holdings out on several accounts and days")
}

func Test_net_worth_leaves_a_not_in_reports_or_linked_tracking_accounts_unpriced_holding_out(t *testing.T) {
	t.Parallel()
	rows := balanceRows(
		buy(acctOne, secAcme, 1, marchDay(1), oneShare), buy(acctTwo, secUSD, 2, marchDay(1), oneShare),
		buy(acctRetirement, secAcme, 3, marchDay(1), oneShare))
	rows.Accounts[1].NotInReports = true
	rows.Accounts[3].LinkedTracking = true
	st := newStoreWith(t, rows)

	got := unvaluedOn(t, st, marchDay(2))

	assert.Equal(t, []string{acctOne}, unvaluedIDs(got))
}

func Test_net_worth_and_accounts_leave_a_non_investment_accounts_unpriced_holding_out(t *testing.T) {
	t.Parallel()
	rows := balanceRows(buy(acctOne, secAcme, 1, marchDay(1), oneShare), buy(acctChequing, secAcme, 2, marchDay(1), oneShare))
	st := newStoreWith(t, rows)

	netWorth := unvaluedOn(t, st, marchDay(2))
	accounts, err := st.Accounts(t.Context())

	require.NoError(t, err)
	assert.Equal(t, []string{acctOne}, unvaluedIDs(netWorth))
	assert.Equal(t, []string{acctOne}, unvaluedIDs(accounts.Unvalued))
}

func Test_net_worth_names_a_closed_accounts_unpriced_holding(t *testing.T) {
	t.Parallel()
	rows := balanceRows(buy(acctOne, secAcme, 1, marchDay(1), oneShare), buy(acctTwo, secAcme, 2, marchDay(1), oneShare))
	rows.Accounts[0].Closed = true
	st := newStoreWith(t, rows)

	got := unvaluedOn(t, st, marchDay(2))

	assert.Equal(t, []string{acctTwo, acctOne}, unvaluedIDs(got))
}

func Test_accounts_lists_every_accounts_unvalued_holding_as_of_today(t *testing.T) {
	t.Parallel()
	rows := balanceRows(
		buy(acctOne, secAcme, 1, marchDay(1), oneShare), buy(acctTwo, secUSD, 2, marchDay(1), oneShare),
		buy(acctRetirement, secAcme, 3, marchDay(1), oneShare), buy(acctEUR, secEUR, 4, marchDay(1), oneShare),
		buy(acctEUR, secEUR, 5, marchDay(3), -oneShare))
	rows.Accounts[1].NotInReports = true
	rows.Accounts[3].LinkedTracking = true
	rows.Accounts[0].Closed = true
	st := newStoreWith(t, rows)

	got, err := st.Accounts(t.Context())

	require.NoError(t, err)
	assert.Equal(t, []string{acctTwo, acctOne, acctRetirement}, unvaluedIDs(got.Unvalued))
	assert.Equal(t, []time.Time{got.AsOf, got.AsOf, got.AsOf}, []time.Time{got.Unvalued[0].Date, got.Unvalued[1].Date, got.Unvalued[2].Date})
}

func Test_net_worth_reads_unvalued_holdings_beside_a_balance_past_64_bits(t *testing.T) {
	t.Parallel()
	rows := balanceRows(buy(acctOne, secAcme, 1, marchDay(1), maxDecimal18x6), buy(acctOne, secEUR, 2, marchDay(1), oneShare))
	rows.Prices = []store.Price{quote(secAcme, 1, marchDay(1), maxDecimal18x6)}
	st := newStoreWithRates(t, rows, ratesOn(1, 1_250_000, "FXUSDCAD"))

	got := unvaluedOn(t, st, marchDay(2))

	assert.Equal(t, []string{"2026-03-02 Chequing Euro Fund"}, unvaluedKeys(got))
}

func Test_net_worth_orders_unvalued_holdings_by_date_account_then_security(t *testing.T) {
	t.Parallel()
	rows := balanceRows(
		buy(acctTwo, secAcme, 1, marchDay(1), oneShare), buy(acctOne, secEUR, 2, marchDay(1), oneShare),
		buy(acctOne, secAcme, 3, marchDay(1), oneShare))
	st := newStoreWith(t, rows)

	got := unvaluedOn(t, st, marchDay(2), marchDay(1))

	assert.Equal(t, []string{
		"2026-03-01 Brokerage USD Acme Corp", "2026-03-01 Chequing Acme Corp", "2026-03-01 Chequing Euro Fund",
		"2026-03-02 Brokerage USD Acme Corp", "2026-03-02 Chequing Acme Corp", "2026-03-02 Chequing Euro Fund",
	}, unvaluedKeys(got))
}

func Test_net_worth_for_no_dates_runs_no_query(t *testing.T) {
	t.Parallel()
	spy := &spyReadDB{}
	st := newBuiltStore(t, spyOpener(spy))

	_ = unvaluedOn(t, st)
	idle := spy.queries
	_ = unvaluedOn(t, st, marchDay(1))

	assert.Zero(t, idle)
	assert.Equal(t, 4, spy.queries)
}

func Test_net_worth_and_accounts_return_the_unvalued_holdings_query_fault_as_another_fault(t *testing.T) {
	t.Parallel()
	for _, op := range unvaluedReads() {
		t.Run(op.name, func(t *testing.T) {
			t.Parallel()
			fault := ioFault(`query rows "SELECT"`)
			st := newBuiltStore(t, spyOpener(&spyReadDB{passQueries: 1, queryFault: fault}))

			err := op.call(t.Context(), st)

			assertOtherFault(t, err, "disk read failed")
			assert.ErrorIs(t, err, fault)
		})
	}
}

func Test_net_worth_and_accounts_return_the_unvalued_holdings_scan_fault_as_another_fault(t *testing.T) {
	t.Parallel()
	for _, op := range unvaluedReads() {
		t.Run(op.name, func(t *testing.T) {
			t.Parallel()
			st := newBuiltStore(t, spyOpener(&spyReadDB{passQueries: 1, scanFault: errScanFailed}))

			err := op.call(t.Context(), st)

			assertOtherFault(t, err, errScanFailed.Error())
			assert.ErrorIs(t, err, errScanFailed)
		})
	}
}
