package importer

import (
	"math/big"
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

// snapToleranceInverse is 1 over the tolerance, in dollars, within which a REAL
// with more than 2 decimals is float residue of the cent it sits on.
const snapToleranceInverse = 1_000_000 // tolerance 1e-6

// parseMoney reads typ (SQLite's typeof()) and text (CAST(col AS TEXT))
// for one non-NULL money column and returns its value in cents, parsed
// exactly from the digit string — never through float arithmetic. It
// reports moneyPrecision for more than 2 decimal places, moneyTooLarge for
// a value outside DECIMAL(18,2)'s trustworthy range, and moneyNotANumber
// for a column stored as text or blob, or real text that is not a decimal.
func parseMoney(typ, text string) (int64, moneyFault) {
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

// parseRealMoney reads a real-stored money column's text as SQLite renders a
// REAL: an optional "-", digits, and an optional "." with digits, or exponent form.
func parseRealMoney(text string) (int64, moneyFault) {
	unsigned := strings.TrimPrefix(text, "-")
	cents, fault := realCents(unsigned)
	if fault != moneyOK {
		return 0, fault
	}
	if unsigned != text {
		cents = -cents
	}
	return cents, moneyOK
}

// realCents returns the cents of an unsigned REAL text: exact for at most 2
// decimals, snapped to the nearest cent when the text is float residue of it.
func realCents(unsigned string) (int64, moneyFault) {
	if unsigned == "Inf" {
		return 0, moneyTooLarge
	}
	mantissa, hasExponent := unsigned, false
	if i := strings.IndexAny(unsigned, "eE"); i >= 0 {
		var exponent string
		mantissa, exponent, hasExponent = unsigned[:i], unsigned[i+1:], true
		if !strings.HasPrefix(exponent, "-") {
			return 0, moneyTooLarge
		}
		if exponent = exponent[1:]; exponent == "" || !allDigits(exponent) {
			return 0, moneyNotANumber
		}
	}
	intPart, fracPart, hasDot := strings.Cut(mantissa, ".")
	if intPart == "" || !allDigits(intPart) || !allDigits(fracPart) || (hasDot && fracPart == "") {
		return 0, moneyNotANumber
	}
	if hasExponent {
		return snappedCents(unsigned)
	}
	intVal, err := strconv.ParseInt(intPart, 10, 64)
	if err != nil || intVal >= realIntBound {
		return 0, moneyTooLarge
	}
	if len(fracPart) > 2 {
		return snappedCents(unsigned)
	}
	fracPart += "00"[len(fracPart):]
	return intVal*100 + int64(fracPart[0]-'0')*10 + int64(fracPart[1]-'0'), moneyOK
}

// snappedCents snaps the unsigned decimal text to its cent, refusing text
// beyond the tolerance or a snap that reaches the REAL bound.
func snappedCents(unsigned string) (int64, moneyFault) {
	cents, ok := snapToCent(unsigned)
	switch {
	case !ok:
		return 0, moneyPrecision
	case cents >= realIntBound*100:
		return 0, moneyTooLarge
	}
	return cents, moneyOK
}

// snapToCent returns the whole cents nearest the non-negative decimal text and
// whether text lies within the snap tolerance of them, compared exactly.
func snapToCent(text string) (int64, bool) {
	dollars, ok := new(big.Rat).SetString(text)
	if !ok {
		return 0, false
	}
	scaled := dollars.Mul(dollars, big.NewRat(100, 1))
	nearest := new(big.Rat).Add(scaled, big.NewRat(1, 2))
	cents := new(big.Int).Quo(nearest.Num(), nearest.Denom())
	distance := new(big.Rat).Sub(scaled, new(big.Rat).SetInt(cents))
	if distance.Abs(distance).Cmp(big.NewRat(100, snapToleranceInverse)) > 0 {
		return 0, false
	}
	return cents.Int64(), true
}

// allDigits reports whether s holds only ASCII digits; "" does.
func allDigits(s string) bool {
	return strings.Trim(s, "0123456789") == ""
}
