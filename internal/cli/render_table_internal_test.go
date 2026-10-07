// White-box: windowCaption is the one caption chokepoint every report's table shares.
package cli

import (
	"testing"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/stretchr/testify/assert"
)

func Test_windowCaption_names_the_reporting_currency_only_for_cad_and_usd(t *testing.T) {
	cases := []struct {
		name     string
		currency money.Currency
		want     string
	}{
		{name: "CAD", currency: money.CAD, want: "Spending 2026-01-01 to 2026-09-29 in all accounts, amounts in CAD"},
		{name: "USD", currency: money.USD, want: "Spending 2026-01-01 to 2026-09-29 in all accounts, amounts in USD"},
		{name: "native adds nothing", currency: money.Native, want: "Spending 2026-01-01 to 2026-09-29 in all accounts"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, windowCaption("Spending", spendWindow(), nil, c.currency))
		})
	}
}

func Test_escapeCell_writes_newline_tab_and_carriage_return_as_backslash_forms(t *testing.T) {
	assert.Equal(t, `a\nb\tc\rd`, escapeCell("a\nb\tc\rd"))
	assert.Equal(t, "plain 'name' \\ é", escapeCell("plain 'name' \\ é"))
}
