package document_test

import (
	"testing"

	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/report/document"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
)

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
