package document_test

import (
	"testing"
	"time"

	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/report/document"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
)

func leftOutDay(d int) time.Time { return time.Date(2026, time.March, d, 0, 0, 0, 0, time.UTC) }

// unpriced is a holding of account with no price, in CAD, on day d of March 2026.
func unpriced(accountID, account, securityID, security string, d int) store.UnvaluedHolding {
	return store.UnvaluedHolding{
		Date: leftOutDay(d), AccountID: accountID, Account: account, SecurityID: securityID, Security: security,
		Currency: new("CAD"),
	}
}

// noCurrency is a priced holding with no currency, on day 12.
func noCurrency(accountID, account, securityID, security string) store.UnvaluedHolding {
	return store.UnvaluedHolding{
		Date: leftOutDay(12), AccountID: accountID, Account: account, SecurityID: securityID, Security: security, Priced: true,
	}
}

// pricedIn is a priced holding in currency, on day 12.
func pricedIn(currency, accountID, account, securityID, security string) store.UnvaluedHolding {
	return store.UnvaluedHolding{
		Date: leftOutDay(12), AccountID: accountID, Account: account, SecurityID: securityID, Security: security,
		Currency: &currency, Priced: true,
	}
}

func snapshot(rows ...store.UnvaluedHolding) report.NetWorth {
	return report.NetWorth{AsOf: leftOutDay(12), Unvalued: rows}
}

func history(rows ...store.UnvaluedHolding) report.NetWorth {
	return report.NetWorth{
		AsOf: leftOutDay(12), Window: &store.Window{Since: leftOutDay(1), Until: leftOutDay(12)}, Unvalued: rows,
	}
}

const (
	brokerageNoPriceLine = `"Brokerage" holds 1 security with no price on or before 2026-03-12, ` +
		`so its balance leaves it out; enter a price in Quicken, then run quarry sync`
	acmeNoCurrencyLine = `"Acme" has no currency in Quicken, so quarry leaves its value out of "Brokerage"'s balance; ` +
		`set its currency in Quicken, then run quarry sync`
)

func Test_net_worth_warnings_name_the_account_that_holds_one_unpriced_security(t *testing.T) {
	n := snapshot(unpriced("a-1", "Brokerage", "s-1", "Acme", 12))

	assert.Equal(t, []string{brokerageNoPriceLine}, document.NetWorthWarnings(n))
}

func Test_net_worth_warnings_count_the_unpriced_securities_when_an_account_holds_several(t *testing.T) {
	n := snapshot(unpriced("a-1", "Brokerage", "s-1", "Acme", 12), unpriced("a-1", "Brokerage", "s-2", "Bond", 12))

	assert.Equal(t, []string{`"Brokerage" holds 2 securities with no price on or before 2026-03-12, ` +
		`so its balance leaves them out; enter prices in Quicken, then run quarry sync`}, document.NetWorthWarnings(n))
}

func Test_net_worth_history_warnings_count_the_month_ends_an_account_held_an_unpriced_security(t *testing.T) {
	n := history(
		unpriced("a-1", "Brokerage", "s-1", "Acme", 5), unpriced("a-1", "Brokerage", "s-2", "Bond", 5),
		unpriced("a-1", "Brokerage", "s-1", "Acme", 8), unpriced("a-1", "Brokerage", "s-1", "Acme", 12),
		unpriced("a-2", "Savings", "s-1", "Acme", 8))

	assert.Equal(t, []string{
		`"Brokerage" holds a security with no price on 3 of the month ends listed, ` +
			`so its balance leaves it out on those days; enter prices in Quicken, then run quarry sync`,
		`"Savings" holds a security with no price on 1 of the month ends listed, ` +
			`so its balance leaves it out on those days; enter prices in Quicken, then run quarry sync`,
	}, document.NetWorthWarnings(n))
}

func Test_net_worth_snapshot_warnings_name_the_as_of_day(t *testing.T) {
	n := report.NetWorth{AsOf: leftOutDay(3), Unvalued: []store.UnvaluedHolding{unpriced("a-1", "Brokerage", "s-1", "Acme", 3)}}

	assert.Equal(t, []string{`"Brokerage" holds 1 security with no price on or before 2026-03-03, ` +
		`so its balance leaves it out; enter a price in Quicken, then run quarry sync`}, document.NetWorthWarnings(n))
}

func Test_accounts_warnings_name_the_listings_as_of_day(t *testing.T) {
	l := report.AccountListing{
		AsOf:     leftOutDay(12),
		Unvalued: []store.UnvaluedHolding{unpriced("a-1", "Brokerage", "s-1", "Acme", 12)},
	}

	assert.Equal(t, []string{brokerageNoPriceLine}, document.AccountsWarnings(l))
}

func Test_a_holding_is_warned_about_by_the_reason_it_has_no_value(t *testing.T) {
	cases := []struct {
		name string
		held store.UnvaluedHolding
		want []string
	}{
		{name: "no price in a currency quarry converts is only unpriced", held: unpriced("a-1", "Brokerage", "s-1", "Acme", 12), want: []string{brokerageNoPriceLine}},
		{
			name: "no price and no currency is both unpriced and without a currency",
			held: store.UnvaluedHolding{Date: leftOutDay(12), AccountID: "a-1", Account: "Brokerage", SecurityID: "s-1", Security: "Acme"},
			want: []string{brokerageNoPriceLine, acmeNoCurrencyLine},
		},
		{
			name: "a price with no currency is without a currency", held: noCurrency("a-1", "Brokerage", "s-1", "Acme"),
			want: []string{acmeNoCurrencyLine},
		},
		{
			name: "a price in another currency is not converted", held: pricedIn("EUR", "a-1", "Brokerage", "s-1", "Acme"),
			want: []string{`"Acme" is priced in EUR, which quarry does not convert, so its value is left out of "Brokerage"'s balance`},
		},
		{
			name: "no price in another currency is only unpriced",
			held: store.UnvaluedHolding{
				Date: leftOutDay(12), AccountID: "a-1", Account: "Brokerage", SecurityID: "s-1", Security: "Acme", Currency: new("EUR"),
			},
			want: []string{brokerageNoPriceLine},
		},
		{name: "a priced CAD holding that lacks only a rate has no warning here", held: pricedIn("CAD", "a-1", "Brokerage", "s-1", "Acme"), want: []string{}},
		{name: "a priced USD holding that lacks only a rate has no warning here", held: pricedIn("USD", "a-1", "Brokerage", "s-1", "Acme"), want: []string{}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, document.NetWorthWarnings(snapshot(c.held)))
		})
	}
}

func Test_a_security_with_no_name_is_called_by_its_id(t *testing.T) {
	n := snapshot(noCurrency("a-1", "Brokerage", "sec-7", ""))

	assert.Equal(t, []string{`"sec-7" has no currency in Quicken, so quarry leaves its value out of "Brokerage"'s balance; ` +
		`set its currency in Quicken, then run quarry sync`}, document.NetWorthWarnings(n))
}

func Test_a_name_with_a_quote_is_quoted_with_it_escaped(t *testing.T) {
	n := snapshot(unpriced("a-1", `Bob's "RRSP"`, "s-1", "Acme", 12))

	assert.Equal(t, []string{`"Bob's \"RRSP\"" holds 1 security with no price on or before 2026-03-12, ` +
		`so its balance leaves it out; enter a price in Quicken, then run quarry sync`}, document.NetWorthWarnings(n))
}

func Test_a_security_without_a_currency_is_named_once_per_account_however_many_days_it_was_held(t *testing.T) {
	n := history(
		store.UnvaluedHolding{Date: leftOutDay(5), AccountID: "a-1", Account: "Brokerage", SecurityID: "s-1", Security: "Acme", Priced: true},
		store.UnvaluedHolding{Date: leftOutDay(8), AccountID: "a-1", Account: "Brokerage", SecurityID: "s-1", Security: "Acme", Priced: true})

	assert.Equal(t, []string{acmeNoCurrencyLine}, document.NetWorthWarnings(n))
}

func Test_the_same_security_is_named_for_each_account_that_holds_it(t *testing.T) {
	n := snapshot(noCurrency("a-2", "RRSP", "s-1", "Acme"), noCurrency("a-1", "Brokerage", "s-1", "Acme"))

	assert.Equal(t, []string{
		acmeNoCurrencyLine,
		`"Acme" has no currency in Quicken, so quarry leaves its value out of "RRSP"'s balance; ` +
			`set its currency in Quicken, then run quarry sync`,
	}, document.NetWorthWarnings(n))
}

func Test_warnings_run_by_kind_then_account_not_account_then_kind(t *testing.T) {
	n := snapshot(
		noCurrency("a-1", "Alpha", "s-1", "Acme"),
		unpriced("a-2", "Beta", "s-2", "Bond", 12),
		pricedIn("EUR", "a-1", "Alpha", "s-3", "Euro Fund"))

	assert.Equal(t, []string{
		`"Beta" holds 1 security with no price on or before 2026-03-12, so its balance leaves it out; ` +
			`enter a price in Quicken, then run quarry sync`,
		`"Acme" has no currency in Quicken, so quarry leaves its value out of "Alpha"'s balance; ` +
			`set its currency in Quicken, then run quarry sync`,
		`"Euro Fund" is priced in EUR, which quarry does not convert, so its value is left out of "Alpha"'s balance`,
	}, document.NetWorthWarnings(n))
}

func Test_accounts_run_in_name_order_ignoring_case_then_name_then_id(t *testing.T) {
	n := snapshot(
		unpriced("a-1", "Zeta", "s-1", "Acme", 12),
		unpriced("a-2", "alpha", "s-1", "Acme", 12),
		unpriced("a-3", "Alpha", "s-1", "Acme", 12),
		unpriced("a-5", "Mid", "s-1", "Acme", 12), unpriced("a-5", "Mid", "s-2", "Bond", 12),
		unpriced("a-4", "Mid", "s-1", "Acme", 12))

	assert.Equal(t, []string{
		`"Alpha" holds 1 security with no price on or before 2026-03-12, so its balance leaves it out; enter a price in Quicken, then run quarry sync`,
		`"alpha" holds 1 security with no price on or before 2026-03-12, so its balance leaves it out; enter a price in Quicken, then run quarry sync`,
		`"Mid" holds 1 security with no price on or before 2026-03-12, so its balance leaves it out; enter a price in Quicken, then run quarry sync`,
		`"Mid" holds 2 securities with no price on or before 2026-03-12, so its balance leaves them out; enter prices in Quicken, then run quarry sync`,
		`"Zeta" holds 1 security with no price on or before 2026-03-12, so its balance leaves it out; enter a price in Quicken, then run quarry sync`,
	}, document.NetWorthWarnings(n))
}

func Test_securities_within_an_account_run_in_name_order_ignoring_case_then_name(t *testing.T) {
	n := snapshot(
		noCurrency("a-1", "Brokerage", "s-1", "bond"),
		noCurrency("a-1", "Brokerage", "s-2", "fund"),
		noCurrency("a-1", "Brokerage", "s-3", "Fund"),
		noCurrency("a-1", "Brokerage", "s-4", "Acme"))

	assert.Equal(t, []string{
		`"Acme" has no currency in Quicken, so quarry leaves its value out of "Brokerage"'s balance; set its currency in Quicken, then run quarry sync`,
		`"bond" has no currency in Quicken, so quarry leaves its value out of "Brokerage"'s balance; set its currency in Quicken, then run quarry sync`,
		`"Fund" has no currency in Quicken, so quarry leaves its value out of "Brokerage"'s balance; set its currency in Quicken, then run quarry sync`,
		`"fund" has no currency in Quicken, so quarry leaves its value out of "Brokerage"'s balance; set its currency in Quicken, then run quarry sync`,
	}, document.NetWorthWarnings(n))
}

func Test_warnings_are_an_empty_list_not_nil_when_no_holding_is_left_out(t *testing.T) {
	assert.Equal(t, []string{}, document.NetWorthWarnings(snapshot()))
	assert.Equal(t, []string{}, document.NetWorthWarnings(report.NetWorth{}))
	assert.Equal(t, []string{}, document.AccountsWarnings(report.AccountListing{}))
}
