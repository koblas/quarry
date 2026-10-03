package document

import "github.com/koblas/quarry/internal/report"

// Recurring is recurring's --json document and the recurring_charges tool's structured result;
// Currency is the reporting currency ("native" lists every series in its own).
type Recurring struct {
	Since         string            `json:"since"`
	Until         string            `json:"until"`
	Currency      string            `json:"currency"`
	AccountFilter []AccountFilter   `json:"account_filter"`
	Series        []RecurringSeries `json:"series"`
	Totals        []RecurringTotal  `json:"totals"`
	Warnings      []string          `json:"warnings"`
}

// RecurringSeries is one entry of "series"; PerYear is null when ended, PayeeKey when grouped by payee id.
// The Native fields are the series' own; the rest are in the reporting currency unless unconverted.
type RecurringSeries struct {
	Payee             string                 `json:"payee"`
	PayeeKey          *string                `json:"payee_key"`
	Payees            []RecurringPayee       `json:"payees"`
	Currency          string                 `json:"currency"`
	Cadence           string                 `json:"cadence"`
	Amount            string                 `json:"amount"`
	FirstAmount       string                 `json:"first_amount"`
	PerYear           *string                `json:"per_year"`
	NativeCurrency    string                 `json:"native_currency"`
	NativeAmount      string                 `json:"native_amount"`
	NativeFirstAmount string                 `json:"native_first_amount"`
	FirstCharge       string                 `json:"first_charge"`
	LastCharge        string                 `json:"last_charge"`
	ChargeCount       int                    `json:"charge_count"`
	State             string                 `json:"state"`
	New               bool                   `json:"new"`
	Accounts          []AccountFilter        `json:"accounts"`
	PriceChanges      []RecurringPriceChange `json:"price_changes"`
}

// RecurringPayee is one entry of a series' "payees".
type RecurringPayee struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// RecurringPriceChange is one entry of a series' "price_changes"; Date is the later charge's
// and Currency the series' own currency, the one From and To are in.
type RecurringPriceChange struct {
	Date      string  `json:"date"`
	Currency  string  `json:"currency"`
	From      string  `json:"from"`
	To        string  `json:"to"`
	ChangePct float64 `json:"change_pct"`
}

// RecurringTotal is one entry of "totals".
type RecurringTotal struct {
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

// recurringStatus is the word of each series state.
var recurringStatus = map[report.SeriesState]string{
	report.SeriesActive: "active",
	report.SeriesEnded:  "ended",
}

// RecurringStatus is the word for a series state, shared by the Status column
// and the document's "state"; "" for a state it does not know.
func RecurringStatus(state report.SeriesState) string {
	return recurringStatus[state]
}

// tenthsPerPercent turns a change in tenths of a percent into the percent.
const tenthsPerPercent = 10.0

// NewRecurring converts r into recurring's document with warnings; every array
// is [] rather than null when r holds none.
func NewRecurring(r report.Recurring, warnings []string) Recurring {
	series := make([]RecurringSeries, len(r.Series))
	for i, s := range r.Series {
		series[i] = recurringSeriesOf(s)
	}
	totals := make([]RecurringTotal, len(r.Totals))
	for i, t := range r.Totals {
		totals[i] = RecurringTotal{Currency: t.Currency, PerYear: Money(t.PerYear)}
	}
	return Recurring{
		Since:         r.Window.Since.Format(DateLayout),
		Until:         r.Window.Until.Format(DateLayout),
		Currency:      r.Currency.String(),
		AccountFilter: NewAccountFilters(r.Accounts),
		Series:        series,
		Totals:        totals,
		Warnings:      append([]string{}, warnings...),
	}
}

// recurringSeriesOf is s as one entry of the document's "series".
func recurringSeriesOf(s report.Series) RecurringSeries {
	payees := make([]RecurringPayee, len(s.Payees))
	for i, p := range s.Payees {
		payees[i] = RecurringPayee{ID: p.ID, Name: p.Name}
	}
	changes := make([]RecurringPriceChange, len(s.PriceChanges))
	for i, c := range s.PriceChanges {
		changes[i] = RecurringPriceChange{
			Date: c.Date.Format(DateLayout), Currency: s.NativeCurrency, From: Money(c.From), To: Money(c.To),
			ChangePct: float64(c.Tenths) / tenthsPerPercent,
		}
	}
	var perYear *string
	if s.PerYear != nil {
		perYear = new(Money(*s.PerYear))
	}
	return RecurringSeries{
		Payee: s.Payee, PayeeKey: s.PayeeKey, Payees: payees,
		Currency: s.Currency, Cadence: recurringCadences[s.Cadence],
		Amount: Money(s.Amount), FirstAmount: Money(s.FirstAmount), PerYear: perYear,
		NativeCurrency: s.NativeCurrency, NativeAmount: Money(s.NativeAmount), NativeFirstAmount: Money(s.NativeFirstAmount),
		FirstCharge: s.First.Format(DateLayout), LastCharge: s.Last.Format(DateLayout),
		ChargeCount: s.ChargeCount, State: RecurringStatus(s.State), New: s.New,
		Accounts: NewAccountFilters(s.Accounts), PriceChanges: changes,
	}
}
