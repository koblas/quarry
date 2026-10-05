package report

import (
	"cmp"
	"context"
	"math/big"
	"slices"
	"strings"
	"time"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/store"
)

// NetWorthRequest is what NetWorth reads: the net worth on AsOf, or when Window is set on each month end in
// it, shown in Currency.
type NetWorthRequest struct {
	AsOf     time.Time
	Window   *store.Window
	Currency money.Currency
}

// NetWorthTotal is the sum of the balances of the rows it covers, in Currency.
type NetWorthTotal struct {
	Currency string
	Value    *big.Int
}

// NetWorthDate is the net worth on one day: its rows in the store's order, and their totals.
type NetWorthDate struct {
	Date   time.Time
	Rows   []store.NetWorthRow
	Totals []NetWorthTotal
}

// NetWorth is the net worth on AsOf, or when Window is set at each month end in it, shown in Currency.
// Every day asked for has a Dates entry, rows or not.
type NetWorth struct {
	Dates    []NetWorthDate
	AsOf     time.Time
	Window   *store.Window
	Currency money.Currency

	// Unvalued is the holdings of the counted accounts, on the days listed, that their balances leave out.
	Unvalued []store.UnvaluedHolding

	// FirstRate is the date of the store's earliest exchange rate; zero when it holds none.
	FirstRate time.Time
}

// NeedsRate reports whether row has a balance and only an exchange rate it lacks keeps it out of the converted
// total; never in a native listing.
func (n NetWorth) NeedsRate(row store.NetWorthRow) bool {
	return n.Currency != money.Native && n.Converted(row) == nil && row.Balance.Sign() != 0
}

// TypeNeedsRate reports whether date has a row of accountType that needs a rate and none of that type converts.
func (n NetWorth) TypeNeedsRate(date NetWorthDate, accountType string) bool {
	var needs bool
	for _, row := range date.Rows {
		if row.Type != accountType {
			continue
		}
		if n.Converted(row) != nil {
			return false
		}
		needs = needs || n.NeedsRate(row)
	}
	return needs
}

// Converted is row's balance in the reporting currency in cents; nil in a native listing and when no
// exchange rate converts it.
func (n NetWorth) Converted(row store.NetWorthRow) *big.Int {
	if n.Currency == money.CAD {
		return row.BalanceCAD
	}
	if n.Currency == money.USD {
		return row.BalanceUSD
	}
	return nil
}

// NetWorth lists the net worth on req.AsOf, or on each month end in req.Window, with the totals of each day's
// balances in req.Currency. It reads the store once, and refuses like Status.
func (s *Server) NetWorth(ctx context.Context, req NetWorthRequest) (NetWorth, error) {
	days := []time.Time{req.AsOf}
	if req.Window != nil {
		days = monthEnds(*req.Window)
	}
	read, err := s.store.NetWorth(ctx, store.NetWorthParams{Dates: days})
	if err != nil {
		return NetWorth{}, s.readRefusal(ctx, "networth", err)
	}

	listing := NetWorth{AsOf: req.AsOf, Window: req.Window, Currency: req.Currency, FirstRate: read.FirstRate}
	listing.Dates = make([]NetWorthDate, len(days))
	position := make(map[string]int, len(days))
	for i, day := range days {
		listing.Dates[i].Date = day
		position[day.Format(time.DateOnly)] = i
	}
	for _, row := range read.Rows {
		if i, ok := position[row.Date.Format(time.DateOnly)]; ok {
			listing.Dates[i].Rows = append(listing.Dates[i].Rows, row)
		}
	}
	for _, held := range read.Unvalued {
		if _, ok := position[held.Date.Format(time.DateOnly)]; ok {
			listing.Unvalued = append(listing.Unvalued, held)
		}
	}
	for i := range listing.Dates {
		listing.Dates[i].Totals = listing.total(listing.Dates[i].Rows)
	}
	return listing, nil
}

// monthEnds is the last day of each month from window.Since's through window.Until, then window.Until
// itself when it is not a month end; empty when Since is after Until. Days are UTC midnights.
func monthEnds(window store.Window) []time.Time {
	if window.Since.After(window.Until) {
		return nil
	}
	var ends []time.Time
	for end := monthEnd(window.Since); !end.After(window.Until); end = monthEnd(end.AddDate(0, 0, 1)) {
		ends = append(ends, end)
	}
	if len(ends) == 0 || ends[len(ends)-1].Before(window.Until) {
		ends = append(ends, window.Until)
	}
	return ends
}

// monthEnd is the last day of day's month.
func monthEnd(day time.Time) time.Time {
	return time.Date(day.Year(), day.Month()+1, 0, 0, 0, 0, 0, time.UTC)
}

// Types is the account types with a non-zero balance on some day, alphabetically.
func (n NetWorth) Types() []string {
	var types []string
	for _, date := range n.Dates {
		for _, row := range date.Rows {
			if row.Balance.Sign() != 0 {
				types = append(types, row.Type)
			}
		}
	}
	slices.Sort(types)
	return slices.Compact(types)
}

// TypeConverted is the sum of date's balances of one account type in the reporting currency, adding only
// those a rate converts; nil when the type has no row that day or no row converts.
func (n NetWorth) TypeConverted(date NetWorthDate, accountType string) *big.Int {
	var sum *big.Int
	for _, row := range date.Rows {
		if value := n.Converted(row); row.Type == accountType && value != nil {
			if sum == nil {
				sum = new(big.Int)
			}
			sum.Add(sum, value)
		}
	}
	return sum
}

// TypeBalance is the day's balance of one account type in one stored currency; nil when there is no such row.
func (d NetWorthDate) TypeBalance(accountType, currency string) *big.Int {
	for _, row := range d.Rows {
		if row.Type == accountType && row.Currency == currency {
			return row.Balance
		}
	}
	return nil
}

// total is the converted rows' sum in the reporting currency (none when no row converts), then one total per
// currency of the rows that need a rate; a native listing totals each currency alone.
func (n NetWorth) total(rows []store.NetWorthRow) []NetWorthTotal {
	if n.Currency == money.Native {
		return nativeNetWorthTotals(rows)
	}
	sum := new(big.Int)
	var found bool
	var unconverted []store.NetWorthRow
	for _, row := range rows {
		if value := n.Converted(row); value != nil {
			sum.Add(sum, value)
			found = true
		} else if n.NeedsRate(row) {
			unconverted = append(unconverted, row)
		}
	}
	var totals []NetWorthTotal
	if found {
		totals = append(totals, NetWorthTotal{Currency: n.Currency.String(), Value: sum})
	}
	return append(totals, nativeNetWorthTotals(unconverted)...)
}

// nativeNetWorthTotals is one total per stored currency among rows, CAD then USD then the rest alphabetically.
func nativeNetWorthTotals(rows []store.NetWorthRow) []NetWorthTotal {
	sums := map[string]*big.Int{}
	for _, row := range rows {
		if sums[row.Currency] == nil {
			sums[row.Currency] = new(big.Int)
		}
		sums[row.Currency].Add(sums[row.Currency], row.Balance)
	}
	totals := make([]NetWorthTotal, 0, len(sums))
	for code, sum := range sums {
		totals = append(totals, NetWorthTotal{Currency: code, Value: sum})
	}
	slices.SortFunc(totals, func(a, b NetWorthTotal) int {
		return cmp.Or(nativeRank(a.Currency)-nativeRank(b.Currency), strings.Compare(a.Currency, b.Currency))
	})
	return totals
}
