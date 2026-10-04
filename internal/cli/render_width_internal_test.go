// White-box: widestLen and the mismatch row builders are unexported; the
// padding of non-ASCII labels is asserted on the built rows directly.
package cli

import (
	"testing"
	"time"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
)

func Test_widestLen_counts_runes_not_bytes(t *testing.T) {
	cases := []struct {
		name string
		ss   []string
		want int
	}{
		{name: "empty slice", ss: nil, want: 0},
		{name: "ASCII control", ss: []string{"ab", "abcd", "abc"}, want: 4},
		{name: "two-byte rune counts once", ss: []string{"Épargne", "abc"}, want: 7},
		{name: "CJK counts once per glyph", ss: []string{"ニホン", "ab"}, want: 3},
		{name: "four-byte emoji counts once", ss: []string{"📈📉", "abc"}, want: 3},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, widestLen(c.ss))
		})
	}
}

func Test_shareMismatchRows_pad_non_ASCII_names_by_runes(t *testing.T) {
	rows := shareMismatchRows([]store.ShareMismatch{
		{Account: "Épargne", Currency: "CAD", Active: true, Security: "Fund", Quarry: 1_000_000, Quicken: 0, Difference: 1_000_000},
		{Account: "RRSP", Currency: "USD", Active: true, Security: "ニホン", Quarry: 2_000_000, Quicken: 0, Difference: 2_000_000},
	})

	assert.Equal(t, []string{
		"  ! Épargne (CAD)  Fund  quarry 1  Quicken 0  difference 1",
		"  ! RRSP (USD)     ニホン   quarry 2  Quicken 0  difference 2",
	}, rows)
}

func Test_balanceMismatchRows_pad_non_ASCII_labels_by_runes(t *testing.T) {
	day := time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)

	rows := balanceMismatchRows([]store.BalanceMismatch{
		{Name: "Épargne courante", Currency: "CAD", Active: true, StatementDate: day, Quarry: 831000, Quicken: 830000, Difference: 1000},
		{Name: "Chequing", Currency: "CAD", Active: true, StatementDate: day, Quarry: 831000, Quicken: 830000, Difference: 1000},
	})

	assert.Equal(t, []string{
		"  ! Épargne courante (CAD)  2026-08-31  quarry 8,310.00  Quicken 8,300.00  difference 10.00",
		"  ! Chequing (CAD)          2026-08-31  quarry 8,310.00  Quicken 8,300.00  difference 10.00",
	}, rows)
}
