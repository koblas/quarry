package importer

import "fmt"

// The reason* functions render an unmappable-value refusal's <reason> text, ruled
// verbatim in specification.md's Surface & Copy section. Subjects are
// wrapped in literal double quotes ("%s") rather than Go's %q verb, which
// escapes a quote or backslash inside the name — the ruled copy never does.

func reasonAccountCurrency(name, currency string) string {
	return fmt.Sprintf("account \"%s\" uses currency %s; quarry supports CAD and USD accounts", name, currency)
}

func reasonAccountType(name, typ string) string {
	return fmt.Sprintf("account \"%s\" has type %s, which quarry does not map yet", name, typ)
}

func reasonAccountNoName(sourceID int64) string {
	return fmt.Sprintf("an account (source id %d) has no name", sourceID)
}

func reasonAccountNoType(name string) string {
	return fmt.Sprintf("account \"%s\" has no type", name)
}

func reasonAccountNoCurrency(name string) string {
	return fmt.Sprintf("account \"%s\" has no currency", name)
}

func reasonTransactionPrecision(date, account, amount string) string {
	return fmt.Sprintf("a transaction on %s in \"%s\" has an amount of %s, which has more than 2 decimal places", date, account, amount)
}

func reasonTransactionTooLarge(date, account, amount string) string {
	return fmt.Sprintf("a transaction on %s in \"%s\" has an amount of %s, which is too large for quarry's amounts", date, account, amount)
}

func reasonSplitPrecision(date, account, amount string) string {
	return fmt.Sprintf("a split of a transaction on %s in \"%s\" has an amount of %s, which has more than 2 decimal places", date, account, amount)
}

func reasonSplitTooLarge(date, account, amount string) string {
	return fmt.Sprintf("a split of a transaction on %s in \"%s\" has an amount of %s, which is too large for quarry's amounts", date, account, amount)
}

func reasonTransactionNotANumber(date, account string) string {
	return fmt.Sprintf("a transaction on %s in \"%s\" has an amount that is not a number", date, account)
}

func reasonTransactionNotANumberNoDate(account string, sourceID int64) string {
	return fmt.Sprintf("a transaction in \"%s\" (source id %d) has an amount that is not a number", account, sourceID)
}

func reasonSplitNotANumber(date, account string) string {
	return fmt.Sprintf("a split of a transaction on %s in \"%s\" has an amount that is not a number", date, account)
}

func reasonTransactionStatus(date, account string, status int64) string {
	return fmt.Sprintf("a transaction on %s in \"%s\" has reconcile status %d, which quarry does not map yet", date, account, status)
}

func reasonCategoryType(fullPath string, kind int64) string {
	return fmt.Sprintf("category \"%s\" has type %d, which quarry does not map yet", fullPath, kind)
}

func reasonTransactionNoAmount(date, account string) string {
	return fmt.Sprintf("a transaction on %s in \"%s\" has no amount", date, account)
}

func reasonTransactionNoDate(account string, sourceID int64) string {
	return fmt.Sprintf("a transaction in \"%s\" (source id %d) has no date", account, sourceID)
}

func reasonSplitNoAmount(date, account string) string {
	return fmt.Sprintf("a split of a transaction on %s in \"%s\" has no amount", date, account)
}

func reasonCategoryNoName(sourceID int64) string {
	return fmt.Sprintf("category (source id %d) has no name", sourceID)
}

func reasonCategoryNoType(fullPath string) string {
	return fmt.Sprintf("category \"%s\" has no type", fullPath)
}

func reasonStatementPrecision(date, account, amount string) string {
	return fmt.Sprintf("the %s statement for \"%s\" has a balance of %s, which has more than 2 decimal places", date, account, amount)
}

func reasonStatementPrecisionNoDate(account string, sourceID int64, amount string) string {
	return fmt.Sprintf("a statement for \"%s\" (source id %d) has a balance of %s, which has more than 2 decimal places", account, sourceID, amount)
}

func reasonStatementTooLarge(date, account, amount string) string {
	return fmt.Sprintf("the %s statement for \"%s\" has a balance of %s, which is too large for quarry's amounts", date, account, amount)
}

func reasonStatementTooLargeNoDate(account string, sourceID int64, amount string) string {
	return fmt.Sprintf("a statement for \"%s\" (source id %d) has a balance of %s, which is too large for quarry's amounts", account, sourceID, amount)
}

func reasonStatementNotANumber(date, account string) string {
	return fmt.Sprintf("the %s statement for \"%s\" has a balance that is not a number", date, account)
}

func reasonStatementNotANumberNoDate(account string, sourceID int64) string {
	return fmt.Sprintf("a statement for \"%s\" (source id %d) has a balance that is not a number", account, sourceID)
}

func reasonStatementNoBalance(date, account string) string {
	return fmt.Sprintf("the %s statement for \"%s\" has no balance", date, account)
}

func reasonStatementNoDate(account string, sourceID int64) string {
	return fmt.Sprintf("a statement for \"%s\" (source id %d) has no date", account, sourceID)
}

func reasonPriceNotANumber(security, date string) string {
	return fmt.Sprintf("a price of \"%s\" on %s is not a number", security, date)
}

func reasonPriceTooLarge(security, date, price string) string {
	return fmt.Sprintf("a price of \"%s\" on %s is %s, which is too large for quarry's prices", security, date, price)
}

func reasonInvestmentActionCode(date, account string, code int64) string {
	return fmt.Sprintf("an investment transaction on %s in \"%s\" has action code %d, which quarry does not map yet", date, account, code)
}

func reasonInvestmentNoActionCode(date, account string) string {
	return fmt.Sprintf("an investment transaction on %s in \"%s\" has no action code", date, account)
}

func reasonInvestmentNoDate(account string, sourceID int64) string {
	return fmt.Sprintf("an investment transaction in \"%s\" (source id %d) has no date", account, sourceID)
}

func reasonInvestmentNoAmount(date, account string) string {
	return fmt.Sprintf("an investment transaction on %s in \"%s\" has no amount", date, account)
}

// In the reasonInvestment value refusals below, what names the column: "a commission", "an amount" or "a share count".

func reasonInvestmentNotANumber(date, account, what string) string {
	return fmt.Sprintf("an investment transaction on %s in \"%s\" has %s that is not a number", date, account, what)
}

func reasonInvestmentMoneyPrecision(date, account, what, amount string, places int) string {
	return fmt.Sprintf("an investment transaction on %s in \"%s\" has %s of %s, which has more than %d decimal places", date, account, what, amount, places)
}

func reasonInvestmentMoneyTooLarge(date, account, what, amount string) string {
	return fmt.Sprintf("an investment transaction on %s in \"%s\" has %s of %s, which is too large for quarry's amounts", date, account, what, amount)
}

func reasonInvestmentSharesWithoutSecurity(date, account string) string {
	return fmt.Sprintf("an investment transaction on %s in \"%s\" has shares but no security", date, account)
}

// reasonSplitRatio names the split's security; an empty security leaves the "of" clause out.
func reasonSplitRatio(date, account, security, numerator, denominator string) string {
	of := ""
	if security != "" {
		of = fmt.Sprintf(" of \"%s\"", security)
	}
	return fmt.Sprintf("a stock split on %s in \"%s\"%s has a ratio quarry cannot read (%s:%s)", date, account, of, numerator, denominator)
}

func reasonSecurityNoName(sourceID int64) string {
	return fmt.Sprintf("a security (source id %d) has no name", sourceID)
}

func reasonInvestmentSharesPrecision(date, account, shares string) string {
	return fmt.Sprintf("an investment transaction on %s in \"%s\" has %s shares, which has more than 6 decimal places", date, account, shares)
}

func reasonInvestmentSharesTooLarge(date, account, shares string) string {
	return fmt.Sprintf("an investment transaction on %s in \"%s\" has %s shares, which is too large for quarry's share counts", date, account, shares)
}

// reasonNoLotEntity is the refusal of imported investment transactions with no Lot entity to check them against.
const reasonNoLotEntity = "the snapshot has investment transactions but no Quicken lots to check their share counts against"

func reasonLotNoShareCount(security, account string) string {
	return fmt.Sprintf("a lot of \"%s\" in \"%s\" has no share count", security, account)
}

func reasonLotNotANumber(security, account string) string {
	return fmt.Sprintf("a lot of \"%s\" in \"%s\" has a share count that is not a number", security, account)
}

func reasonLotSharesPrecision(security, account, shares string) string {
	return fmt.Sprintf("a lot of \"%s\" in \"%s\" has %s shares, which has more than 6 decimal places", security, account, shares)
}

func reasonLotSharesTooLarge(security, account, shares string) string {
	return fmt.Sprintf("a lot of \"%s\" in \"%s\" has %s shares, which is too large for quarry's share counts", security, account, shares)
}
