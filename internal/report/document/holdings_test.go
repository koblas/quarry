package document_test

import (
	"encoding/json"
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/report/document"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_big_money_renders_cents_of_any_size_ungrouped(t *testing.T) {
	const pastInt64 = "18446744073709551616"
	pastInt64Cents, _ := new(big.Int).SetString(pastInt64, 10)
	cases := []struct {
		name  string
		cents *big.Int
		want  string
	}{
		{name: "zero is 0.00", cents: big.NewInt(0), want: "0.00"},
		{name: "under a unit keeps its leading zero", cents: big.NewInt(5), want: "0.05"},
		{name: "a whole unit", cents: big.NewInt(100), want: "1.00"},
		{name: "a negative under a unit is -0.05, not -1.95", cents: big.NewInt(-5), want: "-0.05"},
		{name: "a negative is not grouped", cents: big.NewInt(-123_456), want: "-1234.56"},
		{name: "a value past int64 keeps every digit", cents: pastInt64Cents, want: "184467440737095516.16"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, document.BigMoney(c.cents))
		})
	}
}

func holdingsTestRow() store.Holding {
	return store.Holding{
		AccountID: "a-1", Account: "Brokerage", Security: new("Acme Corp"), Ticker: new("ACME"), SecurityID: "s-1",
		Shares: 1_200_000_000, Price: new(int64(31_420_000)), PriceDate: new(time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)),
		Currency: new("CAD"), Value: big.NewInt(3_770_400), ValueCAD: big.NewInt(3_770_400),
	}
}

func Test_NewHoldings_writes_every_key_in_order_with_a_priced_row_and_a_total(t *testing.T) {
	h := report.Holdings{
		Rows:     []store.Holding{holdingsTestRow()},
		Totals:   []report.HoldingsTotal{{Currency: "CAD", Value: big.NewInt(3_770_400)}},
		AsOf:     time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC),
		Currency: money.CAD,
	}

	got := indented(t, document.NewHoldings(h, []string{}))

	//nolint:testifylint // bytes are the contract
	assert.Equal(t, `{
  "as_of": "2026-10-04",
  "currency": "CAD",
  "account_filter": [],
  "holdings": [
    {
      "account_id": "a-1",
      "account": "Brokerage",
      "account_closed": false,
      "security_id": "s-1",
      "security": "Acme Corp",
      "ticker": "ACME",
      "shares": "1200.000000",
      "price": "31.420000",
      "price_date": "2026-09-29",
      "currency": "CAD",
      "value": "37704.00",
      "converted_value": "37704.00"
    }
  ],
  "totals": [
    {
      "currency": "CAD",
      "value": "37704.00"
    }
  ],
  "warnings": []
}
`, got)
}

func Test_NewHoldings_writes_null_not_empty_for_a_row_with_nothing_to_show(t *testing.T) {
	row := store.Holding{AccountID: "a-1", Account: "Brokerage", AccountClosed: true, SecurityID: "s-1", Shares: -500_000}
	h := report.Holdings{Rows: []store.Holding{row}, Currency: money.CAD}

	var got struct {
		Holdings []map[string]any `json:"holdings"`
	}
	mustReadBack(t, document.NewHoldings(h, nil), &got)

	require.Len(t, got.Holdings, 1)
	assert.Equal(t, map[string]any{
		"account_id": "a-1", "account": "Brokerage", "account_closed": true, "security_id": "s-1",
		"security": nil, "ticker": nil, "shares": "-0.500000", "price": nil, "price_date": nil,
		"currency": nil, "value": nil, "converted_value": nil,
	}, got.Holdings[0])
}

func Test_NewHoldings_reads_converted_value_from_the_reporting_currency(t *testing.T) {
	row := holdingsTestRow()
	row.Currency, row.ValueCAD, row.ValueUSD = new("USD"), big.NewInt(4_000_000), big.NewInt(3_770_400)
	cases := []struct {
		currency     money.Currency
		wantCurrency string
		want         any
	}{
		{currency: money.CAD, wantCurrency: "CAD", want: "40000.00"},
		{currency: money.USD, wantCurrency: "USD", want: "37704.00"},
		{currency: money.Native, wantCurrency: "native", want: nil},
	}

	for _, c := range cases {
		t.Run(c.currency.String(), func(t *testing.T) {
			var got struct {
				Currency string           `json:"currency"`
				Holdings []map[string]any `json:"holdings"`
			}
			h := report.Holdings{Rows: []store.Holding{row}, Currency: c.currency}

			mustReadBack(t, document.NewHoldings(h, nil), &got)

			assert.Equal(t, c.wantCurrency, got.Currency)
			assert.Equal(t, "37704.00", got.Holdings[0]["value"])
			assert.Equal(t, c.want, got.Holdings[0]["converted_value"])
		})
	}
}

func Test_NewHoldings_writes_a_zero_price_and_a_zero_value_as_amounts_not_null(t *testing.T) {
	zero := holdingsTestRow()
	zero.Price, zero.Value, zero.ValueCAD = new(int64(0)), big.NewInt(0), big.NewInt(0)
	h := report.Holdings{
		Rows:     []store.Holding{zero},
		Totals:   []report.HoldingsTotal{{Currency: "CAD", Value: big.NewInt(0)}},
		Currency: money.CAD,
	}
	var got struct {
		Holdings []map[string]any `json:"holdings"`
		Totals   []map[string]any `json:"totals"`
	}

	mustReadBack(t, document.NewHoldings(h, nil), &got)

	require.Len(t, got.Holdings, 1)
	assert.Equal(t, "0.000000", got.Holdings[0]["price"])
	assert.Equal(t, "0.00", got.Holdings[0]["value"])
	assert.Equal(t, "0.00", got.Holdings[0]["converted_value"])
	assert.Equal(t, []map[string]any{{"currency": "CAD", "value": "0.00"}}, got.Totals)
}

func Test_NewHoldings_writes_empty_lists_not_null_for_no_holdings(t *testing.T) {
	var got map[string]any

	raw := []byte(indented(t, document.NewHoldings(report.Holdings{Currency: money.CAD}, nil)))
	require.NoError(t, json.Unmarshal(raw, &got))

	assert.Equal(t, []any{}, got["holdings"])
	assert.Equal(t, []any{}, got["totals"])
	assert.Equal(t, []any{}, got["account_filter"])
	assert.Equal(t, []any{}, got["warnings"])
}

func Test_NewHoldings_writes_a_value_past_int64_in_full(t *testing.T) {
	past, _ := new(big.Int).SetString("18446744073709551616", 10)
	h := report.Holdings{
		Rows:     []store.Holding{{Value: past, ValueCAD: past}},
		Totals:   []report.HoldingsTotal{{Currency: "CAD", Value: past}},
		Currency: money.CAD,
	}

	got := document.NewHoldings(h, nil)

	assert.Equal(t, "184467440737095516.16", *got.Holdings[0].ConvertedValue)
	assert.Equal(t, "184467440737095516.16", got.Totals[0].Value)
}

func Test_HoldingsWarnings_is_empty_not_nil(t *testing.T) {
	assert.Equal(t, []string{}, document.HoldingsWarnings(report.Holdings{Rows: []store.Holding{holdingsTestRow()}, Currency: money.CAD}))
}

func unpricedHoldings(n int) []store.Holding {
	rows := make([]store.Holding, n)
	for i := range rows {
		rows[i] = store.Holding{AccountID: "a-1", SecurityID: "s-1", Shares: 1_000_000, Currency: new("CAD")}
	}
	return rows
}

func Test_HoldingsWarnings_is_empty_when_every_holding_has_a_price(t *testing.T) {
	h := report.Holdings{Rows: []store.Holding{holdingsTestRow()}, Currency: money.CAD}

	assert.Equal(t, []string{}, document.HoldingsWarnings(h))
}

func Test_HoldingsWarnings_say_how_many_holdings_have_no_price(t *testing.T) {
	const tail = ", so they have no value and are left out of the total; enter a price for each in Quicken, then run quarry sync"
	cases := []struct {
		name string
		n    int
		want string
	}{
		{
			name: "one holding is singular throughout", n: 1,
			want: "1 holding has no price on or before 2026-03-12, so it has no value and is left out of the total; " +
				"enter a price for it in Quicken, then run quarry sync",
		},
		{name: "two holdings are plural throughout", n: 2, want: "2 holdings have no price on or before 2026-03-12" + tail},
		{name: "a thousand holdings are grouped", n: 1000, want: "1,000 holdings have no price on or before 2026-03-12" + tail},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := report.Holdings{Rows: unpricedHoldings(c.n), AsOf: time.Date(2026, 3, 12, 0, 0, 0, 0, time.UTC), Currency: money.CAD}

			assert.Equal(t, []string{c.want}, document.HoldingsWarnings(h))
		})
	}
}

func Test_HoldingsWarnings_does_not_count_a_zero_price_holding(t *testing.T) {
	zero := holdingsTestRow()
	zero.Price, zero.Value = new(int64(0)), big.NewInt(0)
	h := report.Holdings{Rows: []store.Holding{zero, zero, unpricedHoldings(1)[0]}, AsOf: time.Date(2026, 3, 12, 0, 0, 0, 0, time.UTC), Currency: money.CAD}

	got := document.HoldingsWarnings(h)

	require.Len(t, got, 1)
	assert.Contains(t, got[0], "1 holding has no price")
}

func Test_HoldingsWarnings_name_the_as_of_day_of_the_listing(t *testing.T) {
	h := report.Holdings{Rows: unpricedHoldings(1), AsOf: time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC), Currency: money.CAD}

	got := document.HoldingsWarnings(h)

	require.Len(t, got, 1)
	assert.Contains(t, got[0], "no price on or before 2025-12-31,")
}

func Test_HoldingsWarnings_count_an_unpriced_holding_whatever_else_it_lacks_or_is(t *testing.T) {
	cases := []struct {
		name string
		row  store.Holding
	}{
		{name: "currency CAD", row: store.Holding{Shares: 1_000_000, Currency: new("CAD")}},
		{name: "currency EUR", row: store.Holding{Shares: 1_000_000, Currency: new("EUR")}},
		{name: "account closed", row: store.Holding{Shares: 1_000_000, Currency: new("CAD"), AccountClosed: true}},
		{name: "negative shares", row: store.Holding{Shares: -500_000, Currency: new("CAD")}},
	}

	for _, c := range cases {
		for _, currency := range []money.Currency{money.CAD, money.Native} {
			t.Run(c.name+" in "+currency.String(), func(t *testing.T) {
				h := report.Holdings{Rows: []store.Holding{c.row}, Currency: currency}

				got := document.HoldingsWarnings(h)

				require.Len(t, got, 1)
				assert.Contains(t, got[0], "1 holding has no price")
			})
		}
	}
}

// heldSecurity is one holding of security id in account a-1; a nil price leaves it unpriced.
func heldSecurity(id string, name, currency *string, price *int64) store.Holding {
	return store.Holding{AccountID: "a-1", SecurityID: id, Security: name, Currency: currency, Price: price, Shares: 1_000_000}
}

func Test_HoldingsWarnings_say_which_securities_have_no_conversion(t *testing.T) {
	const (
		noCurrencyFormat = "%s has no currency in Quicken, so quarry leaves its value out of the total; " +
			"set its currency in Quicken, then run quarry sync"
		otherFormat = "%s is priced in %s, which quarry does not convert, so its value is left out of the total"
		noPriceOne  = "1 holding has no price on or before 2026-03-12, so it has no value and is left out of the total; enter a price for it in Quicken, then run quarry sync"
		mystery     = `"Mystery Fund"`
		euro        = `"Euro Fund"`
	)
	priced := new(int64(1_000_000))
	twoAccounts := []store.Holding{heldSecurity("s-1", new("Mystery Fund"), nil, priced), heldSecurity("s-1", new("Mystery Fund"), nil, priced)}
	twoAccounts[1].AccountID = "a-2"
	convertedUSD := heldSecurity("s-2", new("B"), new("USD"), priced)
	convertedUSD.ValueCAD = big.NewInt(136)
	cases := []struct {
		name     string
		rows     []store.Holding
		currency money.Currency
		want     []string
	}{
		{
			name: "no currency in CAD", currency: money.CAD,
			rows: []store.Holding{heldSecurity("s-1", new("Mystery Fund"), nil, priced)},
			want: []string{fmt.Sprintf(noCurrencyFormat, mystery)},
		},
		{
			name: "no currency in USD", currency: money.USD,
			rows: []store.Holding{heldSecurity("s-1", new("Mystery Fund"), nil, priced)},
			want: []string{fmt.Sprintf(noCurrencyFormat, mystery)},
		},
		{
			name: "no currency in native", currency: money.Native,
			rows: []store.Holding{heldSecurity("s-1", new("Mystery Fund"), nil, priced)},
			want: []string{fmt.Sprintf(noCurrencyFormat, mystery)},
		},
		{
			name: "EUR in CAD", currency: money.CAD,
			rows: []store.Holding{heldSecurity("s-2", new("Euro Fund"), new("EUR"), priced)},
			want: []string{fmt.Sprintf(otherFormat, euro, "EUR")},
		},
		{
			name: "EUR in USD", currency: money.USD,
			rows: []store.Holding{heldSecurity("s-2", new("Euro Fund"), new("EUR"), priced)},
			want: []string{fmt.Sprintf(otherFormat, euro, "EUR")},
		},
		{
			name: "EUR in native is totalled as it is, so no line", currency: money.Native,
			rows: []store.Holding{heldSecurity("s-2", new("Euro Fund"), new("EUR"), priced)},
			want: []string{},
		},
		{
			name: "CAD and USD securities have no line", currency: money.CAD,
			rows: []store.Holding{heldSecurity("s-1", new("A"), new("CAD"), priced), convertedUSD},
			want: []string{},
		},
		{
			name: "one security held in two accounts has one line", currency: money.CAD,
			rows: twoAccounts,
			want: []string{fmt.Sprintf(noCurrencyFormat, mystery)},
		},
		{
			name: "several lines keep table order and name the currency as stored", currency: money.CAD,
			rows: []store.Holding{
				heldSecurity("s-1", new("Zeta"), nil, priced), heldSecurity("s-2", new("Alpha"), new("GBP"), priced),
				heldSecurity("s-3", new("Mid"), nil, priced), heldSecurity("s-4", new("Beta"), new("EUR"), priced),
			},
			want: []string{
				fmt.Sprintf(noCurrencyFormat, `"Zeta"`), fmt.Sprintf(noCurrencyFormat, `"Mid"`),
				fmt.Sprintf(otherFormat, `"Alpha"`, "GBP"), fmt.Sprintf(otherFormat, `"Beta"`, "EUR"),
			},
		},
		{
			name: "no price, no currency and other currency come in that order", currency: money.CAD,
			rows: []store.Holding{
				heldSecurity("s-1", new("Mystery Fund"), nil, priced), heldSecurity("s-2", new("Euro Fund"), new("EUR"), priced),
				heldSecurity("s-3", new("Bare"), new("CAD"), nil),
			},
			want: []string{noPriceOne, fmt.Sprintf(noCurrencyFormat, mystery), fmt.Sprintf(otherFormat, euro, "EUR")},
		},
		{
			name: "an unpriced security with no currency has both lines", currency: money.CAD,
			rows: []store.Holding{heldSecurity("s-1", new("Mystery Fund"), nil, nil)},
			want: []string{noPriceOne, fmt.Sprintf(noCurrencyFormat, mystery)},
		},
		{
			name: "an unpriced EUR security is not priced in EUR, so only the no-price line", currency: money.CAD,
			rows: []store.Holding{heldSecurity("s-2", new("Euro Fund"), new("EUR"), nil)},
			want: []string{noPriceOne},
		},
		{
			name: "a security with no name is called by its id", currency: money.CAD,
			rows: []store.Holding{heldSecurity("sec-9", nil, nil, priced)},
			want: []string{fmt.Sprintf(noCurrencyFormat, `"sec-9"`)},
		},
		{
			name: "a quote in the name is escaped", currency: money.CAD,
			rows: []store.Holding{heldSecurity("s-1", new(`Fund "X"`), nil, priced)},
			want: []string{fmt.Sprintf(noCurrencyFormat, `"Fund \"X\""`)},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := report.Holdings{Rows: c.rows, AsOf: time.Date(2026, 3, 12, 0, 0, 0, 0, time.UTC), Currency: c.currency}

			assert.Equal(t, c.want, document.HoldingsWarnings(h))
		})
	}
}

func Test_big_money_does_not_change_the_value_it_renders(t *testing.T) {
	cents := big.NewInt(-123_456)

	_ = document.BigMoney(cents)

	assert.Equal(t, "-123456", cents.String())
}

const (
	accountTypeChequing = "chequing"
	accountTypeSavings  = "savings"
)

func namedAccount(id, name, accountType string) store.Account {
	return store.Account{ID: id, Name: name, Type: accountType}
}

func notInvestmentLine(name string) string {
	return `account "` + name + `" is not a brokerage or retirement account, so it has no holdings`
}

func Test_HoldingsWarnings_name_each_named_account_that_is_not_an_investment_account(t *testing.T) {
	cases := []struct {
		name     string
		accounts []store.Account
		want     []string
	}{
		{
			name: "in the order the accounts were given, though the table would sort the other way",
			accounts: []store.Account{
				namedAccount("a-1", "IRA", accountTypeSavings),
				namedAccount("a-2", "brokerage", accountTypeChequing),
			},
			want: []string{notInvestmentLine("IRA"), notInvestmentLine("brokerage"), namedNothingLine},
		},
		{
			name: "in the order given when two names differ only in case",
			accounts: []store.Account{
				namedAccount("a-1", "alpha", accountTypeChequing),
				namedAccount("a-2", "Alpha", accountTypeSavings),
			},
			want: []string{notInvestmentLine("alpha"), notInvestmentLine("Alpha"), namedNothingLine},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := document.HoldingsWarnings(report.Holdings{Accounts: c.accounts, Currency: money.CAD, AsOf: emptyAsOf})

			assert.Equal(t, c.want, got)
		})
	}
}

func Test_HoldingsWarnings_keep_the_given_order_of_the_named_accounts_for_the_caller(t *testing.T) {
	zeta, alpha := namedAccount("a-1", "Zeta", accountTypeChequing), namedAccount("a-2", "Alpha", accountTypeChequing)
	h := report.Holdings{Accounts: []store.Account{zeta, alpha}, Currency: money.CAD}

	_ = document.HoldingsWarnings(h)

	assert.Equal(t, []store.Account{zeta, alpha}, h.Accounts)
}

func Test_HoldingsWarnings_leave_out_a_named_investment_account_whatever_its_state(t *testing.T) {
	closed := namedAccount("a-3", "Old Brokerage", store.AccountTypeBrokerage)
	closed.Closed = true
	hidden := namedAccount("a-4", "Hidden Brokerage", store.AccountTypeBrokerage)
	hidden.NotInReports = true
	h := report.Holdings{
		Rows:     []store.Holding{holdingsTestRow()},
		Accounts: []store.Account{namedAccount("a-1", "Brokerage", store.AccountTypeBrokerage), namedAccount("a-2", "RRSP", store.AccountTypeRetirement), closed, hidden},
		Currency: money.CAD,
	}

	got := document.HoldingsWarnings(h)

	assert.Empty(t, got)
}

func Test_HoldingsWarnings_put_the_non_investment_line_before_the_no_price_line(t *testing.T) {
	row := holdingsTestRow()
	row.Price = nil
	h := report.Holdings{
		Rows:     []store.Holding{row},
		Accounts: []store.Account{namedAccount("a-1", "Chequing", accountTypeChequing), namedAccount(row.AccountID, row.Account, store.AccountTypeBrokerage)},
		Currency: money.CAD,
	}

	got := document.HoldingsWarnings(h)

	require.Len(t, got, 2)
	assert.Equal(t, notInvestmentLine("Chequing"), got[0])
	assert.Contains(t, got[1], "no price on or before")
}

func Test_holdings_json_account_filter_lists_the_named_accounts(t *testing.T) {
	cases := []struct {
		name     string
		accounts []store.Account
		want     string
	}{
		{
			name: "in the order given",
			accounts: []store.Account{
				namedAccount("a-2", "Zeta", store.AccountTypeBrokerage),
				namedAccount("a-1", "Alpha", accountTypeChequing),
			},
			want: `[{"id":"a-2","name":"Zeta"},{"id":"a-1","name":"Alpha"}]`,
		},
		{name: "as [] when none is named", accounts: nil, want: `[]`},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var got struct {
				AccountFilter json.RawMessage `json:"account_filter"`
			}

			mustReadBack(t, document.NewHoldings(report.Holdings{Accounts: c.accounts, Currency: money.CAD}, nil), &got)

			assert.JSONEq(t, c.want, string(got.AccountFilter))
		})
	}
}

var emptyAsOf = time.Date(2026, 3, 12, 0, 0, 0, 0, time.UTC)

const namedNothingLine = "no holdings on 2026-03-12 in the named accounts; they have no investment transactions"

func Test_HoldingsWarnings_word_an_empty_result_by_what_the_store_holds(t *testing.T) {
	named := []store.Account{namedAccount("a-1", "Brokerage", store.AccountTypeBrokerage)}
	cases := []struct {
		name     string
		accounts []store.Account
		asOf     time.Time
		first    time.Time
		last     time.Time
		want     string
	}{
		{
			name: "every account, transactions on other days", asOf: march(1), first: march(2), last: march(5),
			want: "no holdings on 2026-03-01; the store's investment transactions run 2026-03-02 to 2026-03-05",
		},
		{
			name: "every account, no transactions at all", asOf: march(1),
			want: "no holdings on 2026-03-01; the store has no investment transactions",
		},
		{
			name: "named accounts, transactions on other days", accounts: named, asOf: march(1), first: march(2), last: march(5),
			want: "no holdings on 2026-03-01 in the named accounts; their investment transactions run 2026-03-02 to 2026-03-05",
		},
		{
			name: "named accounts, no transactions at all", accounts: named, asOf: march(1),
			want: "no holdings on 2026-03-01 in the named accounts; they have no investment transactions",
		},
		{
			name: "one day of transactions runs from that day to itself", asOf: march(1), first: march(2), last: march(2),
			want: "no holdings on 2026-03-01; the store's investment transactions run 2026-03-02 to 2026-03-02",
		},
		{
			name: "a day after everything was sold gives the same span line", asOf: march(9), first: march(2), last: march(5),
			want: "no holdings on 2026-03-09; the store's investment transactions run 2026-03-02 to 2026-03-05",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := report.Holdings{
				Accounts: c.accounts, AsOf: c.asOf, FirstTransaction: c.first, LastTransaction: c.last, Currency: money.CAD,
			}

			got := document.HoldingsWarnings(h)

			assert.Equal(t, []string{c.want}, got)
		})
	}
}

func Test_HoldingsWarnings_say_nothing_about_an_empty_result_when_something_is_held(t *testing.T) {
	h := report.Holdings{
		Rows: []store.Holding{holdingsTestRow()}, AsOf: march(1), FirstTransaction: march(2), LastTransaction: march(5), Currency: money.CAD,
	}

	got := document.HoldingsWarnings(h)

	assert.Empty(t, got)
}

func Test_HoldingsWarnings_put_the_non_investment_line_before_the_empty_line(t *testing.T) {
	h := report.Holdings{
		Accounts: []store.Account{namedAccount("a-1", "Chequing", accountTypeChequing)}, AsOf: emptyAsOf, Currency: money.CAD,
	}

	got := document.HoldingsWarnings(h)

	assert.Equal(t, []string{notInvestmentLine("Chequing"), namedNothingLine}, got)
}

var (
	beforeFirstRate = time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC)
	firstRateDay    = time.Date(2026, 3, 10, 0, 0, 0, 0, time.UTC)
)

const noRateLinePrefix = "valued on 2026-03-09, before 2026-03-10, the first exchange rate in the store, "

// unconvertedRow is a priced security in code with no converted value; id and account tell two rows apart.
func unconvertedRow(id, account, code string) store.Holding {
	row := heldSecurity(id, new("Fund "+id), new(code), new(int64(1_000_000)))
	row.AccountID = account
	return row
}

func Test_HoldingsWarnings_say_one_holding_is_not_converted_before_the_first_rate(t *testing.T) {
	h := report.Holdings{Rows: []store.Holding{unconvertedRow("s-1", "a-1", "USD")}, Currency: money.CAD, AsOf: beforeFirstRate, FirstRate: firstRateDay}

	got := document.HoldingsWarnings(h)

	assert.Equal(t, []string{"1 holding " + noRateLinePrefix + "is not converted to CAD and is totalled in USD"}, got)
}

func Test_HoldingsWarnings_count_each_row_that_needs_a_rate_not_each_security(t *testing.T) {
	rows := []store.Holding{unconvertedRow("s-1", "a-1", "USD"), unconvertedRow("s-1", "a-2", "USD")}
	h := report.Holdings{Rows: rows, Currency: money.CAD, AsOf: beforeFirstRate, FirstRate: firstRateDay}

	got := document.HoldingsWarnings(h)

	assert.Equal(t, []string{"2 holdings " + noRateLinePrefix + "are not converted to CAD and are totalled in USD"}, got)
}

func Test_HoldingsWarnings_swap_the_currencies_in_a_usd_report(t *testing.T) {
	h := report.Holdings{Rows: []store.Holding{unconvertedRow("s-1", "a-1", "CAD")}, Currency: money.USD, AsOf: beforeFirstRate, FirstRate: firstRateDay}

	got := document.HoldingsWarnings(h)

	assert.Equal(t, []string{"1 holding " + noRateLinePrefix + "is not converted to USD and is totalled in CAD"}, got)
}

func Test_HoldingsWarnings_say_the_store_has_no_rates_when_it_holds_none(t *testing.T) {
	rows := []store.Holding{unconvertedRow("s-1", "a-1", "USD"), unconvertedRow("s-2", "a-1", "USD")}
	h := report.Holdings{Rows: rows, Currency: money.CAD, AsOf: beforeFirstRate}

	got := document.HoldingsWarnings(h)

	assert.Equal(t, []string{"the store has no exchange rates, so values are listed in each security's own currency; run quarry sync to fetch them"}, got)
}

func Test_HoldingsWarnings_are_silent_when_no_row_needs_a_rate(t *testing.T) {
	converted := unconvertedRow("s-2", "a-1", "USD")
	converted.ValueCAD = big.NewInt(136)
	cases := []struct {
		name      string
		row       store.Holding
		currency  money.Currency
		firstRate time.Time
	}{
		{name: "a CAD row in a CAD report before the first rate", row: unconvertedRow("s-1", "a-1", "CAD"), currency: money.CAD, firstRate: firstRateDay},
		{name: "a CAD row in a CAD report with no rates at all", row: unconvertedRow("s-1", "a-1", "CAD"), currency: money.CAD},
		{name: "a converted USD row on a day after the first rate", row: converted, currency: money.CAD, firstRate: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)},
		{name: "a native listing before the first rate", row: unconvertedRow("s-1", "a-1", "USD"), currency: money.Native, firstRate: firstRateDay},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := report.Holdings{Rows: []store.Holding{c.row}, Currency: c.currency, AsOf: beforeFirstRate, FirstRate: c.firstRate}

			assert.Equal(t, []string{}, document.HoldingsWarnings(h))
		})
	}
}

func Test_HoldingsWarnings_put_the_rate_line_after_no_price_and_before_no_currency_and_count_only_cad_and_usd_rows(t *testing.T) {
	const (
		noPrice    = "1 holding has no price on or before 2026-03-09, so it has no value and is left out of the total; enter a price for it in Quicken, then run quarry sync"
		noCurrency = `"Mystery Fund" has no currency in Quicken, so quarry leaves its value out of the total; set its currency in Quicken, then run quarry sync`
		other      = `"Euro Fund" is priced in EUR, which quarry does not convert, so its value is left out of the total`
	)
	priced := new(int64(1_000_000))
	rows := []store.Holding{
		heldSecurity("s-0", new("Bare"), new("CAD"), nil),
		heldSecurity("s-1", new("Mystery Fund"), nil, priced),
		heldSecurity("s-2", new("Euro Fund"), new("EUR"), priced),
		unconvertedRow("s-3", "a-1", "USD"),
	}
	h := report.Holdings{Rows: rows, Currency: money.CAD, AsOf: beforeFirstRate, FirstRate: firstRateDay}

	got := document.HoldingsWarnings(h)

	assert.Equal(t, []string{noPrice, "1 holding " + noRateLinePrefix + "is not converted to CAD and is totalled in USD", noCurrency, other}, got)
}
