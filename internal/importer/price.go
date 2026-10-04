package importer

import (
	"math/big"
	"strings"
)

// priceUnscaledBound is the exclusive bound on a price's millionths: a DECIMAL(18,6) holds 18 unscaled digits.
var priceUnscaledBound = new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)

// priceScale is the factor from a price to its millionths, the DECIMAL(18,6) unscaled value.
var priceScale = big.NewRat(1_000_000, 1)

// sharesBound is the exclusive bound on a share count's magnitude: a DECIMAL(18,6) holds 10^12 shares.
const sharesBound = 1_000_000_000_000

// sharesSnapToleranceInverse is 1 over the share tolerance within which a REAL is float residue of its millionth.
const sharesSnapToleranceInverse = 1_000_000_000 // tolerance 1e-9 shares

// parsePrice returns the millionths of a price column from its typeof() and text, rounded half
// to even; a value reaching the DECIMAL(18,6) bound is moneyTooLarge, non-numeric moneyNotANumber.
func parsePrice(typ, text string) (int64, moneyFault) {
	value, negative, fault := decimalColumn(typ, text)
	if fault != moneyOK {
		return 0, fault
	}
	rounded := roundHalfEven(value.Mul(value, priceScale))
	if rounded.Cmp(priceUnscaledBound) >= 0 {
		return 0, moneyTooLarge
	}
	millionths := rounded.Int64()
	if negative {
		millionths = -millionths
	}
	return millionths, moneyOK
}

// parseShares returns the millionths of a share column exactly: a value within the snap tolerance of a
// millionth is that millionth, one further off is moneyPrecision, one reaching 10^12 shares moneyTooLarge.
func parseShares(typ, text string) (int64, moneyFault) {
	value, negative, fault := decimalColumn(typ, text)
	if fault != moneyOK {
		return 0, fault
	}
	if value.Cmp(big.NewRat(sharesBound, 1)) >= 0 {
		return 0, moneyTooLarge
	}
	millionths := roundHalfEven(new(big.Rat).Mul(value, priceScale))
	snapped := new(big.Rat).SetFrac(millionths, priceScale.Num())
	distance := snapped.Sub(snapped, value)
	if distance.Abs(distance).Cmp(big.NewRat(1, sharesSnapToleranceInverse)) > 0 {
		return 0, moneyPrecision
	}
	if millionths.Cmp(priceUnscaledBound) >= 0 {
		return 0, moneyTooLarge
	}
	if negative {
		return -millionths.Int64(), moneyOK
	}
	return millionths.Int64(), moneyOK
}

// decimalColumn returns the magnitude and sign of a numeric column from its typeof() and text;
// non-numeric text is moneyNotANumber and a REAL infinity moneyTooLarge.
func decimalColumn(typ, text string) (*big.Rat, bool, moneyFault) {
	if typ != "integer" && typ != "real" {
		return nil, false, moneyNotANumber
	}
	unsigned := strings.TrimPrefix(text, "-")
	if typ == "real" && unsigned == "Inf" {
		return nil, false, moneyTooLarge
	}
	if !isDecimalText(unsigned) {
		return nil, false, moneyNotANumber
	}
	value, ok := new(big.Rat).SetString(unsigned)
	if !ok {
		// An exponent beyond big.Rat's range passes isDecimalText but cannot be parsed.
		return nil, false, moneyNotANumber
	}
	return value, unsigned != text, moneyOK
}

// isDecimalText reports whether s is plain decimal text: digits, optional ".digits", optional exponent.
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
