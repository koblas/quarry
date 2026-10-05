package document_test

import (
	"encoding/json"
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

var acbAsOf = time.Date(2026, time.October, 5, 0, 0, 0, 0, time.UTC)

// acbDocumentFixture is XEQT held (a USD buy and a CAD sale) beside a sold-out security with no ticker or
// currency; its 2024 has a flagged loss and a flagged unknown-cost sale, one in each security.
func acbDocumentFixture() report.ACB {
	xeqt := store.Security{ID: "sec-41", Name: "iShares Core Equity ETF", Ticker: new("XEQT"), Currency: new("CAD")}
	gone := store.Security{ID: "sec-7", Name: "Gone Fund"}
	lossSale := report.ACBSale{
		ID: "itxn-9", Date: time.Date(2024, time.March, 4, 0, 0, 0, 0, time.UTC), SecurityID: "sec-41",
		AccountID: "acct-3", Account: "Margin", Shares: big.NewRat(10, 1),
		Proceeds: 100_000, Outlays: 999, ACBRemoved: 120_000, Gain: -20_999, PossibleSuperficialLoss: true,
	}
	unknownSale := report.ACBSale{
		ID: "itxn-12", Date: time.Date(2024, time.April, 5, 0, 0, 0, 0, time.UTC), SecurityID: "sec-7",
		AccountID: "acct-3", Account: "Margin", Shares: big.NewRat(1, 4),
		Proceeds: 5_000, Outlays: 0, ACBRemoved: 0, Gain: 5_000, UnknownCost: true,
	}
	buy := report.ACBEvent{
		ID: "itxn-1", Date: time.Date(2023, time.January, 3, 0, 0, 0, 0, time.UTC), AccountID: "acct-3", Account: "Margin",
		Action: "buy", Shares: big.NewRat(410, 1), Amount: new(int64(-123_456)), Currency: "USD", Rate: 1_351_234, CAD: -166_814,
		Held: big.NewRat(410, 1), ACB: 1_041_233,
	}
	sell := report.ACBEvent{
		ID: "itxn-9", Date: lossSale.Date, AccountID: "acct-3", Account: "Margin", Action: "sell",
		Shares: big.NewRat(10, 1), Amount: new(int64(100_000)), Currency: "CAD", CAD: 100_000, Outlays: new(int64(999)),
		Held: big.NewRat(400, 1), ACB: 1_041_233 - 120_000, Gain: -20_999, Realized: true,
	}
	return report.ACB{
		AsOf: acbAsOf,
		Years: []report.ACBYear{{
			Year: 2024, Sales: []report.ACBSale{lossSale, unknownSale},
			Proceeds: 105_000, Outlays: 999, ACBRemoved: 120_000, Gain: -15_999,
		}},
		Securities: []report.ACBSecurity{
			{Security: xeqt, Shares: big.NewRat(400, 1), ACB: 1_041_233 - 120_000, Events: []report.ACBEvent{buy, sell}},
			{Security: gone, Shares: new(big.Rat), Incomplete: true},
		},
	}
}

// acbJSON is a's document as the command writes it.
func acbJSON(t *testing.T, a report.ACB, warnings []string) []byte {
	t.Helper()
	out, err := json.Marshal(document.NewACB(a, warnings))
	require.NoError(t, err)
	return out
}

// firstOf is the first element of the array under key in the JSON object doc.
func firstOf(t *testing.T, doc []byte, key string) json.RawMessage {
	t.Helper()
	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(doc, &fields))
	var elements []json.RawMessage
	require.NoError(t, json.Unmarshal(fields[key], &elements))
	require.NotEmpty(t, elements)
	return elements[0]
}

func Test_NewACB_writes_every_key_of_every_object_in_order(t *testing.T) {
	doc := acbJSON(t, acbDocumentFixture(), []string{"a warning"})
	year := firstOf(t, doc, "years")
	security := firstOf(t, doc, "securities")

	assert.Equal(t, []string{"as_of", "currency", "year", "years", "securities", "warnings"}, topLevelKeys(t, doc))
	assert.Equal(t, []string{
		"year", "sale_count", "proceeds", "outlays", "acb", "gain", "return_of_capital_gain", "possible_superficial_losses",
		"unknown_cost_sales", "sales",
	}, topLevelKeys(t, year))
	assert.Equal(t, []string{
		"date", "investment_transaction_id", "security_id", "security", "ticker", "account_id", "account", "shares",
		"proceeds", "outlays", "acb", "gain", "possible_superficial_loss", "unknown_cost",
	}, topLevelKeys(t, firstOf(t, year, "sales")))
	assert.Equal(t, []string{
		"security_id", "security", "ticker", "currency", "shares", "acb", "acb_per_share", "incomplete", "events",
	}, topLevelKeys(t, security))
	assert.Equal(t, []string{
		"date", "investment_transaction_id", "account_id", "account", "action", "shares", "amount", "amount_currency",
		"usd_cad", "cad", "outlays", "shares_held", "acb", "gain", "unknown_cost",
	}, topLevelKeys(t, firstOf(t, security, "events")))
}

func Test_NewACB_reads_back_the_year_totals_and_each_sale_with_its_security_and_account(t *testing.T) {
	var got document.ACB

	require.NoError(t, json.Unmarshal(acbJSON(t, acbDocumentFixture(), []string{"a warning"}), &got))

	assert.Equal(t, "2026-10-05", got.AsOf)
	assert.Equal(t, "CAD", got.Currency)
	assert.Nil(t, got.Year)
	assert.Equal(t, []string{"a warning"}, got.Warnings)
	require.Len(t, got.Years, 1)
	year := got.Years[0]
	assert.Equal(t, document.ACBYear{
		Year: 2024, SaleCount: 2, Proceeds: "1050.00", Outlays: "9.99", ACB: "1200.00", Gain: "-159.99",
		ReturnOfCapitalGain: "0.00", PossibleSuperficialLosses: 1, UnknownCostSales: 1, Sales: year.Sales,
	}, year)
	assert.Equal(t, []document.ACBSale{
		{
			Date: "2024-03-04", InvestmentTransactionID: "itxn-9", SecurityID: "sec-41", Security: "iShares Core Equity ETF",
			Ticker: new("XEQT"), AccountID: "acct-3", Account: "Margin", Shares: "10.000000", Proceeds: "1000.00",
			Outlays: "9.99", ACB: "1200.00", Gain: "-209.99", PossibleSuperficialLoss: true,
		},
		{
			Date: "2024-04-05", InvestmentTransactionID: "itxn-12", SecurityID: "sec-7", Security: "Gone Fund",
			AccountID: "acct-3", Account: "Margin", Shares: "0.250000", Proceeds: "50.00", Outlays: "0.00", ACB: "0.00",
			Gain: "50.00", UnknownCost: true,
		},
	}, year.Sales)
}

func Test_NewACB_writes_a_sale_only_year_return_of_capital_gain_as_zero(t *testing.T) {
	var got document.ACB

	require.NoError(t, json.Unmarshal(acbJSON(t, acbDocumentFixture(), nil), &got))

	assert.Equal(t, "0.00", got.Years[0].ReturnOfCapitalGain)
}

func Test_NewACB_writes_a_year_with_only_a_return_of_capital_gain(t *testing.T) {
	a := report.ACB{AsOf: acbAsOf, Years: []report.ACBYear{{Year: 2025, ReturnOfCapitalGain: 125_000}}}
	var got document.ACB

	require.NoError(t, json.Unmarshal(acbJSON(t, a, nil), &got))

	assert.Equal(t, []document.ACBYear{{
		Year: 2025, SaleCount: 0, Proceeds: "0.00", Outlays: "0.00", ACB: "0.00", Gain: "0.00",
		ReturnOfCapitalGain: "1250.00", Sales: []document.ACBSale{},
	}}, got.Years)
}

func Test_NewACB_reads_back_a_held_security_with_its_per_share_and_a_sold_out_one_with_none(t *testing.T) {
	var got document.ACB

	require.NoError(t, json.Unmarshal(acbJSON(t, acbDocumentFixture(), nil), &got))

	require.Len(t, got.Securities, 2)
	held, soldOut := got.Securities[0], got.Securities[1]
	assert.Equal(t, document.ACBSecurity{
		SecurityID: "sec-41", Security: "iShares Core Equity ETF", Ticker: new("XEQT"), Currency: new("CAD"),
		Shares: "400.000000", ACB: "9212.33", ACBPerShare: new("23.0308"), Events: held.Events,
	}, held)
	assert.Equal(t, document.ACBSecurity{
		SecurityID: "sec-7", Security: "Gone Fund", Shares: "0.000000", ACB: "0.00", Incomplete: true, Events: []document.ACBEvent{},
	}, soldOut)
}

func Test_NewACB_reads_back_a_usd_buy_with_its_rate_and_a_sale_with_its_outlays_and_gain(t *testing.T) {
	var got document.ACB

	require.NoError(t, json.Unmarshal(acbJSON(t, acbDocumentFixture(), nil), &got))

	assert.Equal(t, []document.ACBEvent{
		{
			Date: "2023-01-03", InvestmentTransactionID: new("itxn-1"), AccountID: new("acct-3"), Account: new("Margin"),
			Action: "buy", Shares: "410.000000", Amount: new("-1234.56"), AmountCurrency: new("USD"), USDCAD: new("1.351234"),
			CAD: new("-1668.14"), SharesHeld: "410.000000", ACB: "10412.33",
		},
		{
			Date: "2024-03-04", InvestmentTransactionID: new("itxn-9"), AccountID: new("acct-3"), Account: new("Margin"),
			Action: "sell", Shares: "10.000000", Amount: new("1000.00"), AmountCurrency: new("CAD"),
			CAD: new("1000.00"), Outlays: new("9.99"), SharesHeld: "400.000000", ACB: "9212.33", Gain: new("-209.99"),
		},
	}, got.Securities[0].Events)
}

func Test_NewACB_writes_an_unvalued_event_with_a_null_cad_and_gain_and_keeps_its_position(t *testing.T) {
	a := report.ACB{AsOf: acbAsOf, Securities: []report.ACBSecurity{{
		Security: store.Security{ID: "sec-41", Name: "XEQT"}, Shares: big.NewRat(1, 1), Incomplete: true,
		Events: []report.ACBEvent{{
			Date: acbAsOf, Action: "sell", Shares: big.NewRat(1, 1), Amount: new(int64(500)), Currency: "USD",
			Outlays: new(int64(0)), Held: big.NewRat(1, 1), ACB: 700, Gain: -700, Realized: true, Unvalued: true,
		}},
	}}}

	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(firstOf(t, acbJSON(t, a, nil), "securities"), &raw))
	var events []map[string]any
	require.NoError(t, json.Unmarshal(raw["events"], &events))

	assert.Nil(t, events[0]["cad"])
	assert.Nil(t, events[0]["gain"])
	assert.Equal(t, "7.00", events[0]["acb"])
}

func Test_NewACB_writes_a_break_even_sale_gain_and_outlays_as_zero_and_a_buy_gain_as_null(t *testing.T) {
	a := report.ACB{AsOf: acbAsOf, Securities: []report.ACBSecurity{{
		Security: store.Security{ID: "sec-41", Name: "XEQT"}, Shares: new(big.Rat),
		Events: []report.ACBEvent{
			{Date: acbAsOf, Action: "buy", Shares: big.NewRat(1, 1), Amount: new(int64(-500)), Currency: "CAD", CAD: -500, Held: big.NewRat(1, 1), ACB: 500},
			{
				Date: acbAsOf, Action: "sell", Shares: big.NewRat(1, 1), Amount: new(int64(500)), Currency: "CAD", CAD: 500,
				Outlays: new(int64(0)), Held: new(big.Rat), Realized: true,
			},
		},
	}}}

	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(firstOf(t, acbJSON(t, a, nil), "securities"), &raw))
	var events []map[string]any
	require.NoError(t, json.Unmarshal(raw["events"], &events))

	assert.Nil(t, events[0]["gain"])
	assert.Nil(t, events[0]["outlays"])
	assert.Equal(t, "0.00", events[1]["gain"])
	assert.Equal(t, "0.00", events[1]["outlays"])
}

func Test_NewACB_writes_unknown_cost_true_on_an_event_that_moved_shares_with_no_cost_and_false_on_one_that_did_not(t *testing.T) {
	a := report.ACB{AsOf: acbAsOf, Securities: []report.ACBSecurity{{
		Security: store.Security{ID: "sec-41", Name: "XEQT"}, Shares: new(big.Rat),
		Events: []report.ACBEvent{
			{Date: acbAsOf, Action: "add_shares", Shares: big.NewRat(1, 1), Amount: new(int64(0)), Currency: "CAD", Held: big.NewRat(1, 1), UnknownCost: true},
			{Date: acbAsOf, Action: "buy", Shares: big.NewRat(1, 1), Amount: new(int64(-500)), Currency: "CAD", CAD: -500, Held: big.NewRat(2, 1), ACB: 500},
		},
	}}}

	got := document.NewACB(a, nil)

	assert.True(t, got.Securities[0].Events[0].UnknownCost)
	assert.False(t, got.Securities[0].Events[1].UnknownCost)
}

func Test_NewACB_writes_an_adjustment_event_with_no_transaction_account_or_amount(t *testing.T) {
	a := report.ACB{AsOf: acbAsOf, Securities: []report.ACBSecurity{{
		Security: store.Security{ID: "sec-41", Name: "XEQT"}, Shares: big.NewRat(10, 1), ACB: 500,
		Events: []report.ACBEvent{{
			Date: acbAsOf, Action: "return of capital", Shares: new(big.Rat), CAD: 250, Held: big.NewRat(10, 1), ACB: 500,
		}},
	}}}

	doc := acbJSON(t, a, nil)
	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(firstOf(t, doc, "securities"), &raw))
	var events []map[string]any
	require.NoError(t, json.Unmarshal(raw["events"], &events))

	assert.Equal(t, map[string]any{
		"date": "2026-10-05", "investment_transaction_id": nil, "account_id": nil, "account": nil,
		"action": "return of capital", "shares": "0.000000", "amount": nil, "amount_currency": nil, "usd_cad": nil,
		"cad": "2.50", "outlays": nil, "shares_held": "10.000000", "acb": "5.00", "gain": nil, "unknown_cost": false,
	}, events[0])
}

func Test_NewACB_writes_a_return_of_capital_above_the_acb_event_with_the_excess_as_its_gain(t *testing.T) {
	a := report.ACB{AsOf: acbAsOf, Securities: []report.ACBSecurity{{
		Security: store.Security{ID: "sec-41", Name: "XEQT"}, Shares: big.NewRat(10, 1),
		Events: []report.ACBEvent{{
			Date: acbAsOf, Action: "return of capital", Shares: new(big.Rat), CAD: 230_000, Held: big.NewRat(10, 1),
			Gain: 125_000, Realized: true,
		}},
	}}}

	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(firstOf(t, acbJSON(t, a, nil), "securities"), &raw))
	var events []map[string]any
	require.NoError(t, json.Unmarshal(raw["events"], &events))

	assert.Equal(t, map[string]any{
		"date": "2026-10-05", "investment_transaction_id": nil, "account_id": nil, "account": nil,
		"action": "return of capital", "shares": "0.000000", "amount": nil, "amount_currency": nil, "usd_cad": nil,
		"cad": "2300.00", "outlays": nil, "shares_held": "10.000000", "acb": "0.00", "gain": "1250.00", "unknown_cost": false,
	}, events[0])
}

func Test_NewACB_trims_a_usd_cad_rate_to_at_least_four_decimals(t *testing.T) {
	cases := []struct {
		name string
		rate money.Rate
		want *string
	}{
		{name: "a short rate is padded to four decimals", rate: 1_250_000, want: new("1.2500")},
		{name: "a six decimal rate is kept", rate: 1_351_234, want: new("1.351234")},
		{name: "a five decimal rate loses its trailing zero", rate: 1_234_560, want: new("1.23456")},
		{name: "a rate with a zero decimal in the middle keeps it", rate: 1_300_040, want: new("1.30004")},
		{name: "no rate on file is null", rate: 0, want: nil},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := report.ACB{AsOf: acbAsOf, Securities: []report.ACBSecurity{{
				Security: store.Security{ID: "sec-1", Name: "VTI"}, Shares: new(big.Rat),
				Events: []report.ACBEvent{{Shares: new(big.Rat), Amount: new(int64(100)), Currency: "USD", Rate: c.rate, Held: new(big.Rat)}},
			}}}

			got := document.NewACB(a, nil)

			assert.Equal(t, c.want, got.Securities[0].Events[0].USDCAD)
		})
	}
}

func Test_NewACB_writes_empty_arrays_not_null_for_an_empty_acb(t *testing.T) {
	doc := acbJSON(t, report.ACB{AsOf: acbAsOf}, nil)
	var fields map[string]json.RawMessage

	require.NoError(t, json.Unmarshal(doc, &fields))

	assert.JSONEq(t, "[]", string(fields["years"]))
	assert.JSONEq(t, "[]", string(fields["securities"]))
	assert.JSONEq(t, "[]", string(fields["warnings"]))
	assert.JSONEq(t, "null", string(fields["year"]))
}

func Test_NewACB_names_the_year_it_is_cut_to_and_lists_it_with_no_sales_when_it_has_none(t *testing.T) {
	cut := report.ACB{AsOf: acbAsOf, Year: 2025}.InYear()
	var got document.ACB

	require.NoError(t, json.Unmarshal(acbJSON(t, cut, nil), &got))

	assert.Equal(t, new(2025), got.Year)
	assert.Equal(t, []document.ACBYear{{
		Year: 2025, Proceeds: "0.00", Outlays: "0.00", ACB: "0.00", Gain: "0.00", ReturnOfCapitalGain: "0.00", Sales: []document.ACBSale{},
	}}, got.Years)
	assert.Empty(t, got.Securities)
}

func Test_NewACB_writes_the_year_entry_of_a_cut_with_return_of_capital_gain_right_after_gain(t *testing.T) {
	doc := acbJSON(t, report.ACB{AsOf: acbAsOf, Year: 2025}.InYear(), nil)

	assert.Equal(t, []string{
		"year", "sale_count", "proceeds", "outlays", "acb", "gain", "return_of_capital_gain", "possible_superficial_losses",
		"unknown_cost_sales", "sales",
	}, topLevelKeys(t, firstOf(t, doc, "years")))
	assert.Contains(t, string(doc), `"gain":"0.00","return_of_capital_gain":"0.00","possible_superficial_losses":0`)
}

func Test_NewACB_writes_no_year_and_empty_arrays_for_a_report_with_nothing_in_it(t *testing.T) {
	doc := acbJSON(t, report.ACB{AsOf: acbAsOf}, nil)

	assert.Contains(t, string(doc), `"year":null,"years":[],"securities":[]`)
}

func Test_NewACB_rounds_the_shares_a_consolidation_leaves_to_millionths(t *testing.T) {
	a := report.ACB{AsOf: acbAsOf, Securities: []report.ACBSecurity{{
		Security: store.Security{ID: "sec-1", Name: "XEQT"}, Shares: big.NewRat(2, 3), ACB: 1_000,
		Events: []report.ACBEvent{{Shares: new(big.Rat), Held: big.NewRat(1, 3), ACB: 1_000}},
	}}}

	got := document.NewACB(a, nil)

	assert.Equal(t, "0.666667", got.Securities[0].Shares)
	assert.Equal(t, "0.333333", got.Securities[0].Events[0].SharesHeld)
	assert.Equal(t, new("15.0000"), got.Securities[0].ACBPerShare)
}

func Test_NewACB_counts_every_unknown_cost_sale_of_a_year(t *testing.T) {
	a := report.ACB{AsOf: acbAsOf, Years: []report.ACBYear{{Year: 2024, Sales: []report.ACBSale{
		{Date: acbAsOf, Shares: big.NewRat(1, 1), UnknownCost: true},
		{Date: acbAsOf, Shares: big.NewRat(1, 1)},
		{Date: acbAsOf, Shares: big.NewRat(1, 1), UnknownCost: true},
	}}}}
	var got document.ACB

	require.NoError(t, json.Unmarshal(acbJSON(t, a, nil), &got))

	require.Len(t, got.Years, 1)
	assert.Equal(t, 2, got.Years[0].UnknownCostSales)
}
