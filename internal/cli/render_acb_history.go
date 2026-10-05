package cli

import (
	"strings"
	"time"

	"github.com/koblas/quarry/internal/platform/humanize"
	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/report/document"
	"github.com/koblas/quarry/internal/store"
)

// renderACBHistory is the full history of each security a names, in a.SelectedIDs' order: one captioned table
// each, a blank line between. A security held only in registered accounts has no event, so its table is the
// caption and header alone.
func renderACBHistory(a report.ACB) string {
	events := make(map[string][]report.ACBEvent, len(a.Securities))
	named := make(map[string]store.Security, len(a.Securities)+len(a.RegisteredOnly))
	for _, position := range a.Securities {
		events[position.Security.ID] = position.Events
		named[position.Security.ID] = position.Security
	}
	for _, security := range a.RegisteredOnly {
		named[security.ID] = security
	}
	marked := markedSaleIDs(a)

	blocks := make([]string, 0, len(a.SelectedIDs))
	for _, id := range a.SelectedIDs {
		blocks = append(blocks, renderACBSecurityHistory(named[id], events[id], marked))
	}

	return strings.Join(blocks, "\n")
}

// markedSaleIDs is the ids of the sales a marks as possible superficial losses.
func markedSaleIDs(a report.ACB) map[string]bool {
	marked := make(map[string]bool)
	for _, year := range a.Years {
		for _, sale := range year.Sales {
			if sale.PossibleSuperficialLoss {
				marked[sale.ID] = true
			}
		}
	}

	return marked
}

// renderACBSecurityHistory is one security's table: a row per event, a last, unheaded column carrying the notes
// on it. The CAD and Gain cells of an event quarry could not value are blank, as is the Rate of one with none.
func renderACBSecurityHistory(security store.Security, events []report.ACBEvent, marked map[string]bool) string {
	rows := make([][]string, 0, 1+len(events))
	rows = append(rows, []string{"Date", "Account", "Action", "Shares", "Amount", "Rate", "CAD", "Shares held", "ACB", "Gain or loss", ""})
	for _, e := range events {
		rows = append(rows, acbHistoryRow(e, marked[e.ID]))
	}

	return renderTable(acbHistoryCaption(security),
		[]tableAlign{
			alignLeft, alignLeft, alignLeft, alignRight, alignRight, alignRight, alignRight, alignRight, alignRight, alignRight, alignLeft,
		}, rows)
}

// acbHistoryCaption names the security by its name, and its ticker when it has one.
func acbHistoryCaption(security store.Security) string {
	caption := `ACB history of "` + escapeCell(security.Name) + `"`
	if security.Ticker != nil && *security.Ticker != "" {
		caption += " (" + escapeCell(*security.Ticker) + ")"
	}

	return caption + ", in " + money.CAD.String()
}

// acbHistoryRow is e as one row. An adjustment (an event with no amount) has no account, shares, amount or rate.
func acbHistoryRow(e report.ACBEvent, superficial bool) []string {
	var shares, amount, rate, cad, gain string
	if e.Amount != nil {
		shares = humanize.Shares(report.Millionths(e.Shares))
		amount = formatMoney(*e.Amount) + " " + e.Currency
	}
	if e.Rate != 0 {
		rate = document.Rate(e.Rate)
	}
	if !e.Unvalued {
		cad = formatMoney(e.CAD)
	}
	if e.Realized && !e.Unvalued {
		gain = formatMoney(e.Gain)
	}

	return []string{
		e.Date.Format(time.DateOnly), escapeCell(e.Account), e.Action, shares, amount, rate, cad,
		humanize.Shares(report.Millionths(e.Held)), formatMoney(e.ACB), gain,
		strings.Join(acbHistoryNotes(e, superficial), ", "),
	}
}

// acbHistoryNotes are the notes after an event's row, in the ruled order.
func acbHistoryNotes(e report.ACBEvent, superficial bool) []string {
	var notes []string
	if superficial {
		notes = append(notes, "possible superficial loss")
	}
	if e.UnknownCost {
		notes = append(notes, "unknown cost")
	}

	return notes
}
