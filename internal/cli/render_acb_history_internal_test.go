// White-box: renderACBHistory's cell rules (blank cells, adjustment rows, notes, escaping) are unexported layout
// rules driven directly with hand-built reports, not through a command.
package cli

import (
	"fmt"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
)

// historyRow is one history table line with the first ten columns w wide, the last cell unheaded.
func historyRow(w [10]int, c [11]string) string {
	return strings.TrimRight(fmt.Sprintf("%-*s  %-*s  %-*s  %*s  %*s  %*s  %*s  %*s  %*s  %*s  %s",
		w[0], c[0], w[1], c[1], w[2], c[2], w[3], c[3], w[4], c[4], w[5], c[5], w[6], c[6], w[7], c[7], w[8], c[8], w[9], c[9], c[10]), " ") + "\n"
}

var historyHeader = [11]string{"Date", "Account", "Action", "Shares", "Amount", "Rate", "CAD", "Shares held", "ACB", "Gain or loss"}

// historyOf is a report naming the one security, with events, and the sale ids Years marks.
func historyOf(security store.Security, events []report.ACBEvent, markedSales ...string) report.ACB {
	sales := make([]report.ACBSale, 0, len(markedSales))
	for _, id := range markedSales {
		sales = append(sales, report.ACBSale{ID: id, PossibleSuperficialLoss: true})
	}

	return report.ACB{
		Selected: true, SelectedIDs: []string{security.ID},
		Years:      []report.ACBYear{{Year: 2025, Sales: sales}},
		Securities: []report.ACBSecurity{{Security: security, Events: events}},
	}
}

var historySecurity = store.Security{ID: "sec-1", Name: "Acme Corp", Ticker: new("ACME")}

func historyEvent(id, action string, cad int64) report.ACBEvent {
	return report.ACBEvent{
		ID: id, Date: utcDay(2025, time.March, 3), Account: "Margin", Action: action,
		Shares: big.NewRat(2, 1), Amount: new(int64(100)), Currency: "CAD", CAD: cad, Held: big.NewRat(8, 1), ACB: 500,
	}
}

func Test_renderACBHistory_leaves_the_cad_and_gain_cells_blank_on_an_event_quarry_could_not_value(t *testing.T) {
	unvalued := historyEvent("itxn-1", store.ActionSell, 0)
	unvalued.Currency, unvalued.Realized, unvalued.Gain, unvalued.Unvalued = "USD", true, -700, true
	w := [10]int{10, 7, 6, 6, 8, 4, 3, 11, 4, 12}

	got := renderACBHistory(historyOf(historySecurity, []report.ACBEvent{unvalued}))

	assert.Equal(t, "ACB history of \"Acme Corp\" (ACME), in CAD\n\n"+
		historyRow(w, historyHeader)+
		historyRow(w, [11]string{"2025-03-03", "Margin", "sell", "2", "1.00 USD", "", "", "8", "5.00", ""}), got)
}

func Test_renderACBHistory_prints_a_usd_events_rate_and_leaves_a_cad_events_blank(t *testing.T) {
	usd := historyEvent("itxn-1", store.ActionBuy, -125)
	usd.Currency, usd.Rate = "USD", 1_250_000
	cad := historyEvent("itxn-2", store.ActionBuy, -100)
	w := [10]int{10, 7, 6, 6, 8, 6, 5, 11, 4, 12}

	got := renderACBHistory(historyOf(historySecurity, []report.ACBEvent{usd, cad}))

	assert.Equal(t, "ACB history of \"Acme Corp\" (ACME), in CAD\n\n"+
		historyRow(w, historyHeader)+
		historyRow(w, [11]string{"2025-03-03", "Margin", "buy", "2", "1.00 USD", "1.2500", "-1.25", "8", "5.00", ""})+
		historyRow(w, [11]string{"2025-03-03", "Margin", "buy", "2", "1.00 CAD", "", "-1.00", "8", "5.00", ""}), got)
}

func Test_renderACBHistory_prints_an_adjustment_with_no_account_shares_amount_or_rate(t *testing.T) {
	adjustment := report.ACBEvent{
		Date: utcDay(2025, time.June, 30), Action: report.ACBActionReinvestedDistribution,
		Shares: new(big.Rat), CAD: -2_000, Held: big.NewRat(8, 1), ACB: 52_000,
	}
	w := [10]int{10, 7, 23, 6, 6, 4, 6, 11, 6, 12}

	got := renderACBHistory(historyOf(historySecurity, []report.ACBEvent{adjustment}))

	assert.Equal(t, "ACB history of \"Acme Corp\" (ACME), in CAD\n\n"+
		historyRow(w, historyHeader)+
		historyRow(w, [11]string{"2025-06-30", "", "reinvested distribution", "", "", "", "-20.00", "8", "520.00", ""}), got)
}

func Test_renderACBHistory_prints_the_excess_of_a_return_of_capital_above_the_acb_as_its_gain(t *testing.T) {
	excess := report.ACBEvent{
		Date: utcDay(2025, time.June, 30), Action: report.ACBActionReturnOfCapital,
		Shares: new(big.Rat), CAD: 23_000, Held: big.NewRat(8, 1), Realized: true, Gain: 12_500,
	}
	w := [10]int{10, 7, 17, 6, 6, 4, 6, 11, 4, 12}

	got := renderACBHistory(historyOf(historySecurity, []report.ACBEvent{excess}))

	assert.Equal(t, "ACB history of \"Acme Corp\" (ACME), in CAD\n\n"+
		historyRow(w, historyHeader)+
		historyRow(w, [11]string{"2025-06-30", "", "return of capital", "", "", "", "230.00", "8", "0.00", "125.00"}), got)
}

func Test_renderACBHistory_marks_a_sale_marked_a_possible_superficial_loss_and_leaves_another_unmarked(t *testing.T) {
	w := [10]int{10, 7, 6, 6, 8, 4, 4, 11, 4, 12}
	events := []report.ACBEvent{historyEvent("itxn-1", store.ActionSell, 100), historyEvent("itxn-2", store.ActionSell, 100)}

	got := renderACBHistory(historyOf(historySecurity, events, "itxn-2"))

	assert.Equal(t, "ACB history of \"Acme Corp\" (ACME), in CAD\n\n"+
		historyRow(w, historyHeader)+
		historyRow(w, [11]string{"2025-03-03", "Margin", "sell", "2", "1.00 CAD", "", "1.00", "8", "5.00", ""})+
		historyRow(w, [11]string{"2025-03-03", "Margin", "sell", "2", "1.00 CAD", "", "1.00", "8", "5.00", "", "possible superficial loss"}), got)
}

func Test_renderACBHistory_puts_the_possible_superficial_loss_before_unknown_cost(t *testing.T) {
	sell := historyEvent("itxn-1", store.ActionSell, 100)
	sell.UnknownCost = true
	w := [10]int{10, 7, 6, 6, 8, 4, 4, 11, 4, 12}

	got := renderACBHistory(historyOf(historySecurity, []report.ACBEvent{sell}, "itxn-1"))

	assert.Equal(t, "ACB history of \"Acme Corp\" (ACME), in CAD\n\n"+
		historyRow(w, historyHeader)+
		historyRow(w, [11]string{"2025-03-03", "Margin", "sell", "2", "1.00 CAD", "", "1.00", "8", "5.00", "", "possible superficial loss, unknown cost"}), got)
}

func Test_renderACBHistory_names_a_security_by_name_alone_when_it_has_no_ticker(t *testing.T) {
	cases := []struct {
		name   string
		ticker *string
	}{
		{name: "a nil ticker", ticker: nil},
		{name: "an empty ticker", ticker: new("")},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := renderACBHistory(historyOf(store.Security{ID: "sec-1", Name: "Plain Fund", Ticker: c.ticker}, nil))

			assert.True(t, strings.HasPrefix(got, "ACB history of \"Plain Fund\", in CAD\n\n"), got)
		})
	}
}

func Test_renderACBHistory_escapes_a_name_ticker_and_account_that_would_break_the_line(t *testing.T) {
	event := historyEvent("itxn-1", store.ActionBuy, -100)
	event.Account = "Odd\tAccount"

	got := renderACBHistory(historyOf(store.Security{ID: "sec-1", Name: "Odd\nFund", Ticker: new("O\rD")}, []report.ACBEvent{event}))

	assert.True(t, strings.HasPrefix(got, "ACB history of \"Odd\\nFund\" (O\\rD), in CAD\n\n"), got)
	assert.Contains(t, got, "Odd\\tAccount")
	assert.Equal(t, 4, strings.Count(got, "\n"))
}

func Test_renderACBHistory_prints_a_registered_only_security_as_its_caption_and_header_alone(t *testing.T) {
	a := report.ACB{Selected: true, SelectedIDs: []string{"sec-9"}, RegisteredOnly: []store.Security{{ID: "sec-9", Name: "Maple Fund"}}}

	got := renderACBHistory(a)

	assert.Equal(t, "ACB history of \"Maple Fund\", in CAD\n\n"+
		"Date  Account  Action  Shares  Amount  Rate  CAD  Shares held  ACB  Gain or loss\n", got)
}
