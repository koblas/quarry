package report

import (
	"cmp"
	"context"
	"slices"
	"time"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/store"
)

// The thresholds a charge must pass to be listed; amounts are cents.
const (
	// AnomalyPayeeMinHistory is how many earlier charges a payee needs before it has a baseline.
	AnomalyPayeeMinHistory = 3
	// AnomalyPayeeMultiplier is how many times the payee's median a charge must exceed to be listed.
	AnomalyPayeeMultiplier = 2
	// AnomalyMinAmount is the amount below which a charge is never listed (100.00).
	AnomalyMinAmount int64 = 10000
	// AnomalyCategoryMinHistory is how many earlier charges a category needs before it has a baseline.
	AnomalyCategoryMinHistory = 10
	// AnomalyCategoryMultiplier is how many times the category's median a charge must exceed to be listed.
	AnomalyCategoryMultiplier = 5
)

// anomaliesCommand names anomalies in its refusals; it equals the cli command word.
const anomaliesCommand = "anomalies"

// AnomalyBaseline is what an anomaly's charge was compared with.
type AnomalyBaseline int

// The baselines an anomaly can be judged against.
const (
	// BaselinePayee compares a charge with the payee's earlier charges.
	BaselinePayee AnomalyBaseline = iota
	// BaselineCategory compares a charge with the earlier charges of its category, when its payee has too few.
	BaselineCategory
)

// AnomaliesRequest is what an anomalies read needs from its caller: the window of charges to
// list, the clock reading that decides today, and the accounts whose charges to list and count
// (each an id or a name; none means every account).
type AnomaliesRequest struct {
	Window   store.Window
	Now      time.Time
	Accounts []string
	// Currency is the currency Amount and Usual are listed in; Native lists every anomaly in its own.
	Currency money.Currency
}

// Anomaly is one charge unusually large for its baseline.
type Anomaly struct {
	store.Charge

	Baseline AnomalyBaseline
	// Usual is the median of the baseline's earlier charges, in cents.
	Usual int64
	// Earlier is how many earlier charges the baseline held.
	Earlier int
	// TimesTenths is the charge as a multiple of Usual, in tenths.
	TimesTenths int64
	// ListedCurrency is the currency ListedAmount and ListedUsual are in; empty when the anomaly is listed in its own currency,
	// Amount and Usual.
	ListedCurrency string
	// ListedAmount and ListedUsual are Amount and Usual in ListedCurrency, at the charge's own rate.
	ListedAmount, ListedUsual int64
}

// Anomalies is an anomalies read: the window it listed and the charges unusually large in it.
type Anomalies struct {
	Window store.Window
	// Accounts is the accounts the request named, in the order given and without repeats; none when it named none.
	Accounts []store.Account
	Listed   []Anomaly
	// Checked is how many charges the window held in the listed accounts; NotJudged is how many of them, of 100.00 or more, had no baseline.
	Checked, NotJudged int
	// Transactions is store.Charges.Transactions: the span of the named accounts' transactions, or of the store's when none is named.
	Transactions store.TransactionRange
	// Currency is the currency the request asked the anomalies to be listed in.
	Currency money.Currency
	// Unconverted counts the listed anomalies shown in their own currency for want of a rate.
	Unconverted store.Unconverted
}

// Anomalies lists the charges dated in req.Window, in the accounts req.Accounts names (every account when
// none), that are unusually large for their payee, or for their category when the payee has little history.
// Baselines hold strictly earlier charges from every account. An account it cannot pick, or a store it
// cannot read, is a RefusalError.
func (s *Server) Anomalies(ctx context.Context, req AnomaliesRequest) (Anomalies, error) {
	accounts, accountIDs, err := s.namedAccounts(ctx, anomaliesCommand, req.Accounts)
	if err != nil {
		return Anomalies{}, err
	}
	today := DefaultWindow(req.Now).Until
	charges, err := s.store.Charges(ctx, store.ChargeParams{Through: today, AccountIDs: accountIDs})
	if err != nil {
		return Anomalies{}, s.readRefusal(ctx, anomaliesCommand, err)
	}
	result := Anomalies{Window: req.Window, Accounts: accounts, Currency: req.Currency, Transactions: charges.Transactions}
	tallied := func(c store.Charge) bool {
		return inWindow(req.Window, c.Date) && (len(accountIDs) == 0 || slices.Contains(accountIDs, c.Account.ID))
	}
	categories := groupByCategory(charges.Rows)
	for _, c := range charges.Rows {
		if _, ok := chargeKey(c); !ok && tallied(c) {
			result.tally(judge(c, nil, categories.before(c)))
		}
	}
	for _, group := range groupCharges(charges.Rows) {
		dayStart := 0
		for i, c := range group.charges {
			if i > 0 && !c.Date.Equal(group.charges[i-1].Date) {
				dayStart = i
			}
			if tallied(c) {
				result.tally(judge(c, group.charges[:dayStart], categories.before(c)))
			}
		}
	}
	for i, an := range result.Listed {
		converted, ok := an.listedIn(req.Currency)
		result.Listed[i] = converted
		if !ok && isCADOrUSD(an.Currency) {
			result.Unconverted.Transactions++
		}
	}
	if req.Currency != money.Native {
		result.Unconverted.FirstRate = charges.FirstRate
	}
	slices.SortFunc(result.Listed, compareAnomalies)
	return result, nil
}

// listedIn is a with Amount and Usual converted into target at the rate of the charge's own date. Both convert or
// neither does; the result is false when a stays in its own currency for want of a rate.
func (a Anomaly) listedIn(target money.Currency) (Anomaly, bool) {
	if target == money.Native {
		return a, true
	}
	amount, amountOK := chargeIn(a.Charge, target)
	own, _ := money.ParseCurrency(a.Currency)
	usual, usualOK := money.Convert(a.Usual, own, target, a.USDCAD)
	if !amountOK || !usualOK {
		return a, false
	}
	a.ListedCurrency, a.ListedAmount, a.ListedUsual = target.String(), amount, usual
	return a, true
}

// isCADOrUSD is whether currency is one a rate can convert.
func isCADOrUSD(currency string) bool {
	return currency == money.CAD.String() || currency == money.USD.String()
}

// tally counts a judged charge, and lists it when it is unusual.
func (a *Anomalies) tally(anomaly Anomaly, v verdict) {
	a.Checked++
	switch v {
	case verdictUnusual:
		a.Listed = append(a.Listed, anomaly)
	case verdictNotJudged:
		a.NotJudged++
	case verdictUsual:
	}
}

// verdict is how one charge came out.
type verdict int

const (
	// verdictUsual is a judged charge that is not unusual, one under AnomalyMinAmount included.
	verdictUsual verdict = iota
	// verdictUnusual is a charge over its baseline's multiplier times the baseline's median.
	verdictUnusual
	// verdictNotJudged is a charge of AnomalyMinAmount or more with no baseline.
	verdictNotJudged
)

// baseline is the earlier charges a charge is compared with, and how far above their median it must be.
type baseline struct {
	kind       AnomalyBaseline
	earlier    []store.Charge
	multiplier int64
}

// baselineFor is the baseline of a charge: its payee's earlier charges when there are enough, else its
// category's; ok is false when neither is.
func baselineFor(payee, category []store.Charge) (baseline, bool) {
	switch {
	case len(payee) >= AnomalyPayeeMinHistory:
		return baseline{BaselinePayee, payee, AnomalyPayeeMultiplier}, true
	case len(category) >= AnomalyCategoryMinHistory:
		return baseline{BaselineCategory, category, AnomalyCategoryMultiplier}, true
	}
	return baseline{}, false
}

// judge is the verdict on c, given its payee's charges and its category's charges on strictly earlier
// dates, and the anomaly it makes when unusual.
func judge(c store.Charge, payee, category []store.Charge) (Anomaly, verdict) {
	if c.Amount < AnomalyMinAmount {
		return Anomaly{}, verdictUsual
	}
	base, ok := baselineFor(payee, category)
	if !ok {
		return Anomaly{}, verdictNotJudged
	}
	amounts := make([]int64, len(base.earlier))
	for i, e := range base.earlier {
		amounts[i] = e.Amount
	}
	usual := medianCents(amounts)
	if c.Amount <= base.multiplier*usual {
		return Anomaly{}, verdictUsual
	}
	return Anomaly{
		Charge:      c,
		Baseline:    base.kind,
		Usual:       usual,
		Earlier:     len(base.earlier),
		TimesTenths: timesTenths(c.Amount, usual),
	}, verdictUnusual
}

// categoryKey is what charges of one category baseline share: the category and the currency.
type categoryKey struct {
	category string
	currency string
}

// categoryCharges is the single-category charges of each category and currency, oldest first.
type categoryCharges map[categoryKey][]store.Charge

// groupByCategory groups charges, already ordered by date, by category and currency; a charge with no
// single category is left out.
func groupByCategory(charges []store.Charge) categoryCharges {
	groups := categoryCharges{}
	for _, c := range charges {
		if c.Category != nil {
			key := categoryKey{c.Category.ID, c.Currency}
			groups[key] = append(groups[key], c)
		}
	}
	return groups
}

// before is the charges of c's category and currency dated strictly before c; nil when c has no single category.
func (g categoryCharges) before(c store.Charge) []store.Charge {
	if c.Category == nil {
		return nil
	}
	group := g[categoryKey{c.Category.ID, c.Currency}]
	n, _ := slices.BinarySearchFunc(group, c.Date, func(e store.Charge, day time.Time) int { return e.Date.Compare(day) })
	return group[:n]
}

// tenthsPerWhole converts a whole multiple to tenths.
const tenthsPerWhole = 10

// timesTenths is amount as a multiple of usual in tenths, half away from zero.
func timesTenths(amount, usual int64) int64 {
	return (amount*tenthsPerWhole*2 + usual) / (2 * usual)
}

// inWindow is whether day falls in window, both ends included.
func inWindow(window store.Window, day time.Time) bool {
	return !day.Before(window.Since) && !day.After(window.Until)
}

// compareAnomalies orders anomalies newest first, then by descending source id.
func compareAnomalies(a, b Anomaly) int {
	return cmp.Or(b.Date.Compare(a.Date), cmp.Compare(b.SourceID, a.SourceID))
}
