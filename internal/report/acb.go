package report

import (
	"context"
	"math/big"
	"slices"
	"time"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/store"
)

// ACBRequest is what ACB walks: the accounts Classification names non-registered, through Today, and the
// Adjustments (T3 slip amounts) applied to the pools; an item's 1-based place in Adjustments is its number.
type ACBRequest struct {
	Classification Classification
	Today          time.Time
	Adjustments    []ACBAdjustment
}

// ACBAdjustment is one return of capital and/or reinvested distribution, in CAD cents, on a security on a date;
// a kind it does not give is 0.
type ACBAdjustment struct {
	SecurityID             string
	Date                   time.Time
	ReturnOfCapital        int64
	ReinvestedDistribution int64
}

// The actions of an adjustment's events, which no transaction carries.
const (
	ACBActionReturnOfCapital        = "return of capital"
	ACBActionReinvestedDistribution = "reinvested distribution"
)

// ACB is the adjusted cost base of each security pooled across the non-registered accounts, and the capital
// gains realized each tax year, in CAD cents.
type ACB struct {
	AsOf time.Time
	// FirstRate is the date of the first exchange rate in the store; zero when the store has none.
	FirstRate  time.Time
	Years      []ACBYear
	Securities []ACBSecurity
	// AdjustmentIssues are the adjustments skipped or repeated, by item number.
	AdjustmentIssues []ACBAdjustmentIssue
}

// ACBSharedTicker is a ticker that two or more of the report's securities carry.
type ACBSharedTicker struct {
	Ticker     string
	Securities []store.Security
}

// SharedTickers is each ticker of two or more of a's securities, by the walk's first member, members in the walk's
// order. A ticker matches exactly, case included; a security with none or an empty one has no group.
func (a ACB) SharedTickers() []ACBSharedTicker {
	var groups []ACBSharedTicker
	at := make(map[string]int)
	for _, security := range a.Securities {
		ticker, ok := groupingTicker(security.Security)
		if !ok {
			continue
		}
		i, seen := at[ticker]
		if !seen {
			i = len(groups)
			at[ticker] = i
			groups = append(groups, ACBSharedTicker{Ticker: ticker})
		}
		groups[i].Securities = append(groups[i].Securities, security.Security)
	}

	return slices.DeleteFunc(groups, func(g ACBSharedTicker) bool { return len(g.Securities) < 2 })
}

// ACBAdjustmentKind is what the walk found wrong with an adjustment item.
type ACBAdjustmentKind int

// The kinds of ACBAdjustmentIssue: skipped for an unknown or not-held security, or repeating an applied item.
const (
	ACBAdjustmentUnknownSecurity ACBAdjustmentKind = iota
	ACBAdjustmentNotHeld
	ACBAdjustmentRepeated
)

// ACBAdjustmentIssue is an adjustment item (1-based) the walk skipped or applied beside the earlier item First,
// which is 0 unless the kind is Repeated. Security is the security's name, empty when the store lacks it.
type ACBAdjustmentIssue struct {
	Kind       ACBAdjustmentKind
	Item       int
	First      int
	SecurityID string
	Security   string
	Date       time.Time
}

// ACBYear is the sales dated in one calendar year and the sum of their CAD columns, and the returns of capital
// above the ACB that year, a capital gain apart from the sales.
type ACBYear struct {
	Year                                int
	Sales                               []ACBSale
	Proceeds, Outlays, ACBRemoved, Gain int64
	ReturnOfCapitalGain                 int64
}

// PossibleSuperficialLosses is how many of the year's sales are marked as a possible superficial loss.
func (y ACBYear) PossibleSuperficialLosses() int {
	return y.countSales(func(sale ACBSale) bool { return sale.PossibleSuperficialLoss })
}

// UnknownCostSales is how many of the year's sales are marked as sales of shares with no recorded cost.
func (y ACBYear) UnknownCostSales() int {
	return y.countSales(func(sale ACBSale) bool { return sale.UnknownCost })
}

func (y ACBYear) countSales(marked func(ACBSale) bool) int {
	n := 0
	for _, sale := range y.Sales {
		if marked(sale) {
			n++
		}
	}

	return n
}

// ACBSale is one disposition: the shares sold, its proceeds and outlays in CAD cents, the ACB it removed, and
// the gain, which is Proceeds - Outlays - ACBRemoved. PossibleSuperficialLoss marks a loss with the security
// acquired within 30 days of it and still held; UnknownCost marks a sale from a pool holding shares with no cost.
type ACBSale struct {
	ID                                  string
	Date                                time.Time
	SecurityID                          string
	AccountID, Account                  string
	Shares                              *big.Rat
	Proceeds, Outlays, ACBRemoved, Gain int64
	PossibleSuperficialLoss             bool
	UnknownCost                         bool
}

// ACBSecurity is one security's position after its last event and the events that led there.
type ACBSecurity struct {
	Security store.Security
	Shares   *big.Rat
	ACB      int64
	// Incomplete is true when the ACB rests on shares with no recorded cost or on a trade quarry cannot value.
	Incomplete bool
	// NoRate is the earliest trade quarry could not convert to CAD; nil when it valued them all. A security
	// with one is left out of the year totals, every year.
	NoRate *ACBNoRate
	Events []ACBEvent
}

// ACBNoRate is a trade in a currency quarry could not convert to CAD: USD with no rate on or before its date,
// or any other non-CAD currency, Currency being the trade's own code.
type ACBNoRate struct {
	Date     time.Time
	Currency string
}

// PerShare is the ACB per share in CAD dollars, or nil when no shares are held.
func (s ACBSecurity) PerShare() *big.Rat {
	if s.Shares.Sign() == 0 {
		return nil
	}

	return new(big.Rat).Quo(big.NewRat(s.ACB, 100), s.Shares)
}

// ACBEvent is one transaction the walk applied. CAD is Amount at Rate, 0 unless a USD trade with a rate on file;
// Outlays and Gain mean something only when Realized (a sale, or a return of capital above the ACB).
// UnknownCost marks shares moved with no recorded cost; Unvalued marks a trade quarry could not convert to CAD,
// whose CAD and Gain are unknown, not 0.
type ACBEvent struct {
	ID                 string
	Date               time.Time
	AccountID, Account string
	Action             string
	Shares             *big.Rat
	Amount             *int64
	Currency           string
	Rate               money.Rate
	CAD                int64
	Outlays            *int64
	Held               *big.Rat
	ACB                int64
	Gain               int64
	Realized           bool
	UnknownCost        bool
	Unvalued           bool
}

// Millionths is a count of shares in millionths, rounded half away from zero.
func Millionths(shares *big.Rat) int64 {
	return roundHalfAway(new(big.Rat).Mul(shares, big.NewRat(acbUnitsPerShare, 1)))
}

// acbCommand names the command in the interrupt and store refusals.
const acbCommand = "acb"

// ACB walks the investment history of the accounts req.Classification names non-registered, through req.Today.
// It reads the store once, and refuses like Status.
func (s *Server) ACB(ctx context.Context, req ACBRequest) (ACB, error) {
	history, err := s.store.InvestmentHistory(ctx)
	if err != nil {
		return ACB{}, s.readRefusal(ctx, acbCommand, err)
	}

	return walkACB(history, req), nil
}
