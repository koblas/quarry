package document

import (
	"fmt"
	"math/big"
	"strings"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
)

// perShareDecimals is the decimals of an ACB per share.
const perShareDecimals = 4

// minRateDecimals is the fewest decimals a usd_cad rate shows.
const minRateDecimals = 4

// ACB is acb's --json document. Year is the tax year it is cut to, null for the whole report.
type ACB struct {
	AsOf       string        `json:"as_of"`
	Currency   string        `json:"currency"`
	Year       *int          `json:"year"`
	Years      []ACBYear     `json:"years"`
	Securities []ACBSecurity `json:"securities"`
	Warnings   []string      `json:"warnings"`
}

// ACBYear is one tax year of "years": its totals in CAD, the flagged sales among them, and the sales.
// Gain is the sales' alone; ReturnOfCapitalGain is the returns of capital above the ACB, "0.00" when none.
type ACBYear struct {
	Year                      int       `json:"year"`
	SaleCount                 int       `json:"sale_count"`
	Proceeds                  string    `json:"proceeds"`
	Outlays                   string    `json:"outlays"`
	ACB                       string    `json:"acb"`
	Gain                      string    `json:"gain"`
	ReturnOfCapitalGain       string    `json:"return_of_capital_gain"`
	PossibleSuperficialLosses int       `json:"possible_superficial_losses"`
	UnknownCostSales          int       `json:"unknown_cost_sales"`
	Sales                     []ACBSale `json:"sales"`
}

// ACBSale is one sale of a year, in CAD; its shares are the units sold.
type ACBSale struct {
	Date                    string  `json:"date"`
	InvestmentTransactionID string  `json:"investment_transaction_id"`
	SecurityID              string  `json:"security_id"`
	Security                string  `json:"security"`
	Ticker                  *string `json:"ticker"`
	AccountID               string  `json:"account_id"`
	Account                 string  `json:"account"`
	Shares                  string  `json:"shares"`
	Proceeds                string  `json:"proceeds"`
	Outlays                 string  `json:"outlays"`
	ACB                     string  `json:"acb"`
	Gain                    string  `json:"gain"`
	PossibleSuperficialLoss bool    `json:"possible_superficial_loss"`
	UnknownCost             bool    `json:"unknown_cost"`
}

// ACBSecurity is one security of "securities": its position after its last event, and every event.
// ACBPerShare is null with no shares held.
type ACBSecurity struct {
	SecurityID  string     `json:"security_id"`
	Security    string     `json:"security"`
	Ticker      *string    `json:"ticker"`
	Currency    *string    `json:"currency"`
	Shares      string     `json:"shares"`
	ACB         string     `json:"acb"`
	ACBPerShare *string    `json:"acb_per_share"`
	Incomplete  bool       `json:"incomplete"`
	Events      []ACBEvent `json:"events"`
}

// ACBEvent is one event of a security's history. Amount and AmountCurrency are the transaction's own, USDCAD its
// rate when it was in USD with one on file; CAD is the amount at that rate. Outlays and Gain are null off a sale;
// CAD and Gain are null on an event whose amount quarry could not convert. UnknownCost marks shares moved with no
// recorded cost.
type ACBEvent struct {
	Date                    string  `json:"date"`
	InvestmentTransactionID *string `json:"investment_transaction_id"`
	AccountID               *string `json:"account_id"`
	Account                 *string `json:"account"`
	Action                  string  `json:"action"`
	Shares                  string  `json:"shares"`
	Amount                  *string `json:"amount"`
	AmountCurrency          *string `json:"amount_currency"`
	USDCAD                  *string `json:"usd_cad"`
	CAD                     *string `json:"cad"`
	Outlays                 *string `json:"outlays"`
	SharesHeld              string  `json:"shares_held"`
	ACB                     string  `json:"acb"`
	Gain                    *string `json:"gain"`
	UnknownCost             bool    `json:"unknown_cost"`
}

// NewACB converts a into acb's document with warnings; every array is [] rather than null when empty. A report cut
// to a year (report.ACB.InYear) names it in "year".
func NewACB(a report.ACB, warnings []string) ACB {
	securities := make(map[string]store.Security, len(a.Securities))
	for _, s := range a.Securities {
		securities[s.Security.ID] = s.Security
	}

	years := make([]ACBYear, len(a.Years))
	for i, year := range a.Years {
		years[i] = newACBYear(year, securities)
	}
	positions := make([]ACBSecurity, len(a.Securities))
	for i, s := range a.Securities {
		positions[i] = newACBSecurity(s)
	}

	var year *int
	if a.Year != 0 {
		year = &a.Year
	}

	return ACB{
		AsOf:       a.AsOf.Format(DateLayout),
		Currency:   money.CAD.String(),
		Year:       year,
		Years:      years,
		Securities: positions,
		Warnings:   append([]string{}, warnings...),
	}
}

// newACBYear is year as its document, each sale named from securities.
func newACBYear(year report.ACBYear, securities map[string]store.Security) ACBYear {
	out := ACBYear{
		Year: year.Year, SaleCount: len(year.Sales),
		Proceeds: Money(year.Proceeds), Outlays: Money(year.Outlays), ACB: Money(year.ACBRemoved), Gain: Money(year.Gain),
		ReturnOfCapitalGain: Money(year.ReturnOfCapitalGain), PossibleSuperficialLosses: year.PossibleSuperficialLosses(),
		UnknownCostSales: year.UnknownCostSales(),
		Sales:            make([]ACBSale, len(year.Sales)),
	}
	for i, sale := range year.Sales {
		security := securities[sale.SecurityID]
		out.Sales[i] = ACBSale{
			Date: sale.Date.Format(DateLayout), InvestmentTransactionID: sale.ID,
			SecurityID: sale.SecurityID, Security: security.Name, Ticker: security.Ticker,
			AccountID: sale.AccountID, Account: sale.Account,
			Shares: Shares(report.Millionths(sale.Shares)), Proceeds: Money(sale.Proceeds), Outlays: Money(sale.Outlays),
			ACB: Money(sale.ACBRemoved), Gain: Money(sale.Gain),
			PossibleSuperficialLoss: sale.PossibleSuperficialLoss, UnknownCost: sale.UnknownCost,
		}
	}

	return out
}

// newACBSecurity is s as its document.
func newACBSecurity(s report.ACBSecurity) ACBSecurity {
	events := make([]ACBEvent, len(s.Events))
	for i, event := range s.Events {
		events[i] = newACBEvent(event)
	}

	return ACBSecurity{
		SecurityID: s.Security.ID, Security: s.Security.Name, Ticker: s.Security.Ticker, Currency: s.Security.Currency,
		Shares: Shares(report.Millionths(s.Shares)), ACB: Money(s.ACB),
		ACBPerShare: perShare(s.PerShare()),
		Incomplete:  s.Incomplete, Events: events,
	}
}

// perShare is r, an ACB per share in dollars, to perShareDecimals decimals rounded half away from zero; nil for nil.
func perShare(r *big.Rat) *string {
	if r == nil {
		return nil
	}
	s := r.FloatString(perShareDecimals)

	return &s
}

// newACBEvent is e as its document; a field e does not carry is null.
func newACBEvent(e report.ACBEvent) ACBEvent {
	out := ACBEvent{
		Date: e.Date.Format(DateLayout), InvestmentTransactionID: NullString(e.ID),
		AccountID: NullString(e.AccountID), Account: NullString(e.Account),
		Action: e.Action, Shares: Shares(report.Millionths(e.Shares)),
		Amount: nullable(e.Amount, Money), Outlays: nullable(e.Outlays, Money),
		SharesHeld: Shares(report.Millionths(e.Held)), ACB: Money(e.ACB),
		UnknownCost: e.UnknownCost,
	}
	if e.Amount != nil {
		out.AmountCurrency = NullString(e.Currency)
	}
	if e.Rate != 0 {
		rate := Rate(e.Rate)
		out.USDCAD = &rate
	}
	if !e.Unvalued {
		cad := Money(e.CAD)
		out.CAD = &cad
	}
	if e.Realized && !e.Unvalued {
		gain := Money(e.Gain)
		out.Gain = &gain
	}

	return out
}

// Rate renders r, in millionths, with its trailing zeros trimmed to at least minRateDecimals decimals.
func Rate(r money.Rate) string {
	whole, fraction := int64(r)/1_000_000, int64(r)%1_000_000
	decimals := strings.TrimRight(fmt.Sprintf("%06d", fraction), "0")
	for len(decimals) < minRateDecimals {
		decimals += "0"
	}

	return fmt.Sprintf("%d.%s", whole, decimals)
}
