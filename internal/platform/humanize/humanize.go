package humanize

import "strconv"

// Thousands renders n, which must be non-negative, with a comma every
// three digits from the right: 1234567 is "1,234,567".
func Thousands(n int) string {
	s := strconv.Itoa(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

// Count renders n thousands-grouped with singular at exactly 1 and plural
// otherwise: "1 account", "0 accounts", "1,035 accounts".
func Count(n int, singular, plural string) string {
	if n == 1 {
		return "1 " + singular
	}
	return Thousands(n) + " " + plural
}
