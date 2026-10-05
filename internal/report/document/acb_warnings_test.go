package document_test

import (
	"math/big"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/report/document"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
)

func acbRemoval(account string, date time.Time, units *big.Rat) report.ACBEvent {
	return report.ACBEvent{Date: date, Account: account, Action: store.ActionRemoveShares, Shares: units, Held: new(big.Rat)}
}

func Test_ACBWarnings_names_each_removal_of_shares_with_no_sale(t *testing.T) {
	a := report.ACB{Securities: []report.ACBSecurity{{
		Security: store.Security{ID: "sec-41", Name: "iShares Core Equity ETF"},
		Events:   []report.ACBEvent{acbRemoval("Margin", time.Date(2025, time.March, 3, 0, 0, 0, 0, time.UTC), big.NewRat(4, 1))},
	}}}

	warnings := document.ACBWarnings(a)

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

	warnings := document.ACBWarnings(a)

	assert.Len(t, warnings, 3)
	assert.Contains(t, warnings[0], `"Alpha": 1,250.5 shares left "Margin" on 2025-03-03 without a sale;`)
	assert.Contains(t, warnings[1], `"Alpha": 0.25 shares left "Old margin" on 2025-04-04 without a sale;`)
	assert.Contains(t, warnings[2], `"Beta": 1,000 shares left "Margin" on 2025-03-03 without a sale;`)
}

func Test_ACBWarnings_is_empty_when_no_shares_were_removed(t *testing.T) {
	warnings := document.ACBWarnings(acbDocumentFixture())

	assert.Empty(t, warnings)
}
