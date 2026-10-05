package report

import (
	"context"
	"math/big"
	"time"

	"github.com/koblas/quarry/internal/store"
)

// ACBRequest is what ACB walks: the accounts Classification names non-registered, through Today.
type ACBRequest struct {
	Classification Classification
	Today          time.Time
}

// ACB is the adjusted cost base of each security pooled across the non-registered accounts, and the capital
// gains realized each tax year, in CAD cents.
type ACB struct {
	Years      []ACBYear
	Securities []ACBSecurity
}

// ACBYear is the sales dated in one calendar year and the sum of their CAD columns.
type ACBYear struct {
	Year                                int
	Sales                               []ACBSale
	Proceeds, Outlays, ACBRemoved, Gain int64
}

// ACBSale is one disposition: the shares sold, its proceeds and outlays in CAD cents, the ACB it removed, and
// the gain, which is Proceeds - Outlays - ACBRemoved.
type ACBSale struct {
	ID                                  string
	Date                                time.Time
	SecurityID                          string
	Shares                              *big.Rat
	Proceeds, Outlays, ACBRemoved, Gain int64
}

// ACBSecurity is one security's position after its last event and the events that led there.
type ACBSecurity struct {
	Security store.Security
	Shares   *big.Rat
	ACB      int64
	Events   []ACBEvent
}

// PerShare is the ACB per share in CAD dollars, or nil when no shares are held.
func (s ACBSecurity) PerShare() *big.Rat {
	if s.Shares.Sign() == 0 {
		return nil
	}

	return new(big.Rat).Quo(big.NewRat(s.ACB, 100), s.Shares)
}

// ACBEvent is one transaction the walk applied: the units it moved and the pool it left.
type ACBEvent struct {
	ID     string
	Date   time.Time
	Action string
	Shares *big.Rat
	Held   *big.Rat
	ACB    int64
	Gain   int64
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
