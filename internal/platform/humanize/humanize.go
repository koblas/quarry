package humanize

import (
	"fmt"
	"strconv"
	"strings"
)

// Thousands renders n, which must be non-negative, with a comma every
// three digits from the right: 1234567 is "1,234,567".
func Thousands(n int) string {
	return ThousandsDigits(strconv.Itoa(n))
}

// ThousandsDigits groups digits, a string of decimal digits of any length, with a comma every
// three digits from the right: "1234567" is "1,234,567".
func ThousandsDigits(digits string) string {
	for i := len(digits) - 3; i > 0; i -= 3 {
		digits = digits[:i] + "," + digits[i:]
	}
	return digits
}

// Shares renders millionths of a share thousands-grouped with trailing fractional zeros trimmed,
// and a leading "-" for a negative count: 1200500000 is "1,200.5".
func Shares(millionths int64) string {
	const perShare = 1_000_000
	// Split before negating: the whole and fraction parts of math.MinInt64 fit, its magnitude does not.
	whole, frac := millionths/perShare, millionths%perShare
	negative := millionths < 0
	if negative {
		whole, frac = -whole, -frac
	}
	s := Thousands(int(whole))
	if frac != 0 {
		s += "." + strings.TrimRight(fmt.Sprintf("%06d", frac), "0")
	}
	if negative {
		return "-" + s
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
