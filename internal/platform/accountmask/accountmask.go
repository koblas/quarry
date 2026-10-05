package accountmask

import (
	"regexp"
	"strings"
	"unicode"
)

// keptDigits is how many trailing digits Mask leaves readable.
const keptDigits = 4

// quarryID matches an id quarry itself wrote: "acct-" and digits, which is a label, not a number.
var quarryID = regexp.MustCompile(`^acct-[0-9]+$`)

// Mask replaces every digit of s but the last four with "*", counting digits across the whole
// string and keeping every other rune. An id of the exact form acct-<digits> is returned unchanged.
func Mask(s string) string {
	if quarryID.MatchString(s) {
		return s
	}
	hidden := strings.Count(strings.Map(digitOnly, s), "d") - keptDigits
	if hidden <= 0 {
		return s
	}
	var b strings.Builder
	for _, r := range s {
		if hidden > 0 && unicode.IsDigit(r) {
			b.WriteByte('*')
			hidden--
			continue
		}
		b.WriteRune(r)
	}

	return b.String()
}

// digitOnly maps a digit to "d" and drops every other rune, so the result's length counts digits.
func digitOnly(r rune) rune {
	if unicode.IsDigit(r) {
		return 'd'
	}

	return -1
}
