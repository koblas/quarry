package document_test

import (
	"math/big"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/report/document"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const acbConfigShown = "~/Library/Application Support/quarry/config.toml"

var acbDay = time.Date(2025, time.March, 3, 0, 0, 0, 0, time.UTC)

func acbRemoval(account string, date time.Time, units *big.Rat) report.ACBEvent {
	return report.ACBEvent{Date: date, Account: account, Action: store.ActionRemoveShares, Shares: units, Held: new(big.Rat)}
}

// acbMarkedYear is year with sales sales, the first marked of them marked possible superficial losses.
func acbMarkedYear(year, sales, marked int) report.ACBYear {
	y := report.ACBYear{Year: year, Sales: make([]report.ACBSale, sales)}
	for i := range marked {
		y.Sales[i].PossibleSuperficialLoss = true
	}
	return y
}

// acbNoCostEvent is an acquisition of action whose shares have no recorded cost.
func acbNoCostEvent(action string) report.ACBEvent {
	return report.ACBEvent{Date: acbDay, Action: action, Shares: big.NewRat(5, 1), Held: big.NewRat(5, 1), UnknownCost: true}
}

func acbNoCostSecurity(id, name string, events ...report.ACBEvent) report.ACBSecurity {
	return report.ACBSecurity{Security: store.Security{ID: id, Name: name}, Shares: big.NewRat(5, 1), Events: events}
}

func Test_ACBWarnings_names_one_possible_superficial_loss(t *testing.T) {
	a := report.ACB{Years: []report.ACBYear{acbMarkedYear(2025, 3, 1)}}

	warnings := document.ACBWarnings(a, acbConfigShown)

	assert.Equal(t, []string{
		"1 possible superficial loss in 2025: the same security was acquired within 30 days before or after the sale, " +
			"in any account, and still held 30 days after; quarry does not deny or adjust these losses; review them with your accountant",
	}, warnings)
}

func Test_ACBWarnings_counts_the_possible_superficial_losses_of_every_year_in_one_line_by_year(t *testing.T) {
	a := report.ACB{Years: []report.ACBYear{
		acbMarkedYear(2023, 2, 2),
		acbMarkedYear(2024, 2, 0),
		acbMarkedYear(2025, 3, 1),
	}}

	warnings := document.ACBWarnings(a, acbConfigShown)

	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], "3 possible superficial losses in 2023, 2025: the same security was acquired")
}

func Test_ACBWarnings_stays_silent_when_no_sale_is_marked(t *testing.T) {
	a := report.ACB{Years: []report.ACBYear{acbMarkedYear(2025, 3, 0)}}

	warnings := document.ACBWarnings(a, acbConfigShown)

	assert.Empty(t, warnings)
}

func Test_ACBWarnings_names_each_removal_of_shares_with_no_sale(t *testing.T) {
	a := report.ACB{Securities: []report.ACBSecurity{{
		Security: store.Security{ID: "sec-41", Name: "iShares Core Equity ETF"},
		Events:   []report.ACBEvent{acbRemoval("Margin", time.Date(2025, time.March, 3, 0, 0, 0, 0, time.UTC), big.NewRat(4, 1))},
	}}}

	warnings := document.ACBWarnings(a, acbConfigShown)

	assert.Equal(t, []string{
		`"iShares Core Equity ETF": 4 shares left "Margin" on 2025-03-03 without a sale; ` +
			"quarry took their share of the ACB out and reports no gain; if they went to a registered account or to someone else, " +
			"that is a disposition at market value; check it with your accountant",
	}, warnings)
}

func Test_ACBWarnings_lists_removals_by_security_then_event_order_with_grouped_fractional_shares(t *testing.T) {
	first := time.Date(2025, time.March, 3, 0, 0, 0, 0, time.UTC)
	second := time.Date(2025, time.April, 4, 0, 0, 0, 0, time.UTC)
	a := report.ACB{Securities: []report.ACBSecurity{
		{
			Security: store.Security{ID: "sec-1", Name: "Alpha"},
			Events: []report.ACBEvent{
				acbRemoval("Margin", first, big.NewRat(2501, 2)),
				{Date: first, Account: "Margin", Action: store.ActionBuy, Shares: big.NewRat(1, 1)},
				acbRemoval("Old margin", second, big.NewRat(1, 4)),
			},
		},
		{
			Security: store.Security{ID: "sec-2", Name: "Beta"},
			Events:   []report.ACBEvent{acbRemoval("Margin", first, big.NewRat(1000, 1))},
		},
	}}

	warnings := document.ACBWarnings(a, acbConfigShown)

	assert.Len(t, warnings, 3)
	assert.Contains(t, warnings[0], `"Alpha": 1,250.5 shares left "Margin" on 2025-03-03 without a sale;`)
	assert.Contains(t, warnings[1], `"Alpha": 0.25 shares left "Old margin" on 2025-04-04 without a sale;`)
	assert.Contains(t, warnings[2], `"Beta": 1,000 shares left "Margin" on 2025-03-03 without a sale;`)
}

func Test_ACBWarnings_is_empty_when_no_shares_were_removed_no_loss_is_marked_and_none_was_added_with_no_cost(t *testing.T) {
	a := acbDocumentFixture()
	a.Years[0].Sales[0].PossibleSuperficialLoss = false

	warnings := document.ACBWarnings(a, acbConfigShown)

	assert.Empty(t, warnings)
}

func Test_ACBWarnings_names_an_adjustment_for_a_security_not_in_the_store(t *testing.T) {
	a := report.ACB{AdjustmentIssues: []report.ACBAdjustmentIssue{
		{Kind: report.ACBAdjustmentUnknownSecurity, Item: 2, SecurityID: "sec-99", Date: acbDay},
	}}

	warnings := document.ACBWarnings(a, acbConfigShown)

	assert.Equal(t, []string{
		`~/Library/Application Support/quarry/config.toml: acb.adjustment item 2 names "sec-99", ` +
			"which is not a security in quarry's store; quarry skips it",
	}, warnings)
}

func Test_ACBWarnings_writes_an_unknown_security_id_as_a_toml_string(t *testing.T) {
	a := report.ACB{AdjustmentIssues: []report.ACBAdjustmentIssue{
		{Kind: report.ACBAdjustmentUnknownSecurity, Item: 1, SecurityID: `sec"9`, Date: acbDay},
	}}

	warnings := document.ACBWarnings(a, acbConfigShown)

	assert.Contains(t, warnings[0], `item 1 names "sec\"9", which`)
}

func Test_ACBWarnings_names_an_adjustment_no_non_registered_account_holds(t *testing.T) {
	a := report.ACB{AdjustmentIssues: []report.ACBAdjustmentIssue{
		{Kind: report.ACBAdjustmentNotHeld, Item: 3, SecurityID: "sec-41", Security: "iShares Core Equity ETF", Date: acbDay},
	}}

	warnings := document.ACBWarnings(a, acbConfigShown)

	assert.Equal(t, []string{
		`~/Library/Application Support/quarry/config.toml: acb.adjustment item 3 is for "iShares Core Equity ETF", ` +
			"which no non-registered account holds on 2025-03-03; quarry skips it",
	}, warnings)
}

func Test_ACBWarnings_names_two_adjustments_for_one_security_on_one_day(t *testing.T) {
	a := report.ACB{AdjustmentIssues: []report.ACBAdjustmentIssue{
		{Kind: report.ACBAdjustmentRepeated, Item: 4, First: 1, SecurityID: "sec-41", Security: "iShares Core Equity ETF", Date: acbDay},
	}}

	warnings := document.ACBWarnings(a, "/home/me/config.toml")

	assert.Equal(t, []string{
		`/home/me/config.toml: acb.adjustment items 1 and 4 are both for "sec-41" on 2025-03-03; quarry applies both`,
	}, warnings)
}

func Test_ACBWarnings_names_a_return_of_capital_above_the_acb(t *testing.T) {
	a := report.ACB{Securities: []report.ACBSecurity{{
		Security: store.Security{ID: "sec-1", Name: "Acme Corp"},
		Events: []report.ACBEvent{{
			Date: time.Date(2025, time.December, 31, 0, 0, 0, 0, time.UTC), Action: report.ACBActionReturnOfCapital,
			Shares: new(big.Rat), Gain: 125_000, Realized: true,
		}},
	}}}

	warnings := document.ACBWarnings(a, acbConfigShown)

	assert.Equal(t, []string{
		`"Acme Corp": return of capital on 2025-12-31 is 1,250.00 more than its ACB, so its ACB is 0.00 and 1,250.00 is a capital gain in 2025`,
	}, warnings)
}

func Test_ACBWarnings_stays_silent_for_a_return_of_capital_within_the_acb(t *testing.T) {
	a := report.ACB{Securities: []report.ACBSecurity{{
		Security: store.Security{ID: "sec-1", Name: "Acme Corp"},
		Events: []report.ACBEvent{{
			Date: acbDay, Action: report.ACBActionReturnOfCapital, Shares: new(big.Rat), CAD: 15_000,
		}},
	}}}

	warnings := document.ACBWarnings(a, acbConfigShown)

	assert.Empty(t, warnings)
}

func Test_ACBWarnings_lists_adjustment_lines_then_superficial_losses_then_no_cost_then_removals_then_returns_of_capital(t *testing.T) {
	a := report.ACB{
		Years: []report.ACBYear{acbMarkedYear(2025, 1, 1)},
		AdjustmentIssues: []report.ACBAdjustmentIssue{
			{Kind: report.ACBAdjustmentUnknownSecurity, Item: 1, SecurityID: "sec-99", Date: acbDay},
			{Kind: report.ACBAdjustmentNotHeld, Item: 2, SecurityID: "sec-2", Security: "Beta", Date: acbDay},
		},
		Securities: []report.ACBSecurity{
			{
				Security: store.Security{ID: "sec-1", Name: "Alpha"},
				Events: []report.ACBEvent{
					{Date: acbDay, Action: report.ACBActionReturnOfCapital, Shares: new(big.Rat), Gain: 100, Realized: true},
					acbRemoval("Margin", acbDay, big.NewRat(2, 1)),
					acbNoCostEvent(store.ActionAddShares),
				},
			},
			{
				Security: store.Security{ID: "sec-2", Name: "Beta"},
				Events: []report.ACBEvent{
					{Date: acbDay, Action: report.ACBActionReturnOfCapital, Shares: new(big.Rat), Gain: 200, Realized: true},
				},
			},
		},
	}

	warnings := document.ACBWarnings(a, acbConfigShown)

	require.Len(t, warnings, 7)
	assert.Contains(t, warnings[0], "acb.adjustment item 1 names")
	assert.Contains(t, warnings[1], "acb.adjustment item 2 is for")
	assert.Contains(t, warnings[2], "1 possible superficial loss in 2025:")
	assert.Contains(t, warnings[3], `"Alpha" has shares added with no cost,`)
	assert.Contains(t, warnings[4], `"Alpha": 2 shares left`)
	assert.Contains(t, warnings[5], `"Alpha": return of capital on`)
	assert.Contains(t, warnings[6], `"Beta": return of capital on`)
}

func Test_ACBWarnings_names_a_security_with_only_added_shares_with_no_cost(t *testing.T) {
	a := report.ACB{Securities: []report.ACBSecurity{acbNoCostSecurity("sec-1", "Acme Corp", acbNoCostEvent(store.ActionAddShares))}}

	warnings := document.ACBWarnings(a, acbConfigShown)

	assert.Equal(t, []string{
		`"Acme Corp" has shares added with no cost, so its ACB is too low and its gains too high; ` +
			"quarry findings --type shares-without-cost lists them",
	}, warnings)
}

func Test_ACBWarnings_names_a_security_with_only_reinvested_dividends_with_no_cost(t *testing.T) {
	a := report.ACB{Securities: []report.ACBSecurity{acbNoCostSecurity("sec-1", "Acme Corp", acbNoCostEvent(store.ActionReinvestDividend))}}

	warnings := document.ACBWarnings(a, acbConfigShown)

	assert.Equal(t, []string{
		`"Acme Corp" has reinvested dividends with no cost, so its ACB is too low and its gains too high; ` +
			"enter their cost in Quicken; quarry acb --security sec-1 lists them",
	}, warnings)
}

func Test_ACBWarnings_names_shares_added_and_dividends_reinvested_with_no_cost_in_one_line(t *testing.T) {
	a := report.ACB{Securities: []report.ACBSecurity{acbNoCostSecurity(
		"sec-1", "Acme Corp", acbNoCostEvent(store.ActionReinvestDividend), acbNoCostEvent(store.ActionAddShares),
	)}}

	warnings := document.ACBWarnings(a, acbConfigShown)

	assert.Equal(t, []string{
		`"Acme Corp" has shares added and dividends reinvested with no cost, so its ACB is too low and its gains too high; ` +
			"quarry findings --type shares-without-cost lists the added shares; enter the reinvested dividends' cost in Quicken",
	}, warnings)
}

func Test_ACBWarnings_names_two_no_cost_adds_of_one_security_in_one_line(t *testing.T) {
	a := report.ACB{Securities: []report.ACBSecurity{acbNoCostSecurity(
		"sec-1", "Acme Corp", acbNoCostEvent(store.ActionAddShares), acbNoCostEvent(store.ActionAddShares),
	)}}

	warnings := document.ACBWarnings(a, acbConfigShown)

	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], `"Acme Corp" has shares added with no cost,`)
}

func Test_ACBWarnings_gives_each_security_with_a_no_cost_acquisition_its_own_line_in_security_order(t *testing.T) {
	a := report.ACB{Securities: []report.ACBSecurity{
		acbNoCostSecurity("sec-2", "Alpha", acbNoCostEvent(store.ActionReinvestDividend)),
		acbNoCostSecurity("sec-9", "Costed", report.ACBEvent{Date: acbDay, Action: store.ActionAddShares, Shares: big.NewRat(5, 1)}),
		acbNoCostSecurity("sec-1", "Beta", acbNoCostEvent(store.ActionAddShares)),
	}}

	warnings := document.ACBWarnings(a, acbConfigShown)

	require.Len(t, warnings, 2)
	assert.Contains(t, warnings[0], `"Alpha" has reinvested dividends with no cost,`)
	assert.Contains(t, warnings[1], `"Beta" has shares added with no cost,`)
}

func Test_ACBWarnings_names_a_sold_out_security_that_took_in_shares_with_no_cost(t *testing.T) {
	soldOut := acbNoCostSecurity("sec-1", "Acme Corp", acbNoCostEvent(store.ActionAddShares))
	soldOut.Shares, soldOut.Incomplete = new(big.Rat), false

	warnings := document.ACBWarnings(report.ACB{Securities: []report.ACBSecurity{soldOut}}, acbConfigShown)

	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], `"Acme Corp" has shares added with no cost,`)
}

func Test_ACBWarnings_stays_silent_for_a_security_whose_only_marked_event_is_a_disposition(t *testing.T) {
	sale := report.ACBEvent{Date: acbDay, Action: store.ActionSell, Shares: big.NewRat(1, 1), UnknownCost: true}
	security := acbNoCostSecurity("sec-1", "Acme Corp", sale)
	security.Incomplete = true

	warnings := document.ACBWarnings(report.ACB{Securities: []report.ACBSecurity{security}}, acbConfigShown)

	assert.Empty(t, warnings)
}

func acbNoRateSecurity(id, name, currency string, date time.Time) report.ACBSecurity {
	return report.ACBSecurity{
		Security: store.Security{ID: id, Name: name}, Shares: big.NewRat(5, 1), Incomplete: true,
		NoRate: &report.ACBNoRate{Date: date, Currency: currency},
	}
}

func Test_ACBWarnings_names_a_usd_trade_before_the_first_rate_in_the_store(t *testing.T) {
	a := report.ACB{
		FirstRate:  time.Date(2024, time.January, 2, 0, 0, 0, 0, time.UTC),
		Securities: []report.ACBSecurity{acbNoRateSecurity("sec-1", "Acme Corp", "USD", acbDay)},
	}

	warnings := document.ACBWarnings(a, acbConfigShown)

	assert.Equal(t, []string{
		`"Acme Corp" has a USD trade on 2025-03-03, before 2024-01-02, the first exchange rate in the store, ` +
			"so its ACB is incomplete and its gains are left out of the year totals",
	}, warnings)
}

func Test_ACBWarnings_names_a_usd_trade_when_the_store_has_no_rates(t *testing.T) {
	a := report.ACB{Securities: []report.ACBSecurity{acbNoRateSecurity("sec-1", "Acme Corp", "USD", acbDay)}}

	warnings := document.ACBWarnings(a, acbConfigShown)

	assert.Equal(t, []string{
		`"Acme Corp" has a USD trade on 2025-03-03, and the store has no exchange rates, ` +
			"so its ACB is incomplete and its gains are left out of the year totals; run quarry sync to fetch rates",
	}, warnings)
}

func Test_ACBWarnings_names_a_trade_in_a_currency_it_cannot_convert_by_its_code(t *testing.T) {
	cases := []struct {
		name     string
		currency string
		want     string
	}{
		{name: "a euro trade", currency: "EUR", want: `("EUR")`},
		{name: "a trade with no currency code", currency: "", want: `("")`},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := report.ACB{
				FirstRate:  time.Date(2024, time.January, 2, 0, 0, 0, 0, time.UTC),
				Securities: []report.ACBSecurity{acbNoRateSecurity("sec-1", "Acme Corp", c.currency, acbDay)},
			}

			warnings := document.ACBWarnings(a, acbConfigShown)

			assert.Equal(t, []string{
				`"Acme Corp" has a trade on 2025-03-03 in a currency quarry cannot convert to CAD ` + c.want + `, ` +
					"so its ACB is incomplete and its gains are left out of the year totals",
			}, warnings)
		})
	}
}

func Test_ACBWarnings_gives_each_no_rate_security_its_own_line_in_security_order(t *testing.T) {
	a := report.ACB{Securities: []report.ACBSecurity{
		acbNoRateSecurity("sec-1", "Alpha", "USD", acbDay),
		acbNoCostSecurity("sec-2", "Beta"),
		acbNoRateSecurity("sec-3", "Gamma", "USD", acbDay.AddDate(0, 0, 1)),
	}}

	warnings := document.ACBWarnings(a, acbConfigShown)

	require.Len(t, warnings, 2)
	assert.Contains(t, warnings[0], `"Alpha" has a USD trade on 2025-03-03,`)
	assert.Contains(t, warnings[1], `"Gamma" has a USD trade on 2025-03-04,`)
}

func Test_ACBWarnings_lists_a_no_rate_line_after_the_removals_and_before_the_returns_of_capital(t *testing.T) {
	noRate := acbNoRateSecurity("sec-1", "Alpha", "USD", acbDay)
	noRate.Events = []report.ACBEvent{
		{Date: acbDay, Action: report.ACBActionReturnOfCapital, Shares: new(big.Rat), Gain: 100, Realized: true},
		acbRemoval("Margin", acbDay, big.NewRat(2, 1)),
	}
	a := report.ACB{Securities: []report.ACBSecurity{noRate}}

	warnings := document.ACBWarnings(a, acbConfigShown)

	require.Len(t, warnings, 3)
	assert.Contains(t, warnings[0], `"Alpha": 2 shares left`)
	assert.Contains(t, warnings[1], `"Alpha" has a USD trade on`)
	assert.Contains(t, warnings[2], `"Alpha": return of capital on`)
}
