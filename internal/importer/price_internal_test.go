package importer

// White-box: parsePrice is unexported decision logic whose rounding and bound cases
// are too many to drive economically through Import.

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_parsePrice_returns_the_exact_millionths_of_a_readable_price(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		typ  string
		text string
		want int64
	}{
		{name: "a tie rounds down to an even digit", typ: "real", text: "12.3456785", want: 12_345_678},
		{name: "a tie rounds up to an even digit", typ: "real", text: "12.3456775", want: 12_345_678},
		{name: "a negative tie rounds to even", typ: "real", text: "-12.3456785", want: -12_345_678},
		{name: "past the tie rounds up", typ: "real", text: "12.34567851", want: 12_345_679},
		{name: "short of the tie rounds down", typ: "real", text: "12.3456784999", want: 12_345_678},
		{name: "six decimals are exact", typ: "real", text: "12.345678", want: 12_345_678},
		{name: "a small negative exponent", typ: "real", text: "1.0e-05", want: 10},
		{name: "a tie in exponent form rounds to even", typ: "real", text: "5.0e-07", want: 0},
		{name: "a positive exponent", typ: "real", text: "2.5e+01", want: 25_000_000},
		{name: "an integer", typ: "integer", text: "7", want: 7_000_000},
		{name: "zero", typ: "integer", text: "0", want: 0},
		{name: "the largest price in range", typ: "real", text: "999999999999.999999", want: 999_999_999_999_999_999},
		{name: "a value rounding down to the largest price", typ: "real", text: "999999999999.9999994", want: 999_999_999_999_999_999},
		{name: "the most negative price in range", typ: "real", text: "-999999999999.999999", want: -999_999_999_999_999_999},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got, fault := parsePrice(c.typ, c.text)

			assert.Equal(t, moneyOK, fault)
			assert.Equal(t, c.want, got)
		})
	}
}

func Test_parsePrice_refuses_a_price_whose_rounded_magnitude_reaches_the_bound(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		typ  string
		text string
	}{
		{name: "the bound itself", typ: "integer", text: "1000000000000"},
		{name: "a tie rounding up to the bound", typ: "real", text: "999999999999.9999995"},
		{name: "the negative bound", typ: "integer", text: "-1000000000000"},
		{name: "a positive exponent far beyond it", typ: "real", text: "1.0e+20"},
		{name: "infinity", typ: "real", text: "Inf"},
		{name: "negative infinity", typ: "real", text: "-Inf"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			_, fault := parsePrice(c.typ, c.text)

			assert.Equal(t, moneyTooLarge, fault)
		})
	}
}

func Test_parsePrice_refuses_a_price_that_is_not_a_number(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		typ  string
		text string
	}{
		{name: "text storage", typ: "text", text: "12.5"},
		{name: "blob storage", typ: "blob", text: "12.5"},
		{name: "no digits", typ: "real", text: "n/a"},
		{name: "empty text", typ: "real", text: ""},
		{name: "a fraction", typ: "real", text: "1/2"},
		{name: "hexadecimal", typ: "real", text: "0x10"},
		{name: "no integer part", typ: "real", text: ".5"},
		{name: "a dot with no digits", typ: "real", text: "1."},
		{name: "an exponent with no digits", typ: "real", text: "1e"},
		{name: "an exponent with only a sign", typ: "real", text: "1e-"},
		{name: "a doubled sign", typ: "real", text: "--1"},
		{name: "infinity stored as an integer", typ: "integer", text: "Inf"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			_, fault := parsePrice(c.typ, c.text)

			assert.Equal(t, moneyNotANumber, fault)
		})
	}
}
