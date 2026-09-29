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
	moneyNotANumber
)

// dollarBound is the DECIMAL(18,2)-safe bound (exclusive) on whole dollars: cents = dollars*100 must fit 18 unscaled digits.
const dollarBound = 10_000_000_000_000_000 // 1e16

// realIntBound is the bound (exclusive) on a REAL's integer part: above it SQLite
// can render a 2-decimal REAL with float noise, so its cents cannot be trusted.
const realIntBound = 1_000_000_000 // 1e9

// parseMoney reads typ (SQLite's typeof()) and text (CAST(col AS TEXT))
// for one non-NULL money column and returns its value in cents, parsed
// exactly from the digit string — never through float arithmetic. It
// reports moneyPrecision for more than 2 decimal places, moneyTooLarge for
// a value outside DECIMAL(18,2)'s trustworthy range, and moneyNotANumber
// for a column stored as text or blob, or real text that is not a decimal.
func parseMoney(typ, text string) (cents int64, fault moneyFault) {
	switch typ {
	case "integer":
		return parseIntegerMoney(text)
	case "real":
		return parseRealMoney(text)
	default:
		return 0, moneyNotANumber
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

// parseRealMoney reads a real-stored money column's decimal text, as SQLite
// renders a REAL: an optional "-", digits, and an optional "." with digits.
func parseRealMoney(text string) (int64, moneyFault) {
	neg := strings.HasPrefix(text, "-")
	unsigned := strings.TrimPrefix(text, "-")
	if unsigned == "Inf" || strings.ContainsAny(unsigned, "eE") {
		return 0, moneyTooLarge
	}
	intPart, fracPart, _ := strings.Cut(unsigned, ".")
	if intPart == "" || !allDigits(intPart) || !allDigits(fracPart) {
		return 0, moneyNotANumber
	}
	if len(fracPart) > 2 {
		return 0, moneyPrecision
	}
	intVal, err := strconv.ParseInt(intPart, 10, 64)
	if err != nil || intVal >= realIntBound {
		return 0, moneyTooLarge
	}
	fracPart += "00"[len(fracPart):]
	cents := intVal*100 + int64(fracPart[0]-'0')*10 + int64(fracPart[1]-'0')
	if neg {
		cents = -cents
	}
	return cents, moneyOK
}

// allDigits reports whether s holds only ASCII digits; "" does.
func allDigits(s string) bool {
	return strings.Trim(s, "0123456789") == ""
}
