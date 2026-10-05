package report

import (
	"cmp"
	"math/big"
	"slices"
	"strings"
	"time"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/store"
)

// Shares are stored in millionths and commissions in hundredths of a cent.
const (
	acbUnitsPerShare     = 1_000_000
	acbCommissionPerCent = 100
)

// acbTiers orders one day's transactions in a pool: acquisitions, then splits, then dispositions. An action
// absent from it does not touch the pool.
var acbTiers = map[string]int{
	store.ActionBuy:              0,
	store.ActionReinvestDividend: 0,
	store.ActionSplit:            1,
	store.ActionSell:             2,
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
		position, securitySales := walkSecurity(security, txs, history.Rates)
		result.Securities = append(result.Securities, position)
		sales = append(sales, securitySales...)
	}
	slices.SortFunc(result.Securities, func(a, b ACBSecurity) int {
		return cmp.Or(
			cmp.Compare(strings.ToLower(a.Security.Name), strings.ToLower(b.Security.Name)),
			cmp.Compare(a.Security.ID, b.Security.ID),
		)
	})
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
// rates convert each foreign-currency amount at the rate on or before its date.
func walkSecurity(security store.Security, txs []store.InvestmentTransaction, rates []store.Rate) (ACBSecurity, []acbSale) {
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
	var splitDay time.Time
	for _, tx := range txs {
		event := ACBEvent{ID: tx.ID, Date: tx.Date, Action: tx.Action, Shares: units(tx.Shares)}
		rate := rateOn(rates, tx.Date)
		switch tx.Action {
		case store.ActionBuy:
			pool.add(event.Shares, -toCAD(tx.Amount, tx.Currency, rate))
		case store.ActionReinvestDividend:
			pool.add(event.Shares, toCAD(zeroIfNil(tx.CostBasis), tx.Currency, rate))
		case store.ActionSplit:
			// A split is recorded once per account that held the security; the first row of a day is the split.
			if tx.Date.Equal(splitDay) {
				continue
			}
			splitDay = tx.Date
			pool.split(tx.SplitNewShares, tx.SplitOldShares)
		case store.ActionSell:
			sale := pool.sell(tx, event.Shares, rate)
			event.Gain = sale.row.Gain
			sales = append(sales, sale)
		}
		event.Held, event.ACB = new(big.Rat).Set(pool.shares), pool.acb
		position.Events = append(position.Events, event)
	}
	position.Shares, position.ACB = pool.shares, pool.acb

	return position, sales
}

// add puts bought units and their cost into the pool.
func (p *acbPool) add(bought *big.Rat, cost int64) {
	p.shares.Add(p.shares, bought)
	p.acb += cost
}

// split multiplies the units held by newShares over oldShares and leaves the ACB alone.
func (p *acbPool) split(newShares, oldShares *int64) {
	if newShares == nil || oldShares == nil || *newShares <= 0 || *oldShares <= 0 {
		// unreachable: the build refuses a split with a missing or non-positive side (duckstore.go splitRatio).
		return
	}
	p.shares.Mul(p.shares, big.NewRat(*newShares, *oldShares))
}

// sell removes sold units and their share of the ACB, or all of it when they are all the units held or more,
// and returns the sale in CAD.
func (p *acbPool) sell(tx store.InvestmentTransaction, sold *big.Rat, rate money.Rate) acbSale {
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
	proceeds := toCAD(tx.Amount+outlays, tx.Currency, rate)
	outlays = toCAD(outlays, tx.Currency, rate)

	return acbSale{
		row: ACBSale{
			ID: tx.ID, Date: tx.Date, SecurityID: *tx.SecurityID, Shares: sold,
			Proceeds: proceeds, Outlays: outlays, ACBRemoved: removed, Gain: proceeds - outlays - removed,
		},
		sourceID: tx.SourceID,
	}
}

// rateOn is the USD/CAD rate of the latest day on or before day in rates, which are in date order; zero when none.
func rateOn(rates []store.Rate, day time.Time) money.Rate {
	i, found := slices.BinarySearchFunc(rates, day, func(r store.Rate, d time.Time) int { return r.Date.Compare(d) })
	switch {
	case found:
		return rates[i].USDCAD
	case i > 0:
		return rates[i-1].USDCAD
	default:
		return 0
	}
}

// toCAD is cents of currency in CAD at rate, or 0 when it cannot be converted: an unknown currency, or USD with no rate.
func toCAD(cents int64, currency string, rate money.Rate) int64 {
	from, _ := money.ParseCurrency(currency)
	converted, _ := money.Convert(cents, from, money.CAD, rate)

	return converted
}

func zeroIfNil(n *int64) int64 {
	if n == nil {
		return 0
	}

	return *n
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
