package report

import (
	"cmp"
	"math/big"
	"slices"

	"github.com/koblas/quarry/internal/store"
)

// Shares are stored in millionths and commissions in hundredths of a cent.
const (
	acbUnitsPerShare     = 1_000_000
	acbCommissionPerCent = 100
)

// acbTiers orders one day's transactions in a pool: acquisitions, then dispositions. An action absent from
// it does not touch the pool.
var acbTiers = map[string]int{
	store.ActionBuy:  0,
	store.ActionSell: 2,
}

// acbPool is one security's units and adjusted cost base across the non-registered accounts.
type acbPool struct {
	shares *big.Rat
	acb    int64
}

// acbSale is a sale and the source id that breaks a tie with another security's sale the same day.
type acbSale struct {
	row      ACBSale
	sourceID int64
}

// walkACB walks every security's pooled history in the non-registered accounts through req.Today.
func walkACB(history store.InvestmentHistory, req ACBRequest) ACB {
	inPool := nonRegisteredAccounts(history.Accounts, req.Classification)
	bySecurity := make(map[string][]store.InvestmentTransaction)
	for _, tx := range history.Transactions {
		_, walked := acbTiers[tx.Action]
		// unreachable: duckstore's InvestmentHistory reads only transactions whose security_id is set (investments.go).
		if tx.SecurityID == nil || !walked || !inPool[tx.AccountID] || tx.Date.After(req.Today) {
			continue
		}
		bySecurity[*tx.SecurityID] = append(bySecurity[*tx.SecurityID], tx)
	}

	var result ACB
	var sales []acbSale
	for _, security := range history.Securities {
		txs := bySecurity[security.ID]
		if len(txs) == 0 {
			continue
		}
		position, securitySales := walkSecurity(security, txs)
		result.Securities = append(result.Securities, position)
		sales = append(sales, securitySales...)
	}
	result.Years = acbYears(sales)

	return result
}

// nonRegisteredAccounts is the ids of the accounts c lists non-registered; an unclassified account is in no pool.
func nonRegisteredAccounts(accounts []store.Account, c Classification) map[string]bool {
	ids := make(map[string]bool)
	for _, a := range accounts {
		if registered := c.Of(a); registered != nil && !*registered {
			ids[a.ID] = true
		}
	}

	return ids
}

// walkSecurity applies txs, in date, tier then source id order, to one pool and returns its position and sales.
func walkSecurity(security store.Security, txs []store.InvestmentTransaction) (ACBSecurity, []acbSale) {
	slices.SortFunc(txs, func(a, b store.InvestmentTransaction) int {
		return cmp.Or(
			a.Date.Compare(b.Date),
			cmp.Compare(acbTiers[a.Action], acbTiers[b.Action]),
			cmp.Compare(a.SourceID, b.SourceID),
		)
	})

	pool := acbPool{shares: new(big.Rat)}
	position := ACBSecurity{Security: security}
	var sales []acbSale
	for _, tx := range txs {
		event := ACBEvent{ID: tx.ID, Date: tx.Date, Action: tx.Action, Shares: units(tx.Shares)}
		switch tx.Action {
		case store.ActionBuy:
			pool.shares.Add(pool.shares, event.Shares)
			pool.acb -= tx.Amount
		case store.ActionSell:
			sale := pool.sell(tx, event.Shares)
			event.Gain = sale.row.Gain
			sales = append(sales, sale)
		}
		event.Held, event.ACB = new(big.Rat).Set(pool.shares), pool.acb
		position.Events = append(position.Events, event)
	}
	position.Shares, position.ACB = pool.shares, pool.acb

	return position, sales
}

// sell removes sold units and their share of the ACB from the pool, and returns the sale. A sale of all the
// units held, or more, removes the whole ACB and leaves the pool empty.
func (p *acbPool) sell(tx store.InvestmentTransaction, sold *big.Rat) acbSale {
	removed := p.acb
	if p.shares.Sign() > 0 && sold.Cmp(p.shares) < 0 {
		removed = roundHalfAway(new(big.Rat).Mul(big.NewRat(p.acb, 1), new(big.Rat).Quo(sold, p.shares)))
		p.shares.Sub(p.shares, sold)
		p.acb -= removed
	} else {
		p.shares.SetInt64(0)
		p.acb = 0
	}

	outlays := int64(0)
	if tx.Commission != nil {
		outlays = roundHalfAway(big.NewRat(*tx.Commission, acbCommissionPerCent))
	}
	proceeds := tx.Amount + outlays

	return acbSale{
		row: ACBSale{
			ID: tx.ID, Date: tx.Date, SecurityID: *tx.SecurityID, Shares: sold,
			Proceeds: proceeds, Outlays: outlays, ACBRemoved: removed, Gain: proceeds - outlays - removed,
		},
		sourceID: tx.SourceID,
	}
}

// acbYears groups sales by the calendar year of their date, oldest year first, each year's sales in date,
// then source id order.
func acbYears(sales []acbSale) []ACBYear {
	slices.SortFunc(sales, func(a, b acbSale) int {
		return cmp.Or(a.row.Date.Compare(b.row.Date), cmp.Compare(a.sourceID, b.sourceID))
	})

	var years []ACBYear
	for _, sale := range sales {
		if len(years) == 0 || years[len(years)-1].Year != sale.row.Date.Year() {
			years = append(years, ACBYear{Year: sale.row.Date.Year()})
		}
		year := &years[len(years)-1]
		year.Sales = append(year.Sales, sale.row)
		year.Proceeds += sale.row.Proceeds
		year.Outlays += sale.row.Outlays
		year.ACBRemoved += sale.row.ACBRemoved
		year.Gain += sale.row.Gain
	}

	return years
}

// units is shares, in millionths, as a number of shares; a missing count is none.
func units(millionths *int64) *big.Rat {
	if millionths == nil {
		return new(big.Rat)
	}

	return big.NewRat(*millionths, acbUnitsPerShare)
}

// roundHalfAway is r rounded to the nearest whole number, halves away from zero.
func roundHalfAway(r *big.Rat) int64 {
	twice := new(big.Int).Abs(r.Num())
	twice.Lsh(twice, 1)
	twice.Add(twice, r.Denom())
	rounded := twice.Quo(twice, new(big.Int).Lsh(r.Denom(), 1))
	if r.Sign() < 0 {
		rounded.Neg(rounded)
	}

	return rounded.Int64()
}
