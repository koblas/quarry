package tomlstr

import (
	"fmt"
	"strings"
	"unicode"
)

// BasicString writes s as a TOML basic string, always quoted: quote, backslash, newline and tab are
// escaped, other control characters as \uXXXX, and everything else kept, so s shows on one line.
func BasicString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"':
			b.WriteString(`\"`)
		case r == '\\':
			b.WriteString(`\\`)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\t':
			b.WriteString(`\t`)
		case unicode.IsControl(r):
			fmt.Fprintf(&b, `\u%04X`, r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')

	return b.String()
}
