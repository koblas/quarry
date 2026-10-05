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

// NetWorthRequest is what NetWorth reads: the net worth on AsOf, shown in Currency.
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

// NetWorth is the net worth on AsOf, shown in Currency; a snapshot has exactly one Dates entry, rows or not.
type NetWorth struct {
	Dates    []NetWorthDate
	AsOf     time.Time
	Window   *store.Window
	Currency money.Currency
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

// NetWorth lists the net worth on req.AsOf with the totals of its balances in req.Currency.
// It reads the store once, and refuses like Status.
func (s *Server) NetWorth(ctx context.Context, req NetWorthRequest) (NetWorth, error) {
	read, err := s.store.NetWorth(ctx, store.NetWorthParams{Dates: []time.Time{req.AsOf}})
	if err != nil {
		return NetWorth{}, s.readRefusal(ctx, "networth", err)
	}
	listing := NetWorth{AsOf: req.AsOf, Currency: req.Currency}
	date := NetWorthDate{Date: req.AsOf, Rows: read.Rows}
	date.Totals = listing.total(date.Rows)
	listing.Dates = []NetWorthDate{date}
	return listing, nil
}

// total is the sum of the rows' converted balances in the reporting currency, none when no row converts;
// a native listing totals each stored currency's balances on its own instead.
func (n NetWorth) total(rows []store.NetWorthRow) []NetWorthTotal {
	if n.Currency == money.Native {
		return nativeNetWorthTotals(rows)
	}
	sum := new(big.Int)
	var found bool
	for _, row := range rows {
		if value := n.Converted(row); value != nil {
			sum.Add(sum, value)
			found = true
		}
	}
	if !found {
		return nil
	}
	return []NetWorthTotal{{Currency: n.Currency.String(), Value: sum}}
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
