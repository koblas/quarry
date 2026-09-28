package importer

import (
	"strconv"
	"strings"
)

// moneyFault classifies a money column's text that parseMoney cannot map
// to quarry's int64-cents representation.
type moneyFault int

const (
	moneyOK moneyFault = iota
	moneyPrecision
	moneyTooLarge
)

// dollarBound is the largest magnitude (exclusive) an integer-stored money
// column may hold in whole dollars: cents = dollars*100 must stay inside
// DECIMAL(18,2)'s 18-digit unscaled range.
const dollarBound = 10_000_000_000_000_000 // 1e16

// realIntBound is the largest magnitude (exclusive) a real-stored money
// column's integer part may hold: SQLite prints a REAL to 15 significant
// digits, so cents above this are not trustworthy.
const realIntBound = 10_000_000_000_000 // 1e13

// parseMoney reads typ (SQLite's typeof()) and text (CAST(col AS TEXT))
// for one non-NULL money column and returns its value in cents, parsed
// exactly from the digit string — never through float arithmetic. It
// reports moneyPrecision for more than 2 decimal places and moneyTooLarge
// for a value outside DECIMAL(18,2)'s trustworthy range.
func parseMoney(typ, text string) (cents int64, fault moneyFault) {
	switch typ {
	case "integer":
		return parseIntegerMoney(text)
	case "real":
		return parseRealMoney(text)
	default:
		// unreachable: a money column is only ever NULL, integer or real
		// stored; callers only reach parseMoney for a non-NULL one.
		return 0, moneyTooLarge
	}
}

// parseIntegerMoney reads an integer-stored money column: text is whole
// dollars with no fractional part, so cents is always dollars*100.
func parseIntegerMoney(text string) (int64, moneyFault) {
	dollars, err := strconv.ParseInt(text, 10, 64)
	if err != nil || dollars >= dollarBound || dollars <= -dollarBound {
		return 0, moneyTooLarge
	}
	return dollars * 100, moneyOK
}

// parseRealMoney reads a real-stored money column's decimal text.
func parseRealMoney(text string) (int64, moneyFault) {
	if strings.ContainsAny(text, "eE") {
		return 0, moneyTooLarge
	}
	neg := strings.HasPrefix(text, "-")
	unsigned := strings.TrimPrefix(text, "-")
	intPart, fracPart, _ := strings.Cut(unsigned, ".")
	if len(fracPart) > 2 {
		return 0, moneyPrecision
	}
	intVal, err := strconv.ParseInt(intPart, 10, 64)
	if err != nil || intVal >= realIntBound {
		return 0, moneyTooLarge
	}
	for len(fracPart) < 2 {
		fracPart += "0"
	}
	fracVal, err := strconv.ParseInt(fracPart, 10, 64)
	if err != nil {
		// unreachable: fracPart is "" or a 1-2 digit run cut from text by
		// strings.Cut, always padded to exactly 2 numeric digits above.
		return 0, moneyTooLarge
	}
	cents := intVal*100 + fracVal
	if neg {
		cents = -cents
	}
	return cents, moneyOK
}
