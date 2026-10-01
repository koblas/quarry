package duckstore

import "fmt"

// convertedTo is the SQL for amount (a DECIMAL(18,2) in currency) expressed in target, "CAD" or "USD", at
// rate (CAD per USD, DECIMAL(10,6)). It is DECIMAL(18,2), and NULL when amount is NULL, the currency is
// neither CAD nor USD, or a conversion is needed and rate is NULL. It rounds half away from zero, as
// money.Convert does: DuckDB's cast to DECIMAL(18,2) rounds a half away from zero, and the DECIMAL(38,2)
// operand keeps the product from overflowing the 18 digits an amount already has.
func convertedTo(target, amount, currency, rate string) string {
	if target == "CAD" {
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
