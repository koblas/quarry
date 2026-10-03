package cli

import (
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/report/document"
)

// recurringDocument is recurring's --json stdout shape; Currency is the reporting currency
// ("native" lists every series in its own).
type recurringDocument struct {
	Since         string                    `json:"since"`
	Until         string                    `json:"until"`
	Currency      string                    `json:"currency"`
	AccountFilter []document.AccountFilter  `json:"account_filter"`
	Series        []recurringSeriesDocument `json:"series"`
	Totals        []recurringTotalDocument  `json:"totals"`
	Warnings      []string                  `json:"warnings"`
}

// recurringSeriesDocument is one entry of "series"; PerYear is null when ended, PayeeKey when grouped by payee id.
// The Native fields are the series' own; the rest are in the reporting currency unless unconverted.
type recurringSeriesDocument struct {
	Payee             string                         `json:"payee"`
	PayeeKey          *string                        `json:"payee_key"`
	Payees            []recurringPayeeDocument       `json:"payees"`
	Currency          string                         `json:"currency"`
	Cadence           string                         `json:"cadence"`
	Amount            string                         `json:"amount"`
	FirstAmount       string                         `json:"first_amount"`
	PerYear           *string                        `json:"per_year"`
	NativeCurrency    string                         `json:"native_currency"`
	NativeAmount      string                         `json:"native_amount"`
	NativeFirstAmount string                         `json:"native_first_amount"`
	FirstCharge       string                         `json:"first_charge"`
	LastCharge        string                         `json:"last_charge"`
	ChargeCount       int                            `json:"charge_count"`
	State             string                         `json:"state"`
	New               bool                           `json:"new"`
	Accounts          []document.AccountFilter       `json:"accounts"`
	PriceChanges      []recurringPriceChangeDocument `json:"price_changes"`
}

// recurringPayeeDocument is one entry of a series' "payees".
type recurringPayeeDocument struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// recurringPriceChangeDocument is one entry of a series' "price_changes"; Date is the later charge's
// and Currency the series' own currency, the one From and To are in.
type recurringPriceChangeDocument struct {
	Date      string  `json:"date"`
	Currency  string  `json:"currency"`
	From      string  `json:"from"`
	To        string  `json:"to"`
	ChangePct float64 `json:"change_pct"`
}

// recurringTotalDocument is one entry of "totals".
type recurringTotalDocument struct {
	Currency string `json:"currency"`
	PerYear  string `json:"per_year"`
}

// recurringCadences is the "cadence" word of each cadence.
var recurringCadences = map[report.Cadence]string{
	report.CadenceWeekly:    "weekly",
	report.CadenceMonthly:   "monthly",
	report.CadenceQuarterly: "quarterly",
	report.CadenceAnnual:    "annual",
}

// tenthsPerPercent turns a change in tenths of a percent into the percent.
const tenthsPerPercent = 10.0

// renderRecurringJSON renders r as recurring's --json document with warnings; every array is []
// rather than null when r holds none.
func renderRecurringJSON(r report.Recurring, warnings []string) ([]byte, error) {
	series := make([]recurringSeriesDocument, len(r.Series))
	for i, s := range r.Series {
		series[i] = recurringSeriesOf(s)
	}
	totals := make([]recurringTotalDocument, len(r.Totals))
	for i, t := range r.Totals {
		totals[i] = recurringTotalDocument{Currency: t.Currency, PerYear: document.Money(t.PerYear)}
	}
	return marshalDocument(recurringDocument{
		Since:         r.Window.Since.Format(document.DateLayout),
		Until:         r.Window.Until.Format(document.DateLayout),
		Currency:      r.Currency.String(),
		AccountFilter: document.NewAccountFilters(r.Accounts),
		Series:        series,
		Totals:        totals,
		Warnings:      warnings,
	})
}

// recurringSeriesOf is s as one entry of the document's "series".
func recurringSeriesOf(s report.Series) recurringSeriesDocument {
	payees := make([]recurringPayeeDocument, len(s.Payees))
	for i, p := range s.Payees {
		payees[i] = recurringPayeeDocument{ID: p.ID, Name: p.Name}
	}
	changes := make([]recurringPriceChangeDocument, len(s.PriceChanges))
	for i, c := range s.PriceChanges {
		changes[i] = recurringPriceChangeDocument{
			Date: c.Date.Format(document.DateLayout), Currency: s.NativeCurrency, From: document.Money(c.From), To: document.Money(c.To),
			ChangePct: float64(c.Tenths) / tenthsPerPercent,
		}
	}
	var perYear *string
	if s.PerYear != nil {
		perYear = new(document.Money(*s.PerYear))
	}
	return recurringSeriesDocument{
		Payee: s.Payee, PayeeKey: s.PayeeKey, Payees: payees,
		Currency: s.Currency, Cadence: recurringCadences[s.Cadence],
		Amount: document.Money(s.Amount), FirstAmount: document.Money(s.FirstAmount), PerYear: perYear,
		NativeCurrency: s.NativeCurrency, NativeAmount: document.Money(s.NativeAmount), NativeFirstAmount: document.Money(s.NativeFirstAmount),
		FirstCharge: s.First.Format(document.DateLayout), LastCharge: s.Last.Format(document.DateLayout),
		ChargeCount: s.ChargeCount, State: recurringStatus[s.State], New: s.New,
		Accounts: document.NewAccountFilters(s.Accounts), PriceChanges: changes,
	}
}
