package config

import "strings"

// arrayItems splits the text of a TOML array, brackets included, into its items as
// written, comments dropped. A bracket, quote or comma inside a string or a nested
// container does not split.
func arrayItems(text string) []string {
	body := text[1 : len(text)-1]
	var items []string
	var item strings.Builder
	depth := 0
	flush := func() {
		if trimmed := strings.TrimSpace(item.String()); trimmed != "" {
			items = append(items, trimmed)
		}
		item.Reset()
	}
	for i := 0; i < len(body); {
		switch c := body[i]; c {
		case '#':
			for i < len(body) && body[i] != '\n' {
				i++
			}
		case '"', '\'':
			end := stringEnd(body, i)
			item.WriteString(body[i:end])
			i = end
		case ',':
			if depth == 0 {
				flush()
			} else {
				item.WriteByte(c)
			}
			i++
		default:
			if c == '[' || c == '{' {
				depth++
			}
			if c == ']' || c == '}' {
				depth--
			}
			item.WriteByte(c)
			i++
		}
	}
	flush()

	return items
}

// stringEnd is the offset just past the TOML string that opens at s[start].
func stringEnd(s string, start int) int {
	quote := s[start]
	triple := strings.Repeat(string(quote), 3)
	if strings.HasPrefix(s[start:], triple) {
		i := start + 3
		for i < len(s) && !strings.HasPrefix(s[i:], triple) {
			i += escapeWidth(s, i, quote)
		}
		i += 3
		for i < len(s) && s[i] == quote { // up to two quotes may end the content
			i++
		}

		return min(i, len(s))
	}
	i := start + 1
	for i < len(s) && s[i] != quote {
		i += escapeWidth(s, i, quote)
	}

	return min(i+1, len(s))
}

// escapeWidth is how many bytes the character at s[i] takes: a backslash takes the
// next byte too in a basic string, never in a literal one.
func escapeWidth(s string, i int, quote byte) int {
	if s[i] == '\\' && quote == '"' {
		return 2
	}

	return 1
}
