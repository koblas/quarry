// White-box: itemText's split-text/decoded-value disagreement cannot be produced from
// valid TOML, so only a direct call reaches it.
package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_itemText_spells_the_item_from_the_split_text_collapsed_to_one_line(t *testing.T) {
	raw := []string{`"a"`, "{ x = 1,\n  y = 2 }"}

	got := itemText(raw, []any{"a", map[string]any{"x": int64(1), "y": int64(2)}}, 1)

	assert.Equal(t, "{ x = 1, y = 2 }", got)
}

func Test_itemText_spells_the_decoded_value_when_the_split_text_disagrees_in_count(t *testing.T) {
	raw := []string{`"a`, `b"`, "12"}

	got := itemText(raw, []any{"a,b", int64(12)}, 1)

	assert.Equal(t, "12", got)
}

func Test_arrayItems_ends_a_string_that_never_closes_at_the_end_of_the_text(t *testing.T) {
	cases := []struct {
		name string
		text string
		want []string
	}{
		{name: "basic string", text: `["abc]`, want: []string{`"abc`}},
		{name: "multi-line basic string", text: `["""abc]`, want: []string{`"""abc`}},
		{name: "multi-line literal string with quotes at the end", text: `['''abc'']`, want: []string{`'''abc''`}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, arrayItems(c.text))
		})
	}
}
