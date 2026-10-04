package importer

import (
	"math/big"
	"strings"
)

// priceUnscaledBound is the exclusive bound on a price's millionths: a DECIMAL(18,6) holds 18 unscaled digits.
var priceUnscaledBound = new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)

// priceScale is the factor from a price to its millionths, the DECIMAL(18,6) unscaled value.
var priceScale = big.NewRat(1_000_000, 1)

// fixedPoint is a DECIMAL(18, n) column read exactly by parseFixed: scale is the factor from a value to its
// unscaled units, bound the exclusive magnitude in whole units, toleranceInverse 1 over the distance within
// which a REAL is float residue of its nearest unit.
type fixedPoint struct {
	scale            *big.Rat
	bound            int64
	toleranceInverse int64
}

// sharesFixed is the DECIMAL(18,6) share column: 10^12 shares, tolerance 1e-9.
var sharesFixed = fixedPoint{scale: priceScale, bound: 1_000_000_000_000, toleranceInverse: 1_000_000_000}

// commissionFixed is the DECIMAL(18,4) commission column: 10^14 units, tolerance 1e-8, so a value a
// ten-thousandth apart from its neighbour is never mistaken for residue.
var commissionFixed = fixedPoint{scale: big.NewRat(10_000, 1), bound: 100_000_000_000_000, toleranceInverse: 100_000_000}

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

// parseShares returns the millionths of a share column exactly; see parseFixed.
func parseShares(typ, text string) (int64, moneyFault) {
	return parseFixed(typ, text, sharesFixed)
}

// parseCommission returns the ten-thousandths of a commission column exactly; see parseFixed.
func parseCommission(typ, text string) (int64, moneyFault) {
	return parseFixed(typ, text, commissionFixed)
}

// parseFixed returns the unscaled units of a numeric column exactly: a value within the snap tolerance of a
// unit is that unit, one further off is moneyPrecision, one reaching the bound moneyTooLarge.
func parseFixed(typ, text string, f fixedPoint) (int64, moneyFault) {
	value, negative, fault := decimalColumn(typ, text)
	if fault != moneyOK {
		return 0, fault
	}
	if value.Cmp(big.NewRat(f.bound, 1)) >= 0 {
		return 0, moneyTooLarge
	}
	units := roundHalfEven(new(big.Rat).Mul(value, f.scale))
	snapped := new(big.Rat).SetFrac(units, f.scale.Num())
	distance := snapped.Sub(snapped, value)
	if distance.Abs(distance).Cmp(big.NewRat(1, f.toleranceInverse)) > 0 {
		return 0, moneyPrecision
	}
	if units.Cmp(priceUnscaledBound) >= 0 {
		return 0, moneyTooLarge
	}
	if negative {
		return -units.Int64(), moneyOK
	}
	return units.Int64(), moneyOK
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
