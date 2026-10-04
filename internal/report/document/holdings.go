package document

import (
	"fmt"
	"math/big"
	"time"

	"github.com/koblas/quarry/internal/platform/humanize"
	"github.com/koblas/quarry/internal/report"
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
		AccountFilter: NewAccountFilters(nil),
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
// So far it carries the no-price line; the other lines join it in their own slots as they are built.
func HoldingsWarnings(h report.Holdings) []string {
	warnings := []string{}
	if line, ok := noPriceWarning(h); ok {
		warnings = append(warnings, line)
	}
	return warnings
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
