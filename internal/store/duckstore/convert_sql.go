package duckstore

import "fmt"

// convertedTo is the SQL for amount (DECIMAL(18,2) in currency) in target, "CAD" or "USD", at rate (CAD
// per USD): NULL when amount is NULL, the currency is neither, or a conversion needs a NULL rate.
func convertedTo(target, amount, currency, rate string) string {
	if target == "CAD" {
		// The DECIMAL(38,2) operand keeps the product from overflowing 18 digits; the cast back rounds half away from zero.
		return fmt.Sprintf(`CASE WHEN %[2]s = 'CAD' THEN %[1]s
		WHEN %[2]s = 'USD' THEN CAST(CAST(%[1]s AS DECIMAL(38,2)) * %[3]s AS DECIMAL(18,2)) END`,
			amount, currency, rate)
	}
	// With c cents and r millionths of a rate, c * 1000000 / r rounded half away from zero is
	// sign(c) * ((2|c| * 1000000 + r) // 2r); DOUBLE division would round some halves the wrong way.
	cents := fmt.Sprintf("CAST(CAST(%s AS DECIMAL(38,2)) * 100 AS HUGEINT)", amount)
	micro := fmt.Sprintf("CAST(CAST(%s AS DECIMAL(38,6)) * 1000000 AS HUGEINT)", rate)
	return fmt.Sprintf(`CASE WHEN %[1]s = 'USD' THEN %[2]s
		WHEN %[1]s = 'CAD' THEN CAST(CAST(sign(%[3]s) * ((abs(%[3]s) * 2000000 + %[4]s) // (2 * %[4]s)) AS DECIMAL(38,0)) * 0.01 AS DECIMAL(18,2)) END`,
		currency, amount, cents, micro)
}
