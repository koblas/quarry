package document_test

import (
	"math/big"
	"slices"
	"strings"
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

// acbPooled is a with a security the pool traded, so a's own rows are not the only thing it shows.
func acbPooled(a report.ACB) report.ACB {
	a.Securities = []report.ACBSecurity{{Security: store.Security{ID: "sec-pool", Name: "Pool Fund"}, Shares: new(big.Rat)}}
	return a
}

func Test_ACBWarnings_names_each_security_held_only_in_registered_accounts_in_the_reports_order(t *testing.T) {
	a := report.ACB{RegisteredOnly: []store.Security{{ID: "sec-2", Name: "Maple Fund"}, {ID: "sec-9", Name: "Zeta Fund"}}}

	warnings := document.ACBWarnings(acbPooled(a), acbConfigShown, document.ACBAdviceCLI)

	assert.Equal(t, []string{
		`"Maple Fund" is held only in registered accounts, so it has no ACB`,
		`"Zeta Fund" is held only in registered accounts, so it has no ACB`,
	}, warnings)
}

func Test_ACBWarnings_names_one_possible_superficial_loss(t *testing.T) {
	a := report.ACB{Years: []report.ACBYear{acbMarkedYear(2025, 3, 1)}}

	warnings := document.ACBWarnings(acbPooled(a), acbConfigShown, document.ACBAdviceCLI)

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

	warnings := document.ACBWarnings(acbPooled(a), acbConfigShown, document.ACBAdviceCLI)

	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], "3 possible superficial losses in 2023, 2025: the same security was acquired")
}

func Test_ACBWarnings_names_each_removal_of_shares_with_no_sale(t *testing.T) {
	a := report.ACB{Securities: []report.ACBSecurity{{
		Security: store.Security{ID: "sec-41", Name: "iShares Core Equity ETF"},
		Events:   []report.ACBEvent{acbRemoval("Margin", time.Date(2025, time.March, 3, 0, 0, 0, 0, time.UTC), big.NewRat(4, 1))},
	}}}

	warnings := document.ACBWarnings(a, acbConfigShown, document.ACBAdviceCLI)

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

	warnings := document.ACBWarnings(a, acbConfigShown, document.ACBAdviceCLI)

	assert.Len(t, warnings, 3)
	assert.Contains(t, warnings[0], `"Alpha": 1,250.5 shares left "Margin" on 2025-03-03 without a sale;`)
	assert.Contains(t, warnings[1], `"Alpha": 0.25 shares left "Old margin" on 2025-04-04 without a sale;`)
	assert.Contains(t, warnings[2], `"Beta": 1,000 shares left "Margin" on 2025-03-03 without a sale;`)
}

func Test_ACBWarnings_is_empty_when_no_shares_were_removed_no_loss_is_marked_and_none_was_added_with_no_cost(t *testing.T) {
	a := acbDocumentFixture()
	a.Years[0].Sales[0].PossibleSuperficialLoss = false

	warnings := document.ACBWarnings(a, acbConfigShown, document.ACBAdviceCLI)

	assert.Empty(t, warnings)
}

func Test_ACBWarnings_writes_an_unknown_security_id_as_a_toml_string(t *testing.T) {
	a := report.ACB{AdjustmentIssues: []report.ACBAdjustmentIssue{
		{Kind: report.ACBAdjustmentUnknownSecurity, Item: 1, SecurityID: `sec"9`, Date: acbDay},
	}}

	warnings := document.ACBWarnings(a, acbConfigShown, document.ACBAdviceCLI)

	assert.Contains(t, warnings[0], `item 1 names "sec\"9", which`)
}

func Test_ACBWarnings_names_a_return_of_capital_above_the_acb(t *testing.T) {
	a := report.ACB{Securities: []report.ACBSecurity{{
		Security: store.Security{ID: "sec-1", Name: "Acme Corp"},
		Events: []report.ACBEvent{{
			Date: time.Date(2025, time.December, 31, 0, 0, 0, 0, time.UTC), Action: report.ACBActionReturnOfCapital,
			Shares: new(big.Rat), Gain: 125_000, Realized: true,
		}},
	}}}

	warnings := document.ACBWarnings(a, acbConfigShown, document.ACBAdviceCLI)

	assert.Equal(t, []string{
		`"Acme Corp": return of capital on 2025-12-31 is 1,250.00 more than its ACB, so its ACB is 0.00 and 1,250.00 is a capital gain in 2025`,
	}, warnings)
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

	warnings := document.ACBWarnings(a, acbConfigShown, document.ACBAdviceCLI)

	require.Len(t, warnings, 7)
	assert.Contains(t, warnings[0], "acb.adjustment item 1 names")
	assert.Contains(t, warnings[1], "acb.adjustment item 2 is for")
	assert.Contains(t, warnings[2], "1 possible superficial loss in 2025:")
	assert.Contains(t, warnings[3], `"Alpha" has shares added with no cost,`)
	assert.Contains(t, warnings[4], `"Alpha": 2 shares left`)
	assert.Contains(t, warnings[5], `"Alpha": return of capital on`)
	assert.Contains(t, warnings[6], `"Beta": return of capital on`)
}

func acbOversoldSale(account string, date time.Time, short *big.Rat) report.ACBEvent {
	return report.ACBEvent{Date: date, Account: account, Action: store.ActionSell, Shares: big.NewRat(3, 1), Held: new(big.Rat).Neg(short), Oversold: short}
}

func Test_ACBWarnings_names_an_oversold_sale(t *testing.T) {
	cases := []struct {
		name   string
		advice document.ACBAdvice
	}{
		{name: "on the command line", advice: document.ACBAdviceCLI},
		{name: "on the MCP server", advice: document.ACBAdviceTool},
	}
	a := report.ACB{Securities: []report.ACBSecurity{{
		Security: store.Security{ID: "sec-1", Name: "Money Fund"},
		Events:   []report.ACBEvent{acbOversoldSale("CAD Brokerage", time.Date(2017, time.January, 12, 0, 0, 0, 0, time.UTC), big.NewRat(112284, 100))},
	}}}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			warnings := document.ACBWarnings(a, acbConfigShown, c.advice)

			assert.Equal(t, []string{
				`"Money Fund": the sale on 2017-01-12 in "CAD Brokerage" sold 1,122.84 more shares than the non-registered accounts held; ` +
					"quarry counts them at no cost, so the sale's gain is too high by what they cost, " +
					"and the next 1,122.84 shares acquired only bring the holding back to 0; correct the shares in Quicken if they are wrong",
			}, warnings)
		})
	}
}

func Test_ACBWarnings_names_an_oversold_removal_after_its_removal_line(t *testing.T) {
	removal := acbRemoval("Margin", acbDay, big.NewRat(12, 1))
	removal.Oversold = big.NewRat(5, 1)
	a := report.ACB{Securities: []report.ACBSecurity{{Security: store.Security{ID: "sec-1", Name: "Alpha"}, Events: []report.ACBEvent{removal}}}}

	warnings := document.ACBWarnings(a, acbConfigShown, document.ACBAdviceCLI)

	require.Len(t, warnings, 2)
	assert.Contains(t, warnings[0], `"Alpha": 12 shares left "Margin" on 2025-03-03 without a sale;`)
	assert.Equal(t, `"Alpha": 5 more shares left "Margin" on 2025-03-03 than the non-registered accounts held; `+
		"the next 5 shares acquired only bring the holding back to 0; correct the shares in Quicken if they are wrong", warnings[1])
}

func Test_ACBWarnings_names_each_oversold_disposition_in_walk_order(t *testing.T) {
	second := time.Date(2025, time.April, 4, 0, 0, 0, 0, time.UTC)
	a := report.ACB{Securities: []report.ACBSecurity{
		{
			Security: store.Security{ID: "sec-1", Name: "Alpha"},
			Events:   []report.ACBEvent{acbOversoldSale("Margin", acbDay, big.NewRat(2, 1)), acbOversoldSale("Old margin", second, big.NewRat(7, 1))},
		},
		{
			Security: store.Security{ID: "sec-2", Name: "Beta"},
			Events:   []report.ACBEvent{acbOversoldSale("Margin", acbDay, big.NewRat(1, 4)), acbOversoldSale("Margin", second, big.NewRat(9, 4))},
		},
	}}

	warnings := document.ACBWarnings(a, acbConfigShown, document.ACBAdviceCLI)

	require.Len(t, warnings, 4)
	assert.Contains(t, warnings[0], `"Alpha": the sale on 2025-03-03 in "Margin" sold 2 more shares`)
	assert.Contains(t, warnings[1], `"Alpha": the sale on 2025-04-04 in "Old margin" sold 7 more shares`)
	assert.Contains(t, warnings[2], `"Beta": the sale on 2025-03-03 in "Margin" sold 0.25 more shares`)
	assert.Contains(t, warnings[3], `"Beta": the sale on 2025-04-04 in "Margin" sold 2.25 more shares`)
}

func Test_ACBWarnings_names_an_oversold_sale_of_a_security_left_out_for_want_of_a_rate(t *testing.T) {
	security := acbNoRateSecurity("sec-1", "Alpha", "EUR", acbDay)
	security.Events = []report.ACBEvent{acbOversoldSale("Margin", acbDay, big.NewRat(2, 1))}

	warnings := document.ACBWarnings(report.ACB{Securities: []report.ACBSecurity{security}}, acbConfigShown, document.ACBAdviceCLI)

	require.Len(t, warnings, 2)
	assert.Contains(t, warnings[0], `"Alpha" has a trade on 2025-03-03 in a currency quarry cannot convert`)
	assert.Contains(t, warnings[1], `"Alpha": the sale on 2025-03-03 in "Margin" sold 2 more shares`)
}

func Test_ACBWarnings_lists_oversold_lines_after_the_december_sales(t *testing.T) {
	a := report.ACB{
		Years: []report.ACBYear{{Year: 2025, Sales: []report.ACBSale{{Date: time.Date(2025, time.December, 28, 0, 0, 0, 0, time.UTC)}}}},
		Securities: []report.ACBSecurity{{
			Security: store.Security{ID: "sec-1", Name: "Alpha"},
			Events:   []report.ACBEvent{acbOversoldSale("Margin", acbDay, big.NewRat(2, 1))},
		}},
	}

	warnings := document.ACBWarnings(a, acbConfigShown, document.ACBAdviceCLI)

	require.Len(t, warnings, 2)
	assert.Contains(t, warnings[0], "1 sale dated December 24–31, 2025")
	assert.Contains(t, warnings[1], `"Alpha": the sale on 2025-03-03`)
}

func Test_ACBWarnings_names_a_security_with_only_added_shares_with_no_cost(t *testing.T) {
	a := report.ACB{Securities: []report.ACBSecurity{acbNoCostSecurity("sec-1", "Acme Corp", acbNoCostEvent(store.ActionAddShares))}}

	warnings := document.ACBWarnings(a, acbConfigShown, document.ACBAdviceCLI)

	assert.Equal(t, []string{
		`"Acme Corp" has shares added with no cost, so its ACB is too low and its gains too high; ` +
			"quarry findings --type shares-without-cost lists them",
	}, warnings)
}

func Test_ACBWarnings_names_a_security_with_only_reinvested_dividends_with_no_cost(t *testing.T) {
	a := report.ACB{Securities: []report.ACBSecurity{acbNoCostSecurity("sec-1", "Acme Corp", acbNoCostEvent(store.ActionReinvestDividend))}}

	warnings := document.ACBWarnings(a, acbConfigShown, document.ACBAdviceCLI)

	assert.Equal(t, []string{
		`"Acme Corp" has reinvested dividends with no cost, so its ACB is too low and its gains too high; ` +
			"enter their cost in Quicken; quarry acb --security sec-1 lists them",
	}, warnings)
}

func Test_ACBWarnings_names_shares_added_and_dividends_reinvested_with_no_cost_in_one_line(t *testing.T) {
	a := report.ACB{Securities: []report.ACBSecurity{acbNoCostSecurity(
		"sec-1", "Acme Corp", acbNoCostEvent(store.ActionReinvestDividend), acbNoCostEvent(store.ActionAddShares),
	)}}

	warnings := document.ACBWarnings(a, acbConfigShown, document.ACBAdviceCLI)

	assert.Equal(t, []string{
		`"Acme Corp" has shares added and dividends reinvested with no cost, so its ACB is too low and its gains too high; ` +
			"quarry findings --type shares-without-cost lists the added shares; enter the reinvested dividends' cost in Quicken",
	}, warnings)
}

func Test_ACBWarnings_names_two_no_cost_adds_of_one_security_in_one_line(t *testing.T) {
	a := report.ACB{Securities: []report.ACBSecurity{acbNoCostSecurity(
		"sec-1", "Acme Corp", acbNoCostEvent(store.ActionAddShares), acbNoCostEvent(store.ActionAddShares),
	)}}

	warnings := document.ACBWarnings(a, acbConfigShown, document.ACBAdviceCLI)

	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], `"Acme Corp" has shares added with no cost,`)
}

func Test_ACBWarnings_gives_each_security_with_a_no_cost_acquisition_its_own_line_in_security_order(t *testing.T) {
	a := report.ACB{Securities: []report.ACBSecurity{
		acbNoCostSecurity("sec-2", "Alpha", acbNoCostEvent(store.ActionReinvestDividend)),
		acbNoCostSecurity("sec-9", "Costed", report.ACBEvent{Date: acbDay, Action: store.ActionAddShares, Shares: big.NewRat(5, 1)}),
		acbNoCostSecurity("sec-1", "Beta", acbNoCostEvent(store.ActionAddShares)),
	}}

	warnings := document.ACBWarnings(a, acbConfigShown, document.ACBAdviceCLI)

	require.Len(t, warnings, 2)
	assert.Contains(t, warnings[0], `"Alpha" has reinvested dividends with no cost,`)
	assert.Contains(t, warnings[1], `"Beta" has shares added with no cost,`)
}

func Test_ACBWarnings_names_a_sold_out_security_that_took_in_shares_with_no_cost(t *testing.T) {
	soldOut := acbNoCostSecurity("sec-1", "Acme Corp", acbNoCostEvent(store.ActionAddShares))
	soldOut.Shares, soldOut.Incomplete = new(big.Rat), false

	warnings := document.ACBWarnings(report.ACB{Securities: []report.ACBSecurity{soldOut}}, acbConfigShown, document.ACBAdviceCLI)

	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], `"Acme Corp" has shares added with no cost,`)
}

func Test_ACBWarnings_stays_silent_for_a_security_whose_only_marked_event_is_a_disposition(t *testing.T) {
	sale := report.ACBEvent{Date: acbDay, Action: store.ActionSell, Shares: big.NewRat(1, 1), UnknownCost: true}
	security := acbNoCostSecurity("sec-1", "Acme Corp", sale)
	security.Incomplete = true

	warnings := document.ACBWarnings(report.ACB{Securities: []report.ACBSecurity{security}}, acbConfigShown, document.ACBAdviceCLI)

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

	warnings := document.ACBWarnings(a, acbConfigShown, document.ACBAdviceCLI)

	assert.Equal(t, []string{
		`"Acme Corp" has a USD trade on 2025-03-03, before 2024-01-02, the first exchange rate in the store, ` +
			"so its ACB is incomplete and its gains are left out of the year totals",
	}, warnings)
}

func Test_ACBWarnings_names_a_usd_trade_when_the_store_has_no_rates(t *testing.T) {
	a := report.ACB{Securities: []report.ACBSecurity{acbNoRateSecurity("sec-1", "Acme Corp", "USD", acbDay)}}

	warnings := document.ACBWarnings(a, acbConfigShown, document.ACBAdviceCLI)

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

			warnings := document.ACBWarnings(a, acbConfigShown, document.ACBAdviceCLI)

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

	warnings := document.ACBWarnings(a, acbConfigShown, document.ACBAdviceCLI)

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

	warnings := document.ACBWarnings(a, acbConfigShown, document.ACBAdviceCLI)

	require.Len(t, warnings, 3)
	assert.Contains(t, warnings[0], `"Alpha": 2 shares left`)
	assert.Contains(t, warnings[1], `"Alpha" has a USD trade on`)
	assert.Contains(t, warnings[2], `"Alpha": return of capital on`)
}

func Test_ACBWarnings_words_each_no_cost_line_for_the_advice_it_is_given(t *testing.T) {
	const cost = "its ACB is too low and its gains too high; "
	added := []report.ACBEvent{acbNoCostEvent(store.ActionAddShares)}
	reinvested := []report.ACBEvent{acbNoCostEvent(store.ActionReinvestDividend)}
	both := []report.ACBEvent{acbNoCostEvent(store.ActionReinvestDividend), acbNoCostEvent(store.ActionAddShares)}
	cases := []struct {
		name   string
		events []report.ACBEvent
		advice document.ACBAdvice
		want   string
	}{
		{
			name: "added shares on the command line", events: added, advice: document.ACBAdviceCLI,
			want: `"Acme Corp" has shares added with no cost, so ` + cost + "quarry findings --type shares-without-cost lists them",
		},
		{
			name: "added shares on the tool", events: added, advice: document.ACBAdviceTool,
			want: `"Acme Corp" has shares added with no cost, so ` + cost + "data_quality with type shares-without-cost lists them",
		},
		{
			name: "reinvested dividends on the command line", events: reinvested, advice: document.ACBAdviceCLI,
			want: `"Acme Corp" has reinvested dividends with no cost, so ` + cost + "enter their cost in Quicken; quarry acb --security sec-1 lists them",
		},
		{
			name: "reinvested dividends on the tool", events: reinvested, advice: document.ACBAdviceTool,
			want: `"Acme Corp" has reinvested dividends with no cost, so ` + cost + "enter their cost in Quicken; acb with security sec-1 lists them",
		},
		{
			name: "both on the command line", events: both, advice: document.ACBAdviceCLI,
			want: `"Acme Corp" has shares added and dividends reinvested with no cost, so ` + cost +
				"quarry findings --type shares-without-cost lists the added shares; enter the reinvested dividends' cost in Quicken",
		},
		{
			name: "both on the tool", events: both, advice: document.ACBAdviceTool,
			want: `"Acme Corp" has shares added and dividends reinvested with no cost, so ` + cost +
				"data_quality with type shares-without-cost lists the added shares; enter the reinvested dividends' cost in Quicken",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := report.ACB{Securities: []report.ACBSecurity{acbNoCostSecurity("sec-1", "Acme Corp", c.events...)}}

			assert.Equal(t, []string{c.want}, document.ACBWarnings(a, acbConfigShown, c.advice))
		})
	}
}

func Test_ACBWarnings_words_every_line_but_the_no_cost_ones_the_same_for_every_advice(t *testing.T) {
	a := report.ACB{
		AdjustmentIssues: []report.ACBAdjustmentIssue{{Item: 1, Kind: report.ACBAdjustmentUnknownSecurity, SecurityID: "sec-9"}},
		Securities: []report.ACBSecurity{
			acbNoCostSecurity("sec-2", "Beta", acbNoCostEvent(store.ActionAddShares)),
			acbNoRateSecurity("sec-1", "Alpha", "USD", acbDay),
		},
	}
	const (
		config = `~/Library/Application Support/quarry/config.toml: acb.adjustment item 1 names "sec-9", ` +
			"which is not a security in quarry's store; quarry skips it"
		noCost = `"Beta" has shares added with no cost, so its ACB is too low and its gains too high; `
		noRate = `"Alpha" has a USD trade on 2025-03-03, and the store has no exchange rates, so its ACB is incomplete ` +
			"and its gains are left out of the year totals; run quarry sync to fetch rates"
	)

	assert.Equal(t, []string{config, noCost + "quarry findings --type shares-without-cost lists them", noRate},
		document.ACBWarnings(a, acbConfigShown, document.ACBAdviceCLI))
	assert.Equal(t, []string{config, noCost + "data_quality with type shares-without-cost lists them", noRate},
		document.ACBWarnings(a, acbConfigShown, document.ACBAdviceTool))
}

func Test_ACBWarnings_says_what_nothing_to_show_means_the_same_for_every_advice(t *testing.T) {
	const nothingToShow = "no non-registered account has bought or sold a security; quarry acb has nothing to show"

	assert.Equal(t, []string{nothingToShow}, document.ACBWarnings(report.ACB{}, acbConfigShown, document.ACBAdviceCLI))
	assert.Equal(t, []string{nothingToShow}, document.ACBWarnings(report.ACB{}, acbConfigShown, document.ACBAdviceTool))
}

// acbSoldIn is a pool with one security, asked about the tax year cutTo (0 for the whole report), with a sale in
// each of the years.
func acbSoldIn(cutTo int, years ...int) report.ACB {
	a := acbPooled(report.ACB{Year: cutTo})
	for _, year := range years {
		a.Years = append(a.Years, acbSalesOn(year, time.Date(year, time.June, 1, 0, 0, 0, 0, time.UTC)))
	}
	return a
}

func Test_ACBWarnings_says_why_it_has_nothing_to_show(t *testing.T) {
	const nothingToShow = "no non-registered account has bought or sold a security; quarry acb has nothing to show"
	cases := []struct {
		name string
		a    report.ACB
		want string
	}{
		{name: "no non-registered account traded, whole report", a: report.ACB{}, want: nothingToShow},
		{name: "no non-registered account traded, for a year", a: report.ACB{Year: 2025}, want: nothingToShow},
		{
			name: "the year is empty and so is every other",
			a:    acbSoldIn(2025),
			want: "no sales in 2025 in non-registered accounts, nor in any other year",
		},
		{
			name: "the year is empty and the sales span several",
			a:    acbSoldIn(2023, 2022, 2024, 2026),
			want: "no sales in 2023 in non-registered accounts; the sales are in 2022–2026",
		},
		{
			name: "the year is empty and every sale is in one",
			a:    acbSoldIn(2025, 2022),
			want: "no sales in 2025 in non-registered accounts; the sales are in 2022",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, []string{c.want}, document.ACBWarnings(c.a, acbConfigShown, document.ACBAdviceCLI))
		})
	}
}

func Test_ACBWarnings_leaves_out_of_the_span_a_year_with_only_a_return_of_capital_gain(t *testing.T) {
	a := acbSoldIn(2023, 2022, 2024)
	a.Years = append([]report.ACBYear{{Year: 2020, ReturnOfCapitalGain: 125_000}}, a.Years...)
	a.Years = append(a.Years, report.ACBYear{Year: 2026, ReturnOfCapitalGain: 125_000})

	warnings := document.ACBWarnings(a, acbConfigShown, document.ACBAdviceCLI)

	assert.Equal(t, []string{"no sales in 2023 in non-registered accounts; the sales are in 2022–2024"}, warnings)
}

func Test_ACBWarnings_is_silent_for_a_year_with_only_a_return_of_capital_gain(t *testing.T) {
	a := acbPooled(report.ACB{Year: 2025, Years: []report.ACBYear{{Year: 2025, ReturnOfCapitalGain: 125_000}}})

	warnings := document.ACBWarnings(a, acbConfigShown, document.ACBAdviceCLI)

	assert.Empty(t, warnings)
}

func Test_ACBWarnings_is_silent_when_there_is_something_to_show(t *testing.T) {
	cases := []struct {
		name string
		a    report.ACB
	}{
		{name: "the year has a sale", a: acbSoldIn(2025, 2025)},
		{name: "the whole report is a pool that bought and never sold", a: acbSoldIn(0)},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Empty(t, document.ACBWarnings(c.a, acbConfigShown, document.ACBAdviceCLI))
		})
	}
}

func Test_ACBWarnings_quote_every_quicken_name_so_a_quote_or_newline_keeps_the_line_whole(t *testing.T) {
	const (
		name   = "Say \"hi\"\nnext"
		quoted = `"Say \"hi\"\nnext"`
	)
	noCost := func(actions ...string) report.ACB {
		events := make([]report.ACBEvent, 0, len(actions))
		for _, action := range actions {
			events = append(events, acbNoCostEvent(action))
		}
		return report.ACB{Securities: []report.ACBSecurity{acbNoCostSecurity("sec-1", name, events...)}}
	}
	removal := report.ACB{Securities: []report.ACBSecurity{
		acbNoCostSecurity("sec-1", name, acbRemoval(name, acbDay, big.NewRat(2, 1))),
	}}
	oversoldRemoval := acbRemoval(name, acbDay, big.NewRat(3, 1))
	oversoldRemoval.Oversold = big.NewRat(3, 1)
	cases := []struct {
		name string
		acb  report.ACB
		want string
	}{
		{name: "adjustment not held", want: " is for " + quoted + ", which no non-registered account holds on 2025-03-03", acb: report.ACB{
			AdjustmentIssues: []report.ACBAdjustmentIssue{{Kind: report.ACBAdjustmentNotHeld, Item: 1, SecurityID: "sec-1", Security: name, Date: acbDay}},
		}},
		{name: "registered only", want: quoted + " is held only in registered accounts", acb: report.ACB{
			RegisteredOnly: []store.Security{{ID: "sec-1", Name: name}},
		}},
		{name: "shares added with no cost", want: quoted + " has shares added with no cost", acb: noCost(store.ActionAddShares)},
		{name: "dividends reinvested with no cost", want: quoted + " has reinvested dividends with no cost", acb: noCost(store.ActionReinvestDividend)},
		{
			name: "both with no cost", want: quoted + " has shares added and dividends reinvested",
			acb: noCost(store.ActionAddShares, store.ActionReinvestDividend),
		},
		{
			name: "shares removed", want: quoted + ": 2 shares left " + quoted + " on 2025-03-03 without a sale",
			acb: removal,
		},
		{name: "usd trade before the first rate", want: quoted + " has a USD trade on 2025-03-03, before 2024-01-02", acb: report.ACB{
			FirstRate:  time.Date(2024, time.January, 2, 0, 0, 0, 0, time.UTC),
			Securities: []report.ACBSecurity{acbNoRateSecurity("sec-1", name, "USD", acbDay)},
		}},
		{name: "usd trade with no rates", want: quoted + " has a USD trade on 2025-03-03, and the store has no exchange rates", acb: report.ACB{
			Securities: []report.ACBSecurity{acbNoRateSecurity("sec-1", name, "USD", acbDay)},
		}},
		{name: "trade in another currency", want: quoted + ` has a trade on 2025-03-03 in a currency quarry cannot convert to CAD ("EUR")`, acb: report.ACB{
			Securities: []report.ACBSecurity{acbNoRateSecurity("sec-1", name, "EUR", acbDay)},
		}},
		{
			name: "shared ticker", want: quoted + ` is 2 securities in Quicken (` + quoted + `, "Other")`,
			acb: report.ACB{Securities: []report.ACBSecurity{acbTickered("sec-1", name, name), acbTickered("sec-2", "Other", name)}},
		},
		{name: "return of capital above the ACB", want: quoted + ": return of capital on 2025-03-03 is", acb: report.ACB{
			Securities: []report.ACBSecurity{{Security: store.Security{ID: "sec-1", Name: name}, Shares: big.NewRat(5, 1), Events: []report.ACBEvent{
				{Date: acbDay, Action: report.ACBActionReturnOfCapital, Shares: new(big.Rat), Gain: 100, Realized: true},
			}}},
		}},
		{name: "sale that oversold", want: quoted + ": the sale on 2025-03-03 in " + quoted + " sold 3 more shares", acb: report.ACB{
			Securities: []report.ACBSecurity{{Security: store.Security{ID: "sec-1", Name: name}, Events: []report.ACBEvent{
				acbOversoldSale(name, acbDay, big.NewRat(3, 1)),
			}}},
		}},
		{name: "removal that oversold", want: quoted + ": 3 more shares left " + quoted + " on 2025-03-03", acb: report.ACB{
			Securities: []report.ACBSecurity{{Security: store.Security{ID: "sec-1", Name: name}, Events: []report.ACBEvent{oversoldRemoval}}},
		}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := c.acb
			if len(a.Securities) == 0 {
				a = acbPooled(a)
			}

			warnings := document.ACBWarnings(a, acbConfigShown, document.ACBAdviceCLI)

			assert.True(t, slices.ContainsFunc(warnings, func(w string) bool { return strings.Contains(w, c.want) }), "no warning contains %q in %q", c.want, warnings)
			for _, warning := range warnings {
				assert.NotContains(t, warning, "\n")
			}
		})
	}
}

func acbTickered(id, name, ticker string) report.ACBSecurity {
	return report.ACBSecurity{Security: store.Security{ID: id, Name: name, Ticker: &ticker}, Shares: big.NewRat(5, 1)}
}

func Test_ACBWarnings_names_two_securities_that_share_a_ticker(t *testing.T) {
	a := report.ACB{Securities: []report.ACBSecurity{acbTickered("sec-1", "Vanguard Total", "VTI"), acbTickered("sec-2", "Vanguard Total CAD", "VTI")}}

	warnings := document.ACBWarnings(a, acbConfigShown, document.ACBAdviceCLI)

	assert.Equal(t, []string{
		`"VTI" is 2 securities in Quicken ("Vanguard Total", "Vanguard Total CAD"); ` +
			"quarry keeps a separate ACB for each; if they are the same, merge them in Quicken",
	}, warnings)
}

func Test_ACBWarnings_counts_three_securities_that_share_a_ticker_and_names_each(t *testing.T) {
	a := report.ACB{Securities: []report.ACBSecurity{
		acbTickered("sec-1", "Alpha", "VTI"), acbTickered("sec-2", "Beta", "VTI"), acbTickered("sec-3", "Gamma", "VTI"),
	}}

	warnings := document.ACBWarnings(a, acbConfigShown, document.ACBAdviceCLI)

	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], `"VTI" is 3 securities in Quicken ("Alpha", "Beta", "Gamma");`)
}

func Test_ACBWarnings_gives_each_shared_ticker_its_own_line_in_first_member_order(t *testing.T) {
	a := report.ACB{Securities: []report.ACBSecurity{
		acbTickered("sec-1", "Alpha", "XEQT"), acbTickered("sec-2", "Beta", "VTI"),
		acbTickered("sec-3", "Gamma", "VTI"), acbTickered("sec-4", "Delta", "XEQT"),
	}}

	warnings := document.ACBWarnings(a, acbConfigShown, document.ACBAdviceCLI)

	require.Len(t, warnings, 2)
	assert.Contains(t, warnings[0], `"XEQT" is 2 securities in Quicken ("Alpha", "Delta");`)
	assert.Contains(t, warnings[1], `"VTI" is 2 securities in Quicken ("Beta", "Gamma");`)
}

// acbSalesOn is a year with one sale on each of the given dates.
func acbSalesOn(year int, dates ...time.Time) report.ACBYear {
	y := report.ACBYear{Year: year}
	for _, date := range dates {
		y.Sales = append(y.Sales, report.ACBSale{Date: date})
	}
	return y
}

func acbDec(year, day int) time.Time {
	return time.Date(year, time.December, day, 0, 0, 0, 0, time.UTC)
}

func Test_ACBWarnings_dates_a_december_sale_warning_by_the_24th_to_the_31st(t *testing.T) {
	cases := []struct {
		name string
		sale time.Time
		want int
	}{
		{name: "november 30 is outside", sale: time.Date(2025, time.November, 30, 0, 0, 0, 0, time.UTC), want: 0},
		{name: "december 23 is outside", sale: acbDec(2025, 23), want: 0},
		{name: "december 24 is inside", sale: acbDec(2025, 24), want: 1},
		{name: "december 31 is inside", sale: acbDec(2025, 31), want: 1},
		{name: "january 1 is outside", sale: time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC), want: 0},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := report.ACB{Years: []report.ACBYear{acbSalesOn(2025, c.sale)}}

			warnings := document.ACBWarnings(acbPooled(a), acbConfigShown, document.ACBAdviceCLI)

			assert.Len(t, warnings, c.want)
		})
	}
}

func Test_ACBWarnings_words_a_december_sale_warning_in_the_singular(t *testing.T) {
	a := report.ACB{Years: []report.ACBYear{acbSalesOn(2025, acbDec(2025, 28))}}

	warnings := document.ACBWarnings(acbPooled(a), acbConfigShown, document.ACBAdviceCLI)

	assert.Equal(t, []string{
		"1 sale dated December 24–31, 2025: a sale settles a day or two after its trade date and counts for tax in the year it settles; " +
			"check its date on your T5008",
	}, warnings)
}

func Test_ACBWarnings_counts_the_december_sales_of_one_year_in_one_plural_line(t *testing.T) {
	a := report.ACB{Years: []report.ACBYear{acbSalesOn(2025, acbDec(2025, 24), acbDec(2025, 31), acbDec(2025, 10))}}

	warnings := document.ACBWarnings(acbPooled(a), acbConfigShown, document.ACBAdviceCLI)

	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], "2 sales dated December 24–31, 2025:")
}

func Test_ACBWarnings_gives_each_year_with_a_december_sale_its_own_line_oldest_first(t *testing.T) {
	a := report.ACB{Years: []report.ACBYear{
		acbSalesOn(2023, acbDec(2023, 29)), acbSalesOn(2024, acbDec(2024, 2)), acbSalesOn(2025, acbDec(2025, 30)),
	}}

	warnings := document.ACBWarnings(acbPooled(a), acbConfigShown, document.ACBAdviceCLI)

	require.Len(t, warnings, 2)
	assert.Contains(t, warnings[0], "1 sale dated December 24–31, 2023:")
	assert.Contains(t, warnings[1], "1 sale dated December 24–31, 2025:")
}

func Test_ACBWarnings_orders_every_slot(t *testing.T) {
	noRate := acbNoRateSecurity("sec-1", "Alpha", "USD", acbDay)
	noRate.Security.Ticker = new("VTI")
	noRate.Events = []report.ACBEvent{
		{Date: acbDay, Action: report.ACBActionReturnOfCapital, Shares: new(big.Rat), Gain: 100, Realized: true},
		acbRemoval("Margin", acbDay, big.NewRat(2, 1)),
		acbNoCostEvent(store.ActionAddShares),
	}
	marked := acbMarkedYear(2025, 1, 1)
	marked.Sales[0].Date = acbDec(2025, 29)
	a := report.ACB{
		Year:             2024,
		FirstRate:        time.Date(2024, time.January, 2, 0, 0, 0, 0, time.UTC),
		Years:            []report.ACBYear{marked},
		AdjustmentIssues: []report.ACBAdjustmentIssue{{Kind: report.ACBAdjustmentUnknownSecurity, Item: 1, SecurityID: "sec-99", Date: acbDay}},
		Securities:       []report.ACBSecurity{noRate, acbTickered("sec-2", "Beta", "VTI")},
		RegisteredOnly:   []store.Security{{ID: "sec-3", Name: "Gamma"}},
	}

	warnings := document.ACBWarnings(a, acbConfigShown, document.ACBAdviceCLI)

	require.Len(t, warnings, 10)
	assert.Contains(t, warnings[0], "acb.adjustment item 1 names")
	assert.Equal(t, "no sales in 2024 in non-registered accounts; the sales are in 2025", warnings[1])
	assert.Equal(t, `"Gamma" is held only in registered accounts, so it has no ACB`, warnings[2])
	assert.Contains(t, warnings[3], "1 possible superficial loss in 2025:")
	assert.Contains(t, warnings[4], `"Alpha" has shares added with no cost,`)
	assert.Contains(t, warnings[5], `"Alpha": 2 shares left`)
	assert.Contains(t, warnings[6], `"Alpha" has a USD trade on`)
	assert.Contains(t, warnings[7], `"VTI" is 2 securities in Quicken`)
	assert.Contains(t, warnings[8], `"Alpha": return of capital on`)
	assert.Contains(t, warnings[9], "1 sale dated December 24–31, 2025:")
}

func Test_ACBWarnings_stays_silent_when_there_is_nothing_to_warn_about(t *testing.T) {
	cases := []struct {
		name string
		a    report.ACB
	}{
		{name: "names_no_registered_only_security_when_the_request_named_none", a: acbPooled(report.ACB{})},
		{name: "stays_silent_when_no_sale_is_marked", a: acbPooled(report.ACB{Years: []report.ACBYear{acbMarkedYear(2025, 3, 0)}})},
		{
			name: "stays_silent_for_a_return_of_capital_within_the_acb",
			a: report.ACB{Securities: []report.ACBSecurity{{
				Security: store.Security{ID: "sec-1", Name: "Acme Corp"},
				Events: []report.ACBEvent{{
					Date: acbDay, Action: report.ACBActionReturnOfCapital, Shares: new(big.Rat), CAD: 15_000,
				}},
			}}},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			warnings := document.ACBWarnings(c.a, acbConfigShown, document.ACBAdviceCLI)

			assert.Empty(t, warnings)
		})
	}
}

func Test_ACBWarnings_names_each_adjustment_the_config_gets_wrong(t *testing.T) {
	cases := []struct {
		name  string
		a     report.ACB
		shown string
		want  string
	}{
		{
			name: "names_an_adjustment_for_a_security_not_in_the_store",
			a: acbPooled(report.ACB{AdjustmentIssues: []report.ACBAdjustmentIssue{
				{Kind: report.ACBAdjustmentUnknownSecurity, Item: 2, SecurityID: "sec-99", Date: acbDay},
			}}),
			shown: acbConfigShown,
			want: `~/Library/Application Support/quarry/config.toml: acb.adjustment item 2 names "sec-99", ` +
				"which is not a security in quarry's store; quarry skips it",
		},
		{
			name: "names_an_adjustment_no_non_registered_account_holds",
			a: acbPooled(report.ACB{AdjustmentIssues: []report.ACBAdjustmentIssue{
				{Kind: report.ACBAdjustmentNotHeld, Item: 3, SecurityID: "sec-41", Security: "iShares Core Equity ETF", Date: acbDay},
			}}),
			shown: acbConfigShown,
			want: `~/Library/Application Support/quarry/config.toml: acb.adjustment item 3 is for "iShares Core Equity ETF", ` +
				"which no non-registered account holds on 2025-03-03; quarry skips it",
		},
		{
			name: "names_two_adjustments_for_one_security_on_one_day",
			a: acbPooled(report.ACB{AdjustmentIssues: []report.ACBAdjustmentIssue{
				{Kind: report.ACBAdjustmentRepeated, Item: 4, First: 1, SecurityID: "sec-41", Security: "iShares Core Equity ETF", Date: acbDay},
			}}),
			shown: "/home/me/config.toml",
			want:  `/home/me/config.toml: acb.adjustment items 1 and 4 are both for "sec-41" on 2025-03-03; quarry applies both`,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			warnings := document.ACBWarnings(c.a, c.shown, document.ACBAdviceCLI)

			assert.Equal(t, []string{c.want}, warnings)
		})
	}
}
