package humanize_test

import (
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
