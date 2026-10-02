package cli

import (
	"fmt"
	"time"

	"github.com/koblas/quarry/internal/platform/humanize"
	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/report"
)

// anomaliesAligns is the alignment of the anomalies table's columns: Amount, Usual and Times right, the rest left.
var anomaliesAligns = []tableAlign{
	alignLeft, alignLeft, alignLeft, alignLeft, alignRight, alignRight, alignRight, alignLeft,
}

// anomaliesBaselineWord is the word of each baseline in the Compared with cell.
var anomaliesBaselineWord = map[report.AnomalyBaseline]string{
	report.BaselinePayee:    "payee",
	report.BaselineCategory: "category",
}

// renderAnomalies renders a as the anomalies table (caption, header, one row per listed charge), a
// blank line and the footer.
func renderAnomalies(a report.Anomalies) string {
	rows := make([][]string, 0, 1+len(a.Listed))
	rows = append(rows, []string{"Date", "Account", "Payee", "Category", "Amount", "Usual", "Times", "Compared with"})
	for _, an := range a.Listed {
		payee := ""
		if an.Payee != nil {
			payee = *an.Payee
		}
		var path *string
		if an.Category != nil {
			path = &an.Category.Path
		}
		amount, usual := anomalyMoney(an, a.Currency)
		rows = append(rows, []string{
			an.Date.Format(time.DateOnly),
			accountLabel(an.Account.Name, an.Account.Currency, an.Account.Closed, an.Account.Active),
			payeeLabel(payee),
			categoryText(an.ExpenseSplits, path),
			amount,
			usual,
			timesCell(an.TimesTenths),
			fmt.Sprintf("%s, %s earlier", anomaliesBaselineWord[an.Baseline], humanize.Thousands(an.Earlier)),
		})
	}
	return renderTable(windowCaption("Unusually large charges", a.Window, a.Accounts, a.Currency), anomaliesAligns, rows) +
		"\n" + anomaliesFooter(a.Checked, a.NotJudged) + "\n"
}

// anomalyMoney is the Amount and Usual cells of an in a report in target: converted when an was, else in its
// own currency, with its code in front when that is not target.
func anomalyMoney(an report.Anomaly, target money.Currency) (amount, usual string) {
	if an.ListedCurrency != "" {
		return formatMoney(an.ListedAmount), formatMoney(an.ListedUsual)
	}
	amount, usual = formatMoney(an.Amount), formatMoney(an.Usual)
	if target != money.Native && an.Currency != target.String() {
		return an.Currency + " " + amount, an.Currency + " " + usual
	}
	return amount, usual
}

// timesCell is tenths as a multiple with one decimal: 43 is "4.3x".
func timesCell(tenths int64) string {
	return fmt.Sprintf("%d.%dx", tenths/10, tenths%10)
}

// anomaliesFooter is "N charges checked", then "; M had too little history to judge" when any did.
func anomaliesFooter(checked, notJudged int) string {
	line := humanize.Count(checked, "charge", "charges") + " checked"
	if notJudged > 0 {
		line += "; " + humanize.Thousands(notJudged) + " had too little history to judge"
	}
	return line
}
