package document_test

import (
	"math"
	"testing"

	"github.com/koblas/quarry/internal/finding"
	"github.com/koblas/quarry/internal/report/document"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
)

func Test_Money(t *testing.T) {
	cases := []struct {
		name  string
		cents int64
		want  string
	}{
		{name: "zero", cents: 0, want: "0.00"},
		{name: "negative with a zero integer part", cents: -1, want: "-0.01"},
		{name: "negative below one unit", cents: -50, want: "-0.50"},
		{name: "positive with both parts", cents: 120417, want: "1204.17"},
		{name: "large negative has no thousands grouping", cents: -100000000, want: "-1000000.00"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, document.Money(c.cents))
		})
	}
}

func Test_Shares(t *testing.T) {
	cases := []struct {
		name       string
		millionths int64
		want       string
	}{
		{name: "zero", millionths: 0, want: "0.000000"},
		{name: "negative below one share", millionths: -1, want: "-0.000001"},
		{name: "whole shares keep six decimals", millionths: 1_200_000_000, want: "1200.000000"},
		{name: "fraction and whole part", millionths: 120_500_000, want: "120.500000"},
		{name: "smallest int64 does not overflow on negation", millionths: math.MinInt64, want: "-9223372036854.775808"},
		{name: "largest int64", millionths: math.MaxInt64, want: "9223372036854.775807"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, document.Shares(c.millionths))
		})
	}
}

func Test_NullString_is_nil_only_for_the_empty_string(t *testing.T) {
	got := document.NullString("x")

	assert.Equal(t, "x", *got)
	assert.Nil(t, document.NullString(""))
}

func Test_NewFindingCounts_carries_every_count_to_its_own_field(t *testing.T) {
	got := document.NewFindingCounts(finding.Counts{Open: 1, Ignored: 2, Fixed: 3, New: 4, NewlyFixed: 5})

	assert.Equal(t, document.FindingCounts{Open: 1, Ignored: 2, Fixed: 3, New: 4, NewlyFixed: 5}, got)
}

func Test_NewRows_carries_every_table_count_to_its_own_field(t *testing.T) {
	got := document.NewRows(store.Counts{
		Accounts: 1, Categories: 2, Payees: 3, Tags: 4, Transactions: 5, Splits: 6, SplitTags: 7, Transfers: 8,
	})

	assert.Equal(t, document.Rows{
		Accounts: 1, Categories: 2, Payees: 3, Tags: 4, Transactions: 5, Splits: 6, SplitTags: 7, Transfers: 8,
	}, got)
}
