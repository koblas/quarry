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

func reasonTransactionNoAccount(sourceID int64) string {
	return fmt.Sprintf("a transaction (source id %d) has no account", sourceID)
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

func reasonSplitNoTransaction(sourceID int64) string {
	return fmt.Sprintf("a split (source id %d) has no transaction", sourceID)
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
