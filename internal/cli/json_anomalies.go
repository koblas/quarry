package cli

import "github.com/koblas/quarry/internal/report"

// anomaliesDocument is anomalies' --json stdout shape; Currency is the reporting currency ("native"
// lists every anomaly in its own).
type anomaliesDocument struct {
	Since         string                  `json:"since"`
	Until         string                  `json:"until"`
	Currency      string                  `json:"currency"`
	AccountFilter []accountFilterDocument `json:"account_filter"`
	Anomalies     []anomalyDocument       `json:"anomalies"`
	Checked       int                     `json:"checked"`
	NotJudged     int                     `json:"not_judged"`
	Warnings      []string                `json:"warnings"`
}

// anomalyDocument is one entry of "anomalies"; Payee is null for a charge with no payee and Category
// for an uncategorized or split charge. Currency, Amount and Usual are in the reporting currency unless the
// charge could not be converted; the Native fields are always the charge's own. Times is the charge as a
// multiple of its native Usual.
type anomalyDocument struct {
	TransactionID  string  `json:"transaction_id"`
	Date           string  `json:"date"`
	AccountID      string  `json:"account_id"`
	Account        string  `json:"account"`
	Currency       string  `json:"currency"`
	Payee          *string `json:"payee"`
	Category       *string `json:"category"`
	Amount         string  `json:"amount"`
	Baseline       string  `json:"baseline"`
	Usual          string  `json:"usual"`
	NativeCurrency string  `json:"native_currency"`
	NativeAmount   string  `json:"native_amount"`
	NativeUsual    string  `json:"native_usual"`
	Earlier        int     `json:"earlier"`
	Times          float64 `json:"times"`
}

// tenthsPerMultiple turns a multiple in tenths into the multiple.
const tenthsPerMultiple = 10.0

// renderAnomaliesJSON renders a as anomalies' --json document with warnings; every array is []
// rather than null when a holds none.
func renderAnomaliesJSON(a report.Anomalies, warnings []string) ([]byte, error) {
	listed := make([]anomalyDocument, len(a.Listed))
	for i, an := range a.Listed {
		listed[i] = anomalyEntry(an)
	}
	return marshalDocument(anomaliesDocument{
		Since:         a.Window.Since.Format(jsonDateLayout),
		Until:         a.Window.Until.Format(jsonDateLayout),
		Currency:      a.Currency.String(),
		AccountFilter: accountFilterDocuments(a.Accounts),
		Anomalies:     listed,
		Checked:       a.Checked,
		NotJudged:     a.NotJudged,
		Warnings:      warnings,
	})
}

// anomalyEntry is the document entry for an.
func anomalyEntry(an report.Anomaly) anomalyDocument {
	var category *string
	if an.Category != nil && an.ExpenseSplits <= 1 {
		category = &an.Category.Path
	}
	currency, amount, usual := an.Currency, an.Amount, an.Usual
	if an.ListedCurrency != "" {
		currency, amount, usual = an.ListedCurrency, an.ListedAmount, an.ListedUsual
	}
	return anomalyDocument{
		TransactionID:  an.TransactionID,
		Date:           an.Date.Format(jsonDateLayout),
		AccountID:      an.Account.ID,
		Account:        an.Account.Name,
		Currency:       currency,
		Payee:          an.Payee,
		Category:       category,
		Amount:         jsonMoney(amount),
		Baseline:       anomaliesBaselineWord[an.Baseline],
		Usual:          jsonMoney(usual),
		NativeCurrency: an.Currency,
		NativeAmount:   jsonMoney(an.Amount),
		NativeUsual:    jsonMoney(an.Usual),
		Earlier:        an.Earlier,
		Times:          float64(an.TimesTenths) / tenthsPerMultiple,
	}
}
