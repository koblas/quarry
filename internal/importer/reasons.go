package importer

import "fmt"

// The reason* functions render an S4 refusal's <reason> text, ruled
// verbatim in specification.md's Surface & Copy section.

func reasonAccountCurrency(name, currency string) string {
	return fmt.Sprintf("account %q uses currency %s; quarry supports CAD and USD accounts", name, currency)
}

func reasonAccountType(name, typ string) string {
	return fmt.Sprintf("account %q has type %s, which quarry does not map yet", name, typ)
}

func reasonAccountNoName(sourceID int64) string {
	return fmt.Sprintf("an account (source id %d) has no name", sourceID)
}

func reasonAccountNoType(name string) string {
	return fmt.Sprintf("account %q has no type", name)
}

func reasonAccountNoCurrency(name string) string {
	return fmt.Sprintf("account %q has no currency", name)
}

func reasonTransactionPrecision(date, account, amount string) string {
	return fmt.Sprintf("a transaction on %s in %q has an amount of %s, which has more than 2 decimal places", date, account, amount)
}

func reasonTransactionTooLarge(date, account, amount string) string {
	return fmt.Sprintf("a transaction on %s in %q has an amount of %s, which is too large for quarry's amounts", date, account, amount)
}

func reasonSplitPrecision(date, account, amount string) string {
	return fmt.Sprintf("a split of a transaction on %s in %q has an amount of %s, which has more than 2 decimal places", date, account, amount)
}

func reasonSplitTooLarge(date, account, amount string) string {
	return fmt.Sprintf("a split of a transaction on %s in %q has an amount of %s, which is too large for quarry's amounts", date, account, amount)
}

func reasonTransactionStatus(date, account string, status int64) string {
	return fmt.Sprintf("a transaction on %s in %q has reconcile status %d, which quarry does not map yet", date, account, status)
}

func reasonCategoryType(fullPath string, kind int64) string {
	return fmt.Sprintf("category %q has type %d, which quarry does not map yet", fullPath, kind)
}

func reasonTransactionNoAccount(sourceID int64) string {
	return fmt.Sprintf("a transaction (source id %d) has no account", sourceID)
}

func reasonTransactionNoAmount(date, account string) string {
	return fmt.Sprintf("a transaction on %s in %q has no amount", date, account)
}

func reasonTransactionNoDate(account string, sourceID int64) string {
	return fmt.Sprintf("a transaction in %q (source id %d) has no date", account, sourceID)
}

func reasonSplitNoAmount(date, account string) string {
	return fmt.Sprintf("a split of a transaction on %s in %q has no amount", date, account)
}

func reasonSplitNoTransaction(sourceID int64) string {
	return fmt.Sprintf("a split (source id %d) has no transaction", sourceID)
}

func reasonCategoryNoName(sourceID int64) string {
	return fmt.Sprintf("category (source id %d) has no name", sourceID)
}

func reasonCategoryNoType(fullPath string) string {
	return fmt.Sprintf("category %q has no type", fullPath)
}
