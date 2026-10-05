package document

import (
	"time"

	"github.com/koblas/quarry/internal/report"
)

// NetWorth is networth's --json document. AsOf, Since and Until are always present, null when the
// command was not asked for that kind of listing.
type NetWorth struct {
	AsOf     *string        `json:"as_of"`
	Since    *string        `json:"since"`
	Until    *string        `json:"until"`
	Currency string         `json:"currency"`
	Dates    []NetWorthDate `json:"dates"`
	Warnings []string       `json:"warnings"`
}

// NetWorthDate is one entry of "dates": the balances on one day, zero ones included, and their totals.
type NetWorthDate struct {
	Date     string            `json:"date"`
	Balances []NetWorthBalance `json:"balances"`
	Totals   []NetWorthTotal   `json:"totals"`
}

// NetWorthBalance is one entry of "balances"; converted_balance is null in a native listing and when no
// exchange rate converts the balance.
type NetWorthBalance struct {
	Type             string  `json:"type"`
	Currency         string  `json:"currency"`
	Balance          string  `json:"balance"`
	ConvertedBalance *string `json:"converted_balance"`
}

// NetWorthTotal is one entry of "totals".
type NetWorthTotal struct {
	Currency string `json:"currency"`
	Value    string `json:"value"`
}

// NewNetWorth converts n, a snapshot, into networth's document with warnings;
// dates, balances, totals and warnings are [] rather than null when n holds none.
func NewNetWorth(n report.NetWorth, warnings []string) NetWorth {
	dates := make([]NetWorthDate, len(n.Dates))
	for i, d := range n.Dates {
		balances := make([]NetWorthBalance, len(d.Rows))
		for j, r := range d.Rows {
			balances[j] = NetWorthBalance{
				Type: r.Type, Currency: r.Currency, Balance: BigMoney(r.Balance), ConvertedBalance: nullableMoney(n.Converted(r)),
			}
		}
		totals := make([]NetWorthTotal, len(d.Totals))
		for j, t := range d.Totals {
			totals[j] = NetWorthTotal{Currency: t.Currency, Value: BigMoney(t.Value)}
		}
		dates[i] = NetWorthDate{Date: d.Date.Format(DateLayout), Balances: balances, Totals: totals}
	}
	return NetWorth{
		AsOf:     nullable(&n.AsOf, func(d time.Time) string { return d.Format(DateLayout) }),
		Currency: n.Currency.String(),
		Dates:    dates,
		Warnings: append([]string{}, warnings...),
	}
}
