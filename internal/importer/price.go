package importer

import (
	"math/big"
	"strings"
)

// priceUnscaledBound is the exclusive bound on a price's millionths: a DECIMAL(18,6) holds 18 unscaled digits.
var priceUnscaledBound = new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)

// priceScale is the factor from a price to its millionths, the DECIMAL(18,6) unscaled value.
var priceScale = big.NewRat(1_000_000, 1)

// parsePrice returns the millionths of one non-NULL price column from its typeof() and text. It
// rounds half to even on the exact decimal text and refuses a value whose rounded magnitude
// reaches the DECIMAL(18,6) bound with moneyTooLarge; text or blob is moneyNotANumber.
func parsePrice(typ, text string) (int64, moneyFault) {
	if typ != "integer" && typ != "real" {
		return 0, moneyNotANumber
	}
	unsigned := strings.TrimPrefix(text, "-")
	if typ == "real" && unsigned == "Inf" {
		return 0, moneyTooLarge
	}
	if !isDecimalText(unsigned) {
		return 0, moneyNotANumber
	}
	value, ok := new(big.Rat).SetString(unsigned)
	if !ok {
		// unreachable: isDecimalText admits only a digit run, an optional fraction and a
		// signed exponent, and SQLite renders a REAL's exponent within +-308, far inside big.Rat's range.
		return 0, moneyNotANumber
	}
	rounded := roundHalfEven(value.Mul(value, priceScale))
	if rounded.Cmp(priceUnscaledBound) >= 0 {
		return 0, moneyTooLarge
	}
	millionths := rounded.Int64()
	if unsigned != text {
		millionths = -millionths
	}
	return millionths, moneyOK
}

// isDecimalText reports whether s is digits, an optional "." with digits, and an
// optional exponent "e" or "E" with an optional sign and digits, as SQLite renders a number.
func isDecimalText(s string) bool {
	mantissa, exponent, hasExponent := strings.Cut(strings.ToLower(s), "e")
	if hasExponent {
		if exponent != "" && (exponent[0] == '+' || exponent[0] == '-') {
			exponent = exponent[1:]
		}
		if exponent == "" || !allDigits(exponent) {
			return false
		}
	}
	intPart, fracPart, hasDot := strings.Cut(mantissa, ".")
	return intPart != "" && allDigits(intPart) && allDigits(fracPart) && (!hasDot || fracPart != "")
}

// roundHalfEven returns the integer nearest the non-negative value, a tie going to the even neighbour.
func roundHalfEven(value *big.Rat) *big.Int {
	quotient, remainder := new(big.Int).QuoRem(value.Num(), value.Denom(), new(big.Int))
	switch remainder.Lsh(remainder, 1).Cmp(value.Denom()) {
	case 1:
		quotient.Add(quotient, big.NewInt(1))
	case 0:
		if quotient.Bit(0) == 1 {
			quotient.Add(quotient, big.NewInt(1))
		}
	}
	return quotient
}
