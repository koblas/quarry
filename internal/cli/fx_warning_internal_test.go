// White-box: unconvertedWarnings and its counted noun are unexported; the line's exact wording is the rule.
package cli

import (
	"testing"
	"time"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
)

func Test_unconvertedWarnings_words_the_before_the_first_rate_line_for_each_counted_noun(t *testing.T) {
	first := time.Date(1990, time.January, 2, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		name     string
		currency money.Currency
		noun     countedNoun
		count    int
		want     string
	}{
		{
			name: "transactions keep the spend and cashflow wording", currency: money.CAD, noun: transactionsNoun, count: 3,
			want: "3 transactions dated before 1990-01-02, the first exchange rate in the store, are listed in USD, not converted to CAD",
		},
		{
			name: "one transaction", currency: money.CAD, noun: transactionsNoun, count: 1,
			want: "1 transaction dated before 1990-01-02, the first exchange rate in the store, is listed in USD, not converted to CAD",
		},
		{
			name: "one series", currency: money.CAD, noun: seriesNoun, count: 1,
			want: "1 series with a charge dated before 1990-01-02, the first exchange rate in the store, is listed in USD, not converted to CAD",
		},
		{
			name: "two series in USD", currency: money.USD, noun: seriesNoun, count: 2,
			want: "2 series with a charge dated before 1990-01-02, the first exchange rate in the store, are listed in CAD, not converted to USD",
		},
		{
			name: "a thousand series are grouped", currency: money.CAD, noun: seriesNoun, count: 1234,
			want: "1,234 series with a charge dated before 1990-01-02, the first exchange rate in the store, are listed in USD, not converted to CAD",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := unconvertedWarnings(c.currency, store.Unconverted{Transactions: c.count, FirstRate: first}, c.noun)

			assert.Equal(t, []string{c.want}, got)
		})
	}
}

func Test_unconvertedWarnings_says_the_store_has_no_rates_without_naming_what_it_counts(t *testing.T) {
	cases := []struct {
		name string
		noun countedNoun
	}{
		{name: "transactions", noun: transactionsNoun},
		{name: "series", noun: seriesNoun},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := unconvertedWarnings(money.CAD, store.Unconverted{Transactions: 4}, c.noun)

			assert.Equal(t, []string{noRatesWarning}, got)
		})
	}
}

func Test_unconvertedWarnings_says_nothing_when_nothing_was_left_unconverted(t *testing.T) {
	first := time.Date(1990, time.January, 2, 0, 0, 0, 0, time.UTC)

	assert.Empty(t, unconvertedWarnings(money.CAD, store.Unconverted{FirstRate: first}, seriesNoun))
	assert.Empty(t, unconvertedWarnings(money.CAD, store.Unconverted{}, seriesNoun))
}
