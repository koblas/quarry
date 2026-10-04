package duckstore

import (
	"fmt"

	"github.com/koblas/quarry/internal/platform/money"
)

// convertedColumn returns the column of a view holding column's value converted into currency, column_cad or
// column_usd, and that currency's code; the bool is false for Native and any value outside the three, which convert nothing.
func convertedColumn(column string, currency money.Currency) (string, string, bool) {
	switch currency { //nolint:exhaustive // Native, and any value outside the three, converts nothing
	case money.CAD:
		return column + "_cad", "CAD", true
	case money.USD:
		return column + "_usd", "USD", true
	default:
		return "", "", false
	}
}

// keepOwnCurrency is the currency and column projections of a relation converting column into code through
// converted: a split with no converted cell keeps its own currency and amount, so a total never mixes them.
func keepOwnCurrency(column, converted, code string) string {
	return fmt.Sprintf(`CASE WHEN %[1]s IS NULL THEN currency ELSE '%[2]s' END AS currency,
	COALESCE(%[1]s, %[3]s) AS %[3]s`, converted, code, column)
}

// convertedTo is the SQL for amount (DECIMAL(18,2) in currency) in target, "CAD" or "USD", at rate (CAD
// per USD): NULL when amount is NULL, the currency is neither, or a conversion needs a NULL rate.
func convertedTo(target, amount, currency, rate string) string {
	return convertedToWide(target, amount, currency, rate, 18)
}

// convertedToWide is convertedTo for an amount, and a result, of type DECIMAL(digits,2).
func convertedToWide(target, amount, currency, rate string, digits int) string {
	if target == "CAD" {
		// The DECIMAL(38,2) operand keeps the product from overflowing; the cast back rounds half away from zero.
		return fmt.Sprintf(`CASE WHEN %[2]s = 'CAD' THEN %[1]s
		WHEN %[2]s = 'USD' THEN CAST(CAST(%[1]s AS DECIMAL(38,2)) * %[3]s AS DECIMAL(%[4]d,2)) END`,
			amount, currency, rate, digits)
	}
	// With c cents and r millionths of a rate, c * 1000000 / r rounded half away from zero is
	// sign(c) * ((2|c| * 1000000 + r) // 2r); DOUBLE division would round some halves the wrong way.
	cents := fmt.Sprintf("CAST(CAST(%s AS DECIMAL(38,2)) * 100 AS HUGEINT)", amount)
	micro := fmt.Sprintf("CAST(CAST(%s AS DECIMAL(38,6)) * 1000000 AS HUGEINT)", rate)
	return fmt.Sprintf(`CASE WHEN %[1]s = 'USD' THEN %[2]s
		WHEN %[1]s = 'CAD' THEN CAST(CAST(sign(%[3]s) * ((abs(%[3]s) * 2000000 + %[4]s) // (2 * %[4]s)) AS DECIMAL(38,0)) * 0.01 AS DECIMAL(%[5]d,2)) END`,
		currency, amount, cents, micro, digits)
}
