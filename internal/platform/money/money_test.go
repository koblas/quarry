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

func Test_ParseCurrency_reads_each_currency_in_any_letter_case(t *testing.T) {
	cases := []struct {
		text string
		want money.Currency
	}{
		{text: "CAD", want: money.CAD},
		{text: "cad", want: money.CAD},
		{text: "Cad", want: money.CAD},
		{text: "USD", want: money.USD},
		{text: "usd", want: money.USD},
		{text: "native", want: money.Native},
		{text: "NATIVE", want: money.Native},
	}

	for _, c := range cases {
		t.Run(c.text, func(t *testing.T) {
			got, ok := money.ParseCurrency(c.text)

			assert.True(t, ok)
			assert.Equal(t, c.want, got)
		})
	}
}

func Test_ParseCurrency_refuses_anything_else_without_trimming(t *testing.T) {
	cases := []struct {
		name string
		text string
	}{
		{name: "empty", text: ""},
		{name: "another currency", text: "EUR"},
		{name: "leading space", text: " CAD"},
		{name: "trailing space", text: "CAD "},
		{name: "truncated native", text: "nativ"},
		{name: "long s folds to nothing", text: "UſD"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, ok := money.ParseCurrency(c.text)

			assert.False(t, ok)
		})
	}
}

func Test_Currency_String_is_the_canonical_spelling_ParseCurrency_reads_back(t *testing.T) {
	cases := []struct {
		cur  money.Currency
		want string
	}{
		{cur: money.CAD, want: "CAD"},
		{cur: money.USD, want: "USD"},
		{cur: money.Native, want: "native"},
		{cur: money.Currency(99), want: "native"},
	}

	for _, c := range cases {
		t.Run(c.want, func(t *testing.T) {
			assert.Equal(t, c.want, c.cur.String())
		})
	}
}

func Test_NativeOf_is_the_other_of_cad_and_usd(t *testing.T) {
	cases := []struct {
		name string
		cur  money.Currency
		want money.Currency
	}{
		{name: "a CAD report leaves USD unconverted", cur: money.CAD, want: money.USD},
		{name: "a USD report leaves CAD unconverted", cur: money.USD, want: money.CAD},
		{name: "native reads as CAD", cur: money.Native, want: money.CAD},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, money.NativeOf(c.cur))
		})
	}
}

func Test_money_ParseCents_is_exact_where_float_is_not(t *testing.T) {
	cases := []struct {
		name string
		text string
		want int64
	}{
		{name: "whole number", text: "12", want: 1200},
		{name: "one decimal", text: "12.3", want: 1230},
		{name: "two decimals", text: "12.34", want: 1234},
		{name: "zero", text: "0", want: 0},
		{name: "zero with decimals", text: "0.00", want: 0},
		{name: "1.15 is 114 through a float", text: "1.15", want: 115},
		{name: "0.29 is 28 through a float", text: "0.29", want: 29},
		{name: "the largest amount an int64 of cents holds", text: "92233720368547758.07", want: math.MaxInt64},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := money.ParseCents(c.text)

			assert.True(t, ok)
			assert.Equal(t, c.want, got)
		})
	}
}

func Test_money_ParseCents_reports_false_for_text_that_is_not_an_unsigned_amount_of_at_most_two_decimals(t *testing.T) {
	cases := []struct {
		name string
		text string
	}{
		{name: "three decimals", text: "12.345"},
		{name: "three decimals ending in zero", text: "12.340"},
		{name: "minus sign", text: "-5"},
		{name: "plus sign", text: "+5"},
		{name: "no whole part", text: ".5"},
		{name: "point with no decimals", text: "12."},
		{name: "underscore", text: "1_0"},
		{name: "exponent", text: "1e2"},
		{name: "hex", text: "0x10"},
		{name: "empty", text: ""},
		{name: "quoted", text: `"12.34"`},
		{name: "two points", text: "1.2.3"},
		{name: "one cent past an int64", text: "92233720368547758.08"},
		{name: "far past an int64", text: "99999999999999999999"},
		{name: "non-ASCII digit", text: "١٢"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, ok := money.ParseCents(c.text)

			assert.False(t, ok)
		})
	}
}
