package money

import "math/big"

// Rate is CAD per one USD, in millionths: 1.25 is Rate(1250000).
type Rate int64

// Currency is a currency a report can show amounts in.
type Currency int

// The currencies Convert understands. Native is an account's own currency,
// which is never a conversion target or source.
const (
	Native Currency = iota
	CAD
	USD
)

// millionth is the divisor between a Rate and a plain multiplier.
const millionth = 1_000_000

// Convert converts cents held in from into to at rate (CAD per USD), rounding
// half away from zero. It reports false when no conversion exists: from is
// Native, to is neither Native nor a known currency, or the currencies differ
// and rate is not positive. to == Native and from == to return cents unchanged.
func Convert(cents int64, from, to Currency, rate Rate) (int64, bool) {
	switch {
	case from == Native:
		return 0, false
	case to == Native || from == to:
		return cents, true
	case rate <= 0:
		return 0, false
	case from == USD && to == CAD:
		return divRound(mul(cents, int64(rate)), big.NewInt(millionth))
	case from == CAD && to == USD:
		return divRound(mul(cents, millionth), big.NewInt(int64(rate)))
	default:
		return 0, false
	}
}

func mul(a, b int64) *big.Int {
	return new(big.Int).Mul(big.NewInt(a), big.NewInt(b))
}

// divRound is num/den rounded half away from zero, for den > 0. It reports
// false when the quotient does not fit an int64.
func divRound(num, den *big.Int) (int64, bool) {
	sign := num.Sign()
	twice := new(big.Int).Lsh(new(big.Int).Abs(num), 1)
	twice.Add(twice, den)
	q := twice.Quo(twice, new(big.Int).Lsh(den, 1))
	if sign < 0 {
		q.Neg(q)
	}
	if !q.IsInt64() {
		return 0, false
	}
	return q.Int64(), true
}
