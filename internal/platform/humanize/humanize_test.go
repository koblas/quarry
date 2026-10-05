package humanize_test

import (
	"math"
	"testing"

	"github.com/koblas/quarry/internal/platform/humanize"
	"github.com/stretchr/testify/assert"
)

func Test_Thousands(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		n    int
		want string
	}{
		{name: "zero", n: 0, want: "0"},
		{name: "three digits stay ungrouped", n: 999, want: "999"},
		{name: "four digits get one comma", n: 1000, want: "1,000"},
		{name: "seven digits get two commas", n: 1234567, want: "1,234,567"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, c.want, humanize.Thousands(c.n))
		})
	}
}

func Test_ThousandsDigits(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		digits string
		want   string
	}{
		{name: "empty", digits: "", want: ""},
		{name: "three digits stay ungrouped", digits: "999", want: "999"},
		{name: "four digits get one comma", digits: "1000", want: "1,000"},
		{name: "a number past int64 groups every three", digits: "18446744073709551616", want: "18,446,744,073,709,551,616"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, c.want, humanize.ThousandsDigits(c.digits))
		})
	}
}

func Test_Shares(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		millionths int64
		want       string
	}{
		{name: "one millionth", millionths: 1, want: "0.000001"},
		{name: "negative one millionth keeps its sign", millionths: -1, want: "-0.000001"},
		{name: "zero", millionths: 0, want: "0"},
		{name: "whole count has no fraction", millionths: 10_000_000, want: "10"},
		{name: "trailing fractional zeros are trimmed", millionths: 120_500_000, want: "120.5"},
		{name: "whole count thousands-grouped", millionths: 1_200_000_000, want: "1,200"},
		{name: "full six-decimal fraction", millionths: 1_000_001, want: "1.000001"},
		{name: "smallest count does not overflow on negation", millionths: math.MinInt64, want: "-9,223,372,036,854.775808"},
		{name: "largest count", millionths: math.MaxInt64, want: "9,223,372,036,854.775807"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, c.want, humanize.Shares(c.millionths))
		})
	}
}

func Test_Count(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		n    int
		want string
	}{
		{name: "singular at one", n: 1, want: "1 account"},
		{name: "plural at zero", n: 0, want: "0 accounts"},
		{name: "plural at many, grouped", n: 1035, want: "1,035 accounts"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, c.want, humanize.Count(c.n, "account", "accounts"))
		})
	}
}
