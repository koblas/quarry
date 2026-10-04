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

// HoldingsRequest is what Holdings reads: the holdings on AsOf, shown in Currency, in the accounts
// Accounts name by id or by name, or in every account when it is empty.
type HoldingsRequest struct {
	AsOf     time.Time
	Currency money.Currency
	Accounts []string
}

// HoldingsTotal is the sum of the values of the rows it covers, in Currency.
type HoldingsTotal struct {
	Currency string
	Value    *big.Int
}

// Holdings is the holdings on AsOf, in the store's order, with their totals.
// FirstRate is the date of the store's first exchange rate, zero when it has none.
// FirstTransaction and LastTransaction span the investment transactions in the accounts read, zero when none.
// Accounts are the accounts the request named, in the order given and without repeats; nil when none.
type Holdings struct {
	Rows             []store.Holding
	Totals           []HoldingsTotal
	AsOf             time.Time
	Currency         money.Currency
	FirstRate        time.Time
	FirstTransaction time.Time
	LastTransaction  time.Time
	Accounts         []store.Account
}

// Converted is h's value in the reporting currency in cents; nil in a native listing and when no
// conversion exists (no price, no currency, another currency, no rate).
func (l Holdings) Converted(h store.Holding) *big.Int {
	if l.Currency == money.CAD {
		return h.ValueCAD
	}
	if l.Currency == money.USD {
		return h.ValueUSD
	}
	return nil
}

// Convertible reports whether h's security is priced in CAD or USD, the only currencies quarry converts;
// a security with no currency, or any other, is never converted.
func Convertible(h store.Holding) bool {
	return h.Currency != nil && (*h.Currency == "CAD" || *h.Currency == "USD")
}

// NeedsRate reports whether h is priced in the other of CAD and USD than the reporting currency and has no
// converted value, so only an exchange rate it lacks keeps it out of the converted total.
func (l Holdings) NeedsRate(h store.Holding) bool {
	return l.Currency != money.Native && h.Price != nil && Convertible(h) &&
		*h.Currency != l.Currency.String() && l.Converted(h) == nil
}

// Holdings lists the holdings on req.AsOf with the total of their values in req.Currency.
// It refuses like Spend for an account name that picks none or several, else like Status,
// and reads the holdings once.
func (s *Server) Holdings(ctx context.Context, req HoldingsRequest) (Holdings, error) {
	accounts, ids, err := s.namedAccounts(ctx, "holdings", req.Accounts)
	if err != nil {
		return Holdings{}, err
	}
	read, err := s.store.Holdings(ctx, store.HoldingsParams{AsOf: req.AsOf, AccountIDs: ids})
	if err != nil {
		return Holdings{}, s.readRefusal(ctx, "holdings", err)
	}
	listing := Holdings{
		Rows: read.Holdings, AsOf: req.AsOf, Currency: req.Currency, FirstRate: read.FirstRate, Accounts: accounts,
		FirstTransaction: read.FirstTransaction, LastTransaction: read.LastTransaction,
	}
	listing.Totals = listing.total()
	return listing, nil
}

// total is the sum of the rows' converted values in the reporting currency, then the sum of the values of the
// rows that need a rate, in the other currency; each only when it has a row. A native listing totals each
// row's own value per stored currency instead.
func (l Holdings) total() []HoldingsTotal {
	if l.Currency == money.Native {
		return l.nativeTotals()
	}
	converted, unconverted := new(big.Int), new(big.Int)
	var totals []HoldingsTotal
	var anyConverted, anyUnconverted bool
	for _, row := range l.Rows {
		if value := l.Converted(row); value != nil {
			converted.Add(converted, value)
			anyConverted = true
		}
		if l.NeedsRate(row) {
			unconverted.Add(unconverted, row.Value)
			anyUnconverted = true
		}
	}
	if anyConverted {
		totals = append(totals, HoldingsTotal{Currency: l.Currency.String(), Value: converted})
	}
	if anyUnconverted {
		totals = append(totals, HoldingsTotal{Currency: money.NativeOf(l.Currency).String(), Value: unconverted})
	}
	return totals
}

// nativeTotals is one total per stored currency among the rows with a value, CAD then USD then the
// rest alphabetically. A row with no currency is never totalled.
func (l Holdings) nativeTotals() []HoldingsTotal {
	sums := map[string]*big.Int{}
	for _, row := range l.Rows {
		if row.Currency == nil || row.Value == nil {
			continue
		}
		if sums[*row.Currency] == nil {
			sums[*row.Currency] = new(big.Int)
		}
		sums[*row.Currency].Add(sums[*row.Currency], row.Value)
	}
	totals := make([]HoldingsTotal, 0, len(sums))
	for code, sum := range sums {
		totals = append(totals, HoldingsTotal{Currency: code, Value: sum})
	}
	slices.SortFunc(totals, func(a, b HoldingsTotal) int {
		return cmp.Or(nativeRank(a.Currency)-nativeRank(b.Currency), strings.Compare(a.Currency, b.Currency))
	})
	if len(totals) == 0 {
		return nil
	}
	return totals
}

// nativeRank puts CAD first and USD second; every other currency ties and falls to alphabetical order.
func nativeRank(code string) int {
	switch code {
	case "CAD":
		return 0
	case "USD":
		return 1
	default:
		return 2
	}
}
