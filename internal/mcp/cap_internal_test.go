// White-box: capList is an unexported generic helper whose bounds are cheapest to drive directly.
package mcp

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func numbered(n int) []int {
	list := make([]int, n)
	for i := range list {
		list[i] = i
	}
	return list
}

func Test_capList_leaves_a_list_within_the_cap_and_its_warnings_untouched(t *testing.T) {
	cases := map[string]int{"empty": 0, "one row": 1, "one under the cap": maxRows - 1, "exactly the cap": maxRows}

	for name, n := range cases {
		t.Run(name, func(t *testing.T) {
			list := numbered(n)

			got, warnings := capList(list, []string{"earlier"}, "spending", "rows", "pass less")

			assert.Equal(t, list, got)
			assert.Equal(t, []string{"earlier"}, warnings)
		})
	}
}

func Test_capList_cuts_one_row_over_the_cap_and_ends_the_warnings_with_the_line(t *testing.T) {
	got, warnings := capList(numbered(maxRows+1), []string{"earlier"}, "spending", "rows", "pass less")

	assert.Equal(t, numbered(maxRows), got)
	assert.Equal(t, []string{"earlier", "spending lists the first 500 rows of 501; pass less"}, warnings)
}

func Test_capList_groups_the_count_by_thousands(t *testing.T) {
	_, warnings := capList(numbered(1234), nil, "cash_flow", "periods", "pass less")

	assert.Equal(t, []string{"cash_flow lists the first 500 periods of 1,234; pass less"}, warnings)
}
