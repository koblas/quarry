package money_test

import (
	"math"
	"testing"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/stretchr/testify/assert"
)

const (
	rate125 = money.Rate(1_250_000)
	rate16  = money.Rate(1_600_000)
)

func Test_convert_returns_the_amount_unchanged_for_the_same_currency_even_with_no_rate(t *testing.T) {
	cases := []struct {
		name string
		cur  money.Currency
	}{
		{name: "CAD to CAD", cur: money.CAD},
		{name: "USD to USD", cur: money.USD},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := money.Convert(-1234, c.cur, c.cur, 0)

			assert.True(t, ok)
			assert.Equal(t, int64(-1234), got)
		})
	}
}

func Test_convert_returns_the_amount_unchanged_to_native_even_with_no_rate(t *testing.T) {
	got, ok := money.Convert(777, money.USD, money.Native, 0)

	assert.True(t, ok)
	assert.Equal(t, int64(777), got)
}

func Test_convert_refuses_native_as_the_source(t *testing.T) {
	_, ok := money.Convert(100, money.Native, money.CAD, rate125)

	assert.False(t, ok)
}

func Test_convert_refuses_an_unknown_target_currency(t *testing.T) {
	_, ok := money.Convert(100, money.CAD, money.Currency(99), rate125)

	assert.False(t, ok)
}

func Test_convert_refuses_a_cross_currency_conversion_without_a_positive_rate(t *testing.T) {
	cases := []struct {
		name string
		rate money.Rate
	}{
		{name: "zero rate", rate: 0},
		{name: "negative rate", rate: -1_250_000},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, ok := money.Convert(100, money.USD, money.CAD, c.rate)

			assert.False(t, ok)
		})
	}
}

func Test_convert_rounds_half_a_cent_away_from_zero(t *testing.T) {
	cases := []struct {
		name  string
		cents int64
		from  money.Currency
		to    money.Currency
		rate  money.Rate
		want  int64
	}{
		{name: "USD to CAD half cent up", cents: 10, from: money.USD, to: money.CAD, rate: rate125, want: 13},
		{name: "USD to CAD negative half cent down", cents: -10, from: money.USD, to: money.CAD, rate: rate125, want: -13},
		{name: "USD to CAD just below half a cent", cents: 10, from: money.USD, to: money.CAD, rate: 1_249_999, want: 12},
		{name: "USD to CAD negative just below half a cent", cents: -10, from: money.USD, to: money.CAD, rate: 1_249_999, want: -12},
		{name: "CAD to USD half cent up", cents: 20, from: money.CAD, to: money.USD, rate: rate16, want: 13},
		{name: "CAD to USD negative half cent down", cents: -20, from: money.CAD, to: money.USD, rate: rate16, want: -13},
		{name: "CAD to USD just below half a cent", cents: 20, from: money.CAD, to: money.USD, rate: 1_600_001, want: 12},
		{name: "CAD to USD negative just below half a cent", cents: -20, from: money.CAD, to: money.USD, rate: 1_600_001, want: -12},
		{name: "zero converts to zero", cents: 0, from: money.CAD, to: money.USD, rate: rate16, want: 0},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := money.Convert(c.cents, c.from, c.to, c.rate)

			assert.True(t, ok)
			assert.Equal(t, c.want, got)
		})
	}
}

func Test_convert_handles_the_largest_stored_amount_without_overflow(t *testing.T) {
	const largest = int64(9_999_999_999_999_999) // DECIMAL(18,2) in cents

	got, ok := money.Convert(largest, money.USD, money.CAD, money.Rate(500_000))

	assert.True(t, ok)
	assert.Equal(t, int64(5_000_000_000_000_000), got)
}

func Test_convert_refuses_a_result_that_does_not_fit_an_int64(t *testing.T) {
	_, ok := money.Convert(math.MaxInt64, money.USD, money.CAD, money.Rate(9_999_999_999))

	assert.False(t, ok)
}
