package document

import "github.com/koblas/quarry/internal/report"

// Anomalies is anomalies' --json document and the anomalies tool's structured result;
// Currency is the reporting currency ("native" lists every anomaly in its own).
type Anomalies struct {
	Since         string          `json:"since"`
	Until         string          `json:"until"`
	Currency      string          `json:"currency"`
	AccountFilter []AccountFilter `json:"account_filter"`
	Anomalies     []Anomaly       `json:"anomalies"`
	Checked       int             `json:"checked"`
	NotJudged     int             `json:"not_judged"`
	Warnings      []string        `json:"warnings"`
}

// Anomaly is one entry of "anomalies"; Payee is null for a charge with no payee, Category for an uncategorized
// or split charge. Currency, Amount and Usual are in the reporting currency unless the charge could not be
// converted; the Native fields are always its own, and Times is the charge as a multiple of NativeUsual.
type Anomaly struct {
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

// baselineWords is the word of each baseline.
var baselineWords = map[report.AnomalyBaseline]string{
	report.BaselinePayee:    "payee",
	report.BaselineCategory: "category",
}

// BaselineWord is the word for the baseline a charge was compared with, shared by
// the Compared with column and the document's "baseline"; "" for one it does not know.
func BaselineWord(b report.AnomalyBaseline) string {
	return baselineWords[b]
}

// tenthsPerMultiple turns a multiple in tenths into the multiple.
const tenthsPerMultiple = 10.0

// NewAnomalies converts a into anomalies' document with warnings; every array is []
// rather than null when a holds none.
func NewAnomalies(a report.Anomalies, warnings []string) Anomalies {
	listed := anomalyEntries(a)
	return Anomalies{
		Since:         a.Window.Since.Format(DateLayout),
		Until:         a.Window.Until.Format(DateLayout),
		Currency:      a.Currency.String(),
		AccountFilter: NewAccountFilters(a.Accounts),
		Anomalies:     listed,
		Checked:       a.Checked,
		NotJudged:     a.NotJudged,
		Warnings:      append([]string{}, warnings...),
	}
}

// anomalyEntries is a's listed anomalies as document entries, [] when there are none.
func anomalyEntries(a report.Anomalies) []Anomaly {
	listed := make([]Anomaly, len(a.Listed))
	for i, an := range a.Listed {
		listed[i] = anomalyEntry(an)
	}
	return listed
}

// anomalyEntry is the document entry for an.
func anomalyEntry(an report.Anomaly) Anomaly {
	var category *string
	if an.Category != nil && an.ExpenseSplits <= 1 {
		category = &an.Category.Path
	}
	currency, amount, usual := an.Currency, an.Amount, an.Usual
	if an.ListedCurrency != "" {
		currency, amount, usual = an.ListedCurrency, an.ListedAmount, an.ListedUsual
	}
	return Anomaly{
		TransactionID:  an.TransactionID,
		Date:           an.Date.Format(DateLayout),
		AccountID:      an.Account.ID,
		Account:        an.Account.Name,
		Currency:       currency,
		Payee:          an.Payee,
		Category:       category,
		Amount:         Money(amount),
		Baseline:       BaselineWord(an.Baseline),
		Usual:          Money(usual),
		NativeCurrency: an.Currency,
		NativeAmount:   Money(an.Amount),
		NativeUsual:    Money(an.Usual),
		Earlier:        an.Earlier,
		Times:          float64(an.TimesTenths) / tenthsPerMultiple,
	}
}
