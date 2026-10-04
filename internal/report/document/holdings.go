package document

import (
	"fmt"
	"math/big"

	"github.com/koblas/quarry/internal/report"
)

// bigCentsPerUnit is how many cents make one whole unit of currency.
var bigCentsPerUnit = big.NewInt(100)

// BigMoney renders cents, of any size, as a 2-decimal amount with a leading "-" for a negative
// value and no thousands grouping; it is Money for a value that can pass 64 bits.
func BigMoney(cents *big.Int) string {
	// QuoRem truncates toward zero; DivMod would split -5 cents as -1 and 95.
	whole, fraction := new(big.Int).QuoRem(new(big.Int).Abs(cents), bigCentsPerUnit, new(big.Int))
	s := fmt.Sprintf("%s.%02d", whole, fraction.Int64())
	if cents.Sign() < 0 {
		return "-" + s
	}
	return s
}

// HoldingsWarnings is h's warnings, none yet; it is never nil.
func HoldingsWarnings(report.Holdings) []string {
	return []string{}
}
