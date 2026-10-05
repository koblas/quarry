package report

import (
	"context"
	"math/big"
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
	AsOf       time.Time
	Years      []ACBYear
	Securities []ACBSecurity
	// AdjustmentIssues are the adjustments skipped or repeated, by item number.
	AdjustmentIssues []ACBAdjustmentIssue
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
	marked := 0
	for _, sale := range y.Sales {
		if sale.PossibleSuperficialLoss {
			marked++
		}
	}

	return marked
}

// ACBSale is one disposition: the shares sold, its proceeds and outlays in CAD cents, the ACB it removed, and
// the gain, which is Proceeds - Outlays - ACBRemoved. The two flags mark the sale for the reader; a loss is
// marked possibly superficial when the walk finds the security acquired within 30 days of it and still held.
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
	// Incomplete is true when the ACB rests on shares with no recorded cost or on a trade with no rate.
	Incomplete bool
	Events     []ACBEvent
}

// PerShare is the ACB per share in CAD dollars, or nil when no shares are held.
func (s ACBSecurity) PerShare() *big.Rat {
	if s.Shares.Sign() == 0 {
		return nil
	}

	return new(big.Rat).Quo(big.NewRat(s.ACB, 100), s.Shares)
}

// ACBEvent is one transaction the walk applied: the units it moved and the pool it left. Amount and Currency
// are the transaction's own; CAD is Amount at Rate, which is 0 unless the trade was in USD with a rate on file.
// Outlays (CAD cents) and Gain are meaningful only when Realized, which a sale and a return of capital above the
// ACB set.
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
