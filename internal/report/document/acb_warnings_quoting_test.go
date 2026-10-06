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
)

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
