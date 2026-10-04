package document

import (
	"cmp"
	"fmt"
	"math/big"
	"slices"
	"strings"
	"time"

	"github.com/koblas/quarry/internal/platform/humanize"
	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
)

// bigCentsPerUnit is how many cents make one whole unit of currency.
var bigCentsPerUnit = big.NewInt(100)

// BigMoney renders cents, of any size, as a 2-decimal amount with a leading "-" for a negative
// value and no thousands grouping; it is Money for a value that can pass 64 bits.
func BigMoney(cents *big.Int) string {
	// QuoRem truncates toward zero; DivMod would split -5 cents as -1 and 95.
	whole, fraction := new(big.Int).QuoRem(new(big.Int).Abs(cents), bigCentsPerUnit, new(big.Int))
	s := fmt.Sprintf("%s.%02d", whole, fraction.Int64())
	if cents.Sign() < 0 {
		return "-" + s
	}
	return s
}

// Holdings is holdings's --json document.
type Holdings struct {
	AsOf          string          `json:"as_of"`
	Currency      string          `json:"currency"`
	AccountFilter []AccountFilter `json:"account_filter"`
	Holdings      []Holding       `json:"holdings"`
	Totals        []HoldingsTotal `json:"totals"`
	Warnings      []string        `json:"warnings"`
}

// Holding is one entry of "holdings"; every field after shares is null when the store has no value for it.
type Holding struct {
	AccountID      string  `json:"account_id"`
	Account        string  `json:"account"`
	AccountClosed  bool    `json:"account_closed"`
	SecurityID     string  `json:"security_id"`
	Security       *string `json:"security"`
	Ticker         *string `json:"ticker"`
	Shares         string  `json:"shares"`
	Price          *string `json:"price"`
	PriceDate      *string `json:"price_date"`
	Currency       *string `json:"currency"`
	Value          *string `json:"value"`
	ConvertedValue *string `json:"converted_value"`
}

// HoldingsTotal is one entry of "totals".
type HoldingsTotal struct {
	Currency string `json:"currency"`
	Value    string `json:"value"`
}

// NewHoldings converts h into holdings's document with warnings;
// holdings, totals and account_filter are [] rather than null when h holds none.
func NewHoldings(h report.Holdings, warnings []string) Holdings {
	rows := make([]Holding, len(h.Rows))
	for i, r := range h.Rows {
		rows[i] = Holding{
			AccountID: r.AccountID, Account: r.Account, AccountClosed: r.AccountClosed,
			SecurityID: r.SecurityID, Security: r.Security, Ticker: r.Ticker,
			Shares:         Shares(r.Shares),
			Price:          nullable(r.Price, Shares),
			PriceDate:      nullable(r.PriceDate, func(d time.Time) string { return d.Format(DateLayout) }),
			Currency:       r.Currency,
			Value:          nullableMoney(r.Value),
			ConvertedValue: nullableMoney(h.Converted(r)),
		}
	}
	totals := make([]HoldingsTotal, len(h.Totals))
	for i, t := range h.Totals {
		totals[i] = HoldingsTotal{Currency: t.Currency, Value: BigMoney(t.Value)}
	}
	return Holdings{
		AsOf:          h.AsOf.Format(DateLayout),
		Currency:      h.Currency.String(),
		AccountFilter: NewAccountFilters(h.Accounts),
		Holdings:      rows,
		Totals:        totals,
		Warnings:      append([]string{}, warnings...),
	}
}

// nullable is format applied to v's referent, or nil when v is nil.
func nullable[T any](v *T, format func(T) string) *string {
	if v == nil {
		return nil
	}
	s := format(*v)
	return &s
}

// nullableMoney is cents as BigMoney, or nil when cents is nil.
func nullableMoney(cents *big.Int) *string {
	if cents == nil {
		return nil
	}
	s := BigMoney(cents)
	return &s
}

// HoldingsWarnings is the unprefixed warning lines for h, in the order holdings prints them; never nil.
// Slots: named non-investment accounts, nothing held, no price, no rate, no currency, other currency.
func HoldingsWarnings(h report.Holdings) []string {
	warnings := append(nonInvestmentWarnings(h), emptyWarnings(h)...)
	if line, ok := noPriceWarning(h); ok {
		warnings = append(warnings, line)
	}
	warnings = append(warnings, noRateWarnings(h)...)
	warnings = append(warnings, noCurrencyWarnings(h)...)
	return append(warnings, otherCurrencyWarnings(h)...)
}

// nonInvestmentWarnings is one line per named account that is not a brokerage or retirement account, in
// the holdings table's account order; non-nil, so it seeds HoldingsWarnings.
func nonInvestmentWarnings(h report.Holdings) []string {
	var named []store.Account
	for _, a := range h.Accounts {
		if !store.IsInvestmentAccount(a.Type) {
			named = append(named, a)
		}
	}
	// The same key as the holdings query's ORDER BY: plain name, then source id, then id.
	slices.SortFunc(named, func(a, b store.Account) int {
		return cmp.Or(strings.Compare(a.Name, b.Name), cmp.Compare(a.SourceID, b.SourceID), strings.Compare(a.ID, b.ID))
	})
	lines := make([]string, len(named))
	for i, a := range named {
		lines[i] = fmt.Sprintf("account %q is not a brokerage or retirement account, so it has no holdings", a.Name)
	}
	return lines
}

// emptyWarnings is one line when h has no rows: where the investment transactions in the accounts read start
// and end, or that there are none; none when something is held.
func emptyWarnings(h report.Holdings) []string {
	if len(h.Rows) != 0 {
		return nil
	}
	whose, they := "the store's", "the store has"
	scope := ""
	if h.Accounts != nil {
		whose, they, scope = "their", "they have", " in the named accounts"
	}
	asOf := h.AsOf.Format(DateLayout)
	if h.FirstTransaction.IsZero() {
		return []string{fmt.Sprintf("no holdings on %s%s; %s no investment transactions", asOf, scope, they)}
	}
	return []string{fmt.Sprintf("no holdings on %s%s; %s investment transactions run %s to %s",
		asOf, scope, whose, h.FirstTransaction.Format(DateLayout), h.LastTransaction.Format(DateLayout))}
}

// holdingsNoRatesWarning is the line for a store with no exchange rates at all.
const holdingsNoRatesWarning = "the store has no exchange rates, so values are listed in each security's own currency; " +
	"run quarry sync to fetch them"

// noRateWarnings is one line when some rows need a rate: that the store has no rates, or that the rows
// are valued before its first one; none when no row needs one.
func noRateWarnings(h report.Holdings) []string {
	needing := 0
	for _, r := range h.Rows {
		if h.NeedsRate(r) {
			needing++
		}
	}
	if needing == 0 {
		return nil
	}
	if h.FirstRate.IsZero() {
		return []string{holdingsNoRatesWarning}
	}
	be := "are"
	if needing == 1 {
		be = "is"
	}
	return []string{fmt.Sprintf("%s valued on %s, before %s, the first exchange rate in the store, %s not converted to %s and %s totalled in %s",
		humanize.Count(needing, "holding", "holdings"), h.AsOf.Format(DateLayout), h.FirstRate.Format(DateLayout),
		be, h.Currency, be, money.NativeOf(h.Currency))}
}

// noCurrencyWarnings is one line per security with no currency, priced or not, in every mode.
func noCurrencyWarnings(h report.Holdings) []string {
	return perSecurity(h,
		func(r store.Holding) bool { return r.Currency == nil },
		func(name string, _ store.Holding) string {
			return fmt.Sprintf("%q has no currency in Quicken, so quarry leaves its value out of the total; "+
				"set its currency in Quicken, then run quarry sync", name)
		})
}

// otherCurrencyWarnings is one line per priced security in a currency quarry does not convert; a native
// listing totals it as it is, so it has none.
func otherCurrencyWarnings(h report.Holdings) []string {
	if h.Currency == money.Native {
		return nil
	}
	return perSecurity(h,
		func(r store.Holding) bool { return r.Price != nil && r.Currency != nil && !report.Convertible(r) },
		func(name string, r store.Holding) string {
			return fmt.Sprintf("%q is priced in %s, which quarry does not convert, so its value is left out of the total",
				name, *r.Currency)
		})
}

// perSecurity is line for the first row of each distinct security among the rows that qualify, in row order.
// A security with no name is called by its id.
func perSecurity(h report.Holdings, qualifies func(store.Holding) bool, line func(name string, r store.Holding) string) []string {
	var lines []string
	seen := map[string]bool{}
	for _, r := range h.Rows {
		if !qualifies(r) || seen[r.SecurityID] {
			continue
		}
		seen[r.SecurityID] = true
		name := r.SecurityID
		if r.Security != nil {
			name = *r.Security
		}
		lines = append(lines, line(name, r))
	}
	return lines
}

// noPriceWarning is the line that some of h's rows have no price on or before its as-of day, so they are
// left out of the total; false when every row has one. A priced zero counts as priced.
func noPriceWarning(h report.Holdings) (string, bool) {
	unpriced := 0
	for _, r := range h.Rows {
		if r.Price == nil {
			unpriced++
		}
	}
	if unpriced == 0 {
		return "", false
	}
	clause, remedy := "it has no value and is left out", "for it"
	if unpriced != 1 {
		clause, remedy = "they have no value and are left out", "for each"
	}
	return fmt.Sprintf("%s no price on or before %s, so %s of the total; enter a price %s in Quicken, then run quarry sync",
		humanize.Count(unpriced, "holding has", "holdings have"), h.AsOf.Format(DateLayout), clause, remedy), true
}
