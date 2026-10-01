package config_test

import (
	"testing"

	"github.com/koblas/quarry/internal/config"
	"github.com/stretchr/testify/assert"
)

func Test_BasicString_writes_text_as_one_quoted_line_with_its_special_characters_escaped(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{name: "a bare-looking word is still quoted", in: "abc", want: `"abc"`},
		{name: "empty is an empty pair of quotes", in: "", want: `""`},
		{name: "a double quote is escaped", in: `a"b`, want: `"a\"b"`},
		{name: "a backslash is escaped", in: `a\b`, want: `"a\\b"`},
		{name: "a newline stays on one line", in: "a\nb", want: `"a\nb"`},
		{name: "a tab is escaped", in: "a\tb", want: `"a\tb"`},
		{name: "another control character is a unicode escape", in: "a\x01b", want: `"a\u0001b"`},
		{name: "delete is a unicode escape", in: "a\x7fb", want: `"a\u007Fb"`},
		{name: "non-ASCII text is kept", in: "café", want: `"café"`},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, config.BasicString(c.in))
		})
	}
}
