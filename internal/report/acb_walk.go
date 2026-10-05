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

// A day's pool transactions run acquisitions, then splits and adjustments, then dispositions.
const (
	acbAcquisition = iota
	acbRestructure
	acbDisposition
)

// acbTiers is each walked action's tier; absent actions are skipped.
var acbTiers = map[string]int{
	store.ActionBuy:              acbAcquisition,
	store.ActionReinvestDividend: acbAcquisition,
	store.ActionAddShares:        acbAcquisition,
	store.ActionSplit:            acbRestructure,
	store.ActionSell:             acbDisposition,
	store.ActionRemoveShares:     acbDisposition,
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
		// unreachable: the nil-security arm, since duckstore's InvestmentHistory reads only rows with a security_id (investments.go:20).
		if tx.SecurityID == nil || !walked || !inPool[tx.AccountID] || tx.Date.After(req.Today) {
			continue
		}
		bySecurity[*tx.SecurityID] = append(bySecurity[*tx.SecurityID], tx)
	}

	result := ACB{AsOf: req.Today}
	days, issues := adjustmentDays(req, history.Securities)
	names := accountNames(history.Accounts)
	var sales []acbSale
	var excesses []acbExcess
	// A transaction naming a security absent from Securities is never walked; the importer cannot write one
	// (investments.go resolveSecurity sets security_id only from a mapped security).
	for _, security := range history.Securities {
		txs := bySecurity[security.ID]
		if len(txs) == 0 && len(days[security.ID]) == 0 {
			continue
		}
		walk := walkSecurity(security, txs, days[security.ID], history.Rates, names)
		if len(txs) > 0 {
			result.Securities = append(result.Securities, walk.position)
		}
		sales = append(sales, walk.sales...)
		excesses = append(excesses, walk.excesses...)
		issues = append(issues, walk.issues...)
	}
	slices.SortFunc(result.Securities, func(a, b ACBSecurity) int {
		return cmp.Or(
			cmp.Compare(strings.ToLower(a.Security.Name), strings.ToLower(b.Security.Name)),
			cmp.Compare(a.Security.ID, b.Security.ID),
		)
	})
	slices.SortFunc(issues, func(a, b ACBAdjustmentIssue) int { return cmp.Compare(a.Item, b.Item) })
	markSuperficialLosses(sales, history, req.Today)
	result.Years = acbYears(sales, excesses)
	result.AdjustmentIssues = issues

	return result
}

// acbItem is one adjustment, dated date, and its number among the request's.
type acbItem struct {
	number                        int
	date                          time.Time
	returnOfCapital, reinvestment int64
}

// acbAdjustmentDay is one security's items dated one day, in request order.
type acbAdjustmentDay struct {
	date  time.Time
	items []acbItem
}

// adjustmentDays groups the request's adjustments dated through Today by security, then by day, oldest
// first. An item naming a security absent from securities is skipped and returned as an issue.
func adjustmentDays(req ACBRequest, securities []store.Security) (map[string][]acbAdjustmentDay, []ACBAdjustmentIssue) {
	known := make(map[string]bool, len(securities))
	for _, security := range securities {
		known[security.ID] = true
	}

	days := make(map[string][]acbAdjustmentDay)
	var issues []ACBAdjustmentIssue
	for i, a := range req.Adjustments {
		item := acbItem{number: i + 1, date: a.Date, returnOfCapital: a.ReturnOfCapital, reinvestment: a.ReinvestedDistribution}
		switch {
		case a.Date.After(req.Today):
		case !known[a.SecurityID]:
			issues = append(issues, ACBAdjustmentIssue{Kind: ACBAdjustmentUnknownSecurity, Item: item.number, SecurityID: a.SecurityID, Date: a.Date})
		default:
			days[a.SecurityID] = addToDay(days[a.SecurityID], item)
		}
	}
	for _, security := range days {
		slices.SortFunc(security, func(a, b acbAdjustmentDay) int { return a.date.Compare(b.date) })
	}

	return days, issues
}

// addToDay puts item in the day of days it is dated, opening that day when there is none.
func addToDay(days []acbAdjustmentDay, item acbItem) []acbAdjustmentDay {
	for i := range days {
		if days[i].date.Equal(item.date) {
			days[i].items = append(days[i].items, item)

			return days
		}
	}

	return append(days, acbAdjustmentDay{date: item.date, items: []acbItem{item}})
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

// accountNames is each account's name by id.
func accountNames(accounts []store.Account) map[string]string {
	names := make(map[string]string, len(accounts))
	for _, a := range accounts {
		names[a.ID] = a.Name
	}

	return names
}

// securityWalk is one security's pool as its transactions and adjustments are applied.
type securityWalk struct {
	security store.Security
	rates    []store.Rate
	names    map[string]string
	pool     acbPool
	position ACBSecurity
	sales    []acbSale
	excesses []acbExcess
	issues   []ACBAdjustmentIssue
	splitDay time.Time
	// unknownCost is whether the pool holds shares added with no cost: opened by such an addition, closed
	// when a disposition empties the pool.
	unknownCost bool
}

// walkSecurity applies txs, in date, tier then source id order, to one pool, each of days' adjustments between
// its day's splits and dispositions; rates convert foreign amounts, names are account names by id.
func walkSecurity(security store.Security, txs []store.InvestmentTransaction, days []acbAdjustmentDay, rates []store.Rate, names map[string]string) *securityWalk {
	slices.SortFunc(txs, func(a, b store.InvestmentTransaction) int {
		return cmp.Or(
			a.Date.Compare(b.Date),
			cmp.Compare(acbTiers[a.Action], acbTiers[b.Action]),
			cmp.Compare(a.SourceID, b.SourceID),
		)
	})

	w := &securityWalk{
		security: security, rates: rates, names: names, pool: acbPool{shares: new(big.Rat)}, position: ACBSecurity{Security: security},
	}
	for _, tx := range txs {
		for len(days) > 0 && (days[0].date.Before(tx.Date) || days[0].date.Equal(tx.Date) && acbTiers[tx.Action] == acbDisposition) {
			w.adjust(days[0])
			days = days[1:]
		}
		w.apply(tx)
	}
	for _, day := range days {
		w.adjust(day)
	}
	w.position.Shares, w.position.ACB, w.position.Incomplete = w.pool.shares, w.pool.acb, w.unknownCost

	return w
}

// apply puts tx through the pool and records its event, unless it moves nothing.
func (w *securityWalk) apply(tx store.InvestmentTransaction) {
	if movesNoUnits(tx) {
		return
	}
	rate := rateOn(w.rates, tx.Date)
	event := ACBEvent{
		ID: tx.ID, Date: tx.Date, AccountID: tx.AccountID, Account: w.names[tx.AccountID], Action: tx.Action,
		Shares: units(tx.Shares), Amount: &tx.Amount, Currency: tx.Currency, CAD: toCAD(tx.Amount, tx.Currency, rate),
	}
	if currency, _ := money.ParseCurrency(tx.Currency); currency == money.USD {
		event.Rate = rate
	}
	if noCostAcquisition(tx) {
		w.unknownCost, event.UnknownCost = true, true
	}
	switch tx.Action {
	case store.ActionBuy:
		w.pool.add(event.Shares, -toCAD(tx.Amount, tx.Currency, rate))
	case store.ActionReinvestDividend, store.ActionAddShares:
		w.pool.add(event.Shares, toCAD(zeroIfNil(tx.CostBasis), tx.Currency, rate))
	case store.ActionSplit:
		// A split is recorded once per account that held the security; the first row of a day is the split.
		if tx.Date.Equal(w.splitDay) {
			return
		}
		w.splitDay = tx.Date
		splitShares(w.pool.shares, tx.SplitNewShares, tx.SplitOldShares)
	case store.ActionSell:
		// Quicken stores a sale's shares negative; the units sold are their magnitude.
		event.Shares.Abs(event.Shares)
		event.UnknownCost = w.unknownCost
		sale := w.pool.sell(tx, event.Shares, rate)
		sale.row.AccountID, sale.row.Account, sale.row.UnknownCost = event.AccountID, event.Account, event.UnknownCost
		outlays := sale.row.Outlays
		event.Gain, event.Outlays, event.Realized = sale.row.Gain, &outlays, true
		w.sales = append(w.sales, sale)
		w.closeSpanWhenSoldOut()
	case store.ActionRemoveShares:
		// Stored negative like a sale's; the units leave with their share of the ACB and no gain.
		event.Shares.Abs(event.Shares)
		event.UnknownCost = w.unknownCost
		w.pool.take(event.Shares)
		w.closeSpanWhenSoldOut()
	}
	w.record(event)
}

// closeSpanWhenSoldOut ends the no-cost span once a disposition has emptied the pool.
func (w *securityWalk) closeSpanWhenSoldOut() {
	if w.pool.shares.Sign() == 0 {
		w.unknownCost = false
	}
}

// record keeps event with the pool as it left it.
func (w *securityWalk) record(event ACBEvent) {
	event.Held, event.ACB = new(big.Rat).Set(w.pool.shares), w.pool.acb
	w.position.Events = append(w.position.Events, event)
}

// adjust applies one day's items, every reinvested distribution before every return of capital, each in
// request order. An empty pool skips them all; on a held pool all apply, each later item repeating the first.
func (w *securityWalk) adjust(day acbAdjustmentDay) {
	if w.pool.shares.Sign() == 0 {
		for _, item := range day.items {
			w.issues = append(w.issues, w.issue(ACBAdjustmentNotHeld, item.number, day.date))
		}

		return
	}
	for _, item := range day.items[1:] {
		repeat := w.issue(ACBAdjustmentRepeated, item.number, day.date)
		repeat.First = day.items[0].number
		w.issues = append(w.issues, repeat)
	}
	for _, item := range day.items {
		if item.reinvestment > 0 {
			w.reinvestDistribution(day.date, item.reinvestment)
		}
	}
	for _, item := range day.items {
		if item.returnOfCapital > 0 {
			w.returnOfCapital(day.date, item.returnOfCapital)
		}
	}
}

// issue is an issue with the item numbered number, dated date, on this walk's security.
func (w *securityWalk) issue(kind ACBAdjustmentKind, number int, date time.Time) ACBAdjustmentIssue {
	return ACBAdjustmentIssue{Kind: kind, Item: number, SecurityID: w.security.ID, Security: w.security.Name, Date: date}
}

// adjustmentEvent is an event of an adjustment: no transaction, account or amount of its own, and no units moved.
func adjustmentEvent(date time.Time, action string, cad int64) ACBEvent {
	return ACBEvent{Date: date, Action: action, Shares: new(big.Rat), CAD: cad}
}

// reinvestDistribution raises the ACB by cents, the cost of a distribution reinvested.
func (w *securityWalk) reinvestDistribution(date time.Time, cents int64) {
	w.pool.acb += cents
	w.record(adjustmentEvent(date, ACBActionReinvestedDistribution, -cents))
}

// returnOfCapital lowers the ACB by cents; what exceeds the ACB brings it to 0 and is a capital gain.
func (w *securityWalk) returnOfCapital(date time.Time, cents int64) {
	event := adjustmentEvent(date, ACBActionReturnOfCapital, cents)
	if cents > w.pool.acb {
		excess := cents - w.pool.acb
		w.pool.acb = 0
		event.Gain, event.Realized = excess, true
		w.excesses = append(w.excesses, acbExcess{date: date, amount: excess})
	} else {
		w.pool.acb -= cents
	}
	w.record(event)
}

// movesNoUnits is whether tx adds or removes shares that move nothing: no units, or a negative count added.
func movesNoUnits(tx store.InvestmentTransaction) bool {
	switch tx.Action {
	case store.ActionAddShares:
		return units(tx.Shares).Sign() <= 0
	case store.ActionRemoveShares:
		return units(tx.Shares).Sign() == 0
	default:
		return false
	}
}

// noCostAcquisition is whether tx adds shares with no recorded cost: added shares or a reinvested dividend
// of some units and a missing cost basis.
func noCostAcquisition(tx store.InvestmentTransaction) bool {
	switch tx.Action {
	case store.ActionAddShares, store.ActionReinvestDividend:
		return tx.CostBasis == nil && units(tx.Shares).Sign() > 0
	default:
		return false
	}
}

// add puts bought units and their cost into the pool.
func (p *acbPool) add(bought *big.Rat, cost int64) {
	p.shares.Add(p.shares, bought)
	p.acb += cost
}

// take removes units (a magnitude) and their share of the ACB, or all of it when they are all the units held
// or more, and returns the ACB removed.
func (p *acbPool) take(units *big.Rat) int64 {
	removed := p.acb
	if units.Cmp(p.shares) < 0 {
		removed = roundHalfAway(new(big.Rat).Mul(big.NewRat(p.acb, 1), new(big.Rat).Quo(units, p.shares)))
		p.shares.Sub(p.shares, units)
		p.acb -= removed
	} else {
		p.shares.SetInt64(0)
		p.acb = 0
	}

	return removed
}

// sell takes sold units (a magnitude) out of the pool and returns the sale in CAD.
func (p *acbPool) sell(tx store.InvestmentTransaction, sold *big.Rat, rate money.Rate) acbSale {
	removed := p.take(sold)

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

// acbExcess is a return of capital above the ACB: a capital gain of amount on date.
type acbExcess struct {
	date   time.Time
	amount int64
}

// acbYears groups sales and excesses by the calendar year of their date, oldest year first, each year's
// sales in date, then source id order. A year with only an excess has no sales.
func acbYears(sales []acbSale, excesses []acbExcess) []ACBYear {
	slices.SortFunc(sales, func(a, b acbSale) int {
		return cmp.Or(a.row.Date.Compare(b.row.Date), cmp.Compare(a.sourceID, b.sourceID))
	})

	var years []ACBYear
	yearOf := func(date time.Time) *ACBYear {
		i, found := slices.BinarySearchFunc(years, date.Year(), func(y ACBYear, year int) int { return cmp.Compare(y.Year, year) })
		if !found {
			years = slices.Insert(years, i, ACBYear{Year: date.Year()})
		}

		return &years[i]
	}
	for _, sale := range sales {
		year := yearOf(sale.row.Date)
		year.Sales = append(year.Sales, sale.row)
		year.Proceeds += sale.row.Proceeds
		year.Outlays += sale.row.Outlays
		year.ACBRemoved += sale.row.ACBRemoved
		year.Gain += sale.row.Gain
	}
	for _, excess := range excesses {
		yearOf(excess.date).ReturnOfCapitalGain += excess.amount
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
