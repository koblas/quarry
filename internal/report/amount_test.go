package report_test

import (
	"testing"

	"github.com/koblas/quarry/internal/report"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_ParseSearchAmounts_reads_an_amount_as_cents(t *testing.T) {
	cases := []struct {
		name  string
		value string
		want  int64
	}{
		{name: "whole number", value: "12", want: 1200},
		{name: "one decimal is tenths", value: "12.5", want: 1250},
		{name: "two decimals", value: "12.50", want: 1250},
		{name: "zero", value: "0", want: 0},
		{name: "cents alone", value: "0.05", want: 5},
		{name: "sixteen integer digits is the largest", value: "9999999999999999.99", want: 999999999999999999},
		{name: "sixteen digits without decimals", value: "1000000000000000", want: 100000000000000000},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			asMin, err := report.ParseSearchAmounts(&c.value, nil)
			require.NoError(t, err)
			asMax, err := report.ParseSearchAmounts(nil, &c.value)
			require.NoError(t, err)

			assert.Equal(t, report.SearchAmounts{Min: &c.want}, asMin)
			assert.Equal(t, report.SearchAmounts{Max: &c.want}, asMax)
		})
	}
}

func Test_ParseSearchAmounts_refuses_a_value_outside_the_grammar(t *testing.T) {
	cases := []struct {
		name  string
		value string
	}{
		{name: "thousands separator", value: "1,234.56"},
		{name: "minus sign", value: "-12"},
		{name: "plus sign", value: "+12"},
		{name: "currency symbol", value: "$12"},
		{name: "trailing point", value: "12."},
		{name: "leading point", value: ".5"},
		{name: "three decimals", value: "12.345"},
		{name: "exponent", value: "1e2"},
		{name: "leading space", value: " 12"},
		{name: "trailing space", value: "12 "},
		{name: "trailing newline", value: "12\n"},
		{name: "empty", value: ""},
		{name: "non-ASCII digits", value: "١٢"},
		{name: "seventeen integer digits", value: "10000000000000000"},
		{name: "seventeen integer digits with decimals", value: "10000000000000000.00"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := report.ParseSearchAmounts(&c.value, nil)

			assert.Equal(t, report.AmountError{Kind: report.AmountNotAnAmount, Bound: "min", Value: c.value}, err)
		})
	}
}

func Test_ParseSearchAmounts_names_the_max_bound_when_the_max_is_refused(t *testing.T) {
	bad := "1,234.56"

	_, err := report.ParseSearchAmounts(nil, &bad)

	assert.Equal(t, report.AmountError{Kind: report.AmountNotAnAmount, Bound: "max", Value: bad}, err)
}

func Test_ParseSearchAmounts_words_each_refusal_for_the_command_line(t *testing.T) {
	cases := []struct {
		name     string
		min, max *string
		want     string
	}{
		{name: "min that is not an amount", min: new("-12"), want: `--min "-12" is not an amount; use digits with up to 2 decimals and no sign, such as 25 or 19.99`},
		{name: "max that is not an amount", max: new("1,234.56"), want: `--max "1,234.56" is not an amount; use digits with up to 2 decimals and no sign, such as 25 or 19.99`},
		{name: "min above max keeps the raw text", min: new("50"), max: new("20"), want: "--min 50 is more than --max 20"},
		{name: "min above max keeps an unnormalized value", min: new("50.0"), max: new("49.99"), want: "--min 50.0 is more than --max 49.99"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := report.ParseSearchAmounts(c.min, c.max)

			require.Error(t, err)
			assert.Equal(t, c.want, err.Error())
		})
	}
}

func Test_ParseSearchAmounts_reports_min_above_max_with_both_raw_values(t *testing.T) {
	_, err := report.ParseSearchAmounts(new("50"), new("20"))

	assert.Equal(t, report.AmountError{Kind: report.AmountMinAboveMax, Bound: "min", Value: "50", Other: "20"}, err)
}

func Test_ParseSearchAmounts_refuses_a_min_one_cent_above_the_max(t *testing.T) {
	_, err := report.ParseSearchAmounts(new("20.01"), new("20.00"))

	assert.Equal(t, report.AmountError{Kind: report.AmountMinAboveMax, Bound: "min", Value: "20.01", Other: "20.00"}, err)
}

func Test_ParseSearchAmounts_accepts_a_min_equal_to_the_max(t *testing.T) {
	got, err := report.ParseSearchAmounts(new("20"), new("20.00"))

	require.NoError(t, err)
	assert.Equal(t, report.SearchAmounts{Min: new(int64(2000)), Max: new(int64(2000))}, got)
}

func Test_ParseSearchAmounts_refuses_a_bad_min_before_a_bad_max(t *testing.T) {
	_, err := report.ParseSearchAmounts(new("-1"), new("1,5"))

	assert.Equal(t, report.AmountError{Kind: report.AmountNotAnAmount, Bound: "min", Value: "-1"}, err)
}

func Test_ParseSearchAmounts_refuses_a_bad_max_before_comparing_the_pair(t *testing.T) {
	_, err := report.ParseSearchAmounts(new("50"), new("1,5"))

	assert.Equal(t, report.AmountError{Kind: report.AmountNotAnAmount, Bound: "max", Value: "1,5"}, err)
}

func Test_ParseSearchAmounts_leaves_a_bound_that_was_not_given_open(t *testing.T) {
	got, err := report.ParseSearchAmounts(nil, nil)

	require.NoError(t, err)
	assert.Equal(t, report.SearchAmounts{}, got)
}
