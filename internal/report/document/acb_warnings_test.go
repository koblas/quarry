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

func Test_ACBWarnings_is_empty_when_no_shares_were_removed_and_no_loss_is_marked(t *testing.T) {
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

func Test_ACBWarnings_lists_adjustment_lines_then_possible_superficial_losses_then_removals_then_returns_of_capital_above_the_acb(t *testing.T) {
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

	require.Len(t, warnings, 6)
	assert.Contains(t, warnings[0], "acb.adjustment item 1 names")
	assert.Contains(t, warnings[1], "acb.adjustment item 2 is for")
	assert.Contains(t, warnings[2], "1 possible superficial loss in 2025:")
	assert.Contains(t, warnings[3], `"Alpha": 2 shares left`)
	assert.Contains(t, warnings[4], `"Alpha": return of capital on`)
	assert.Contains(t, warnings[5], `"Beta": return of capital on`)
}
