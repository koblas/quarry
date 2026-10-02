package report

import (
	"cmp"
	"context"
	"slices"
	"strings"
	"time"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/store"
)

// Cadence is how often a series repeats.
type Cadence int

// The cadences a series can have.
const (
	// CadenceWeekly is a charge about every 7 days.
	CadenceWeekly Cadence = iota
	// CadenceMonthly is a charge about every month.
	CadenceMonthly
	// CadenceQuarterly is a charge about every 3 months.
	CadenceQuarterly
	// CadenceAnnual is a charge about every year.
	CadenceAnnual
)

// Each cadence's rule: gap days between charges (inclusive), fewest charges, quiet-period days, charges a year.
const (
	weeklyMinGapDays, weeklyMaxGapDays = 6, 8
	weeklyMinCharges                   = 4
	weeklyEndedAfterDays               = 14
	weeklyChargesPerYear               = 52

	monthlyMinGapDays, monthlyMaxGapDays = 26, 35
	monthlyMinCharges                    = 3
	monthlyEndedAfterDays                = 45
	monthlyChargesPerYear                = 12

	quarterlyMinGapDays, quarterlyMaxGapDays = 84, 98
	quarterlyMinCharges                      = 3
	quarterlyEndedAfterDays                  = 120
	quarterlyChargesPerYear                  = 4

	annualMinGapDays, annualMaxGapDays = 350, 380
	annualMinCharges                   = 2
	annualEndedAfterDays               = 400
	annualChargesPerYear               = 1
)

// cadenceRule is what makes a run of charges one cadence; its fields are the numbers above.
type cadenceRule struct {
	cadence    Cadence
	minGap     int
	maxGap     int
	minCharges int
	endedAfter int
	perYear    int64
}

// cadenceRules lists each cadence's rule; their gap ranges do not overlap.
var cadenceRules = []cadenceRule{
	{CadenceWeekly, weeklyMinGapDays, weeklyMaxGapDays, weeklyMinCharges, weeklyEndedAfterDays, weeklyChargesPerYear},
	{CadenceMonthly, monthlyMinGapDays, monthlyMaxGapDays, monthlyMinCharges, monthlyEndedAfterDays, monthlyChargesPerYear},
	{CadenceQuarterly, quarterlyMinGapDays, quarterlyMaxGapDays, quarterlyMinCharges, quarterlyEndedAfterDays, quarterlyChargesPerYear},
	{CadenceAnnual, annualMinGapDays, annualMaxGapDays, annualMinCharges, annualEndedAfterDays, annualChargesPerYear},
}

// holds is whether a gap of days is between two charges of the rule's cadence.
func (r cadenceRule) holds(days int) bool { return days >= r.minGap && days <= r.maxGap }

// ruleForGap is the cadence rule whose range holds a gap of days.
func ruleForGap(days int) (cadenceRule, bool) {
	for _, r := range cadenceRules {
		if r.holds(days) {
			return r, true
		}
	}
	return cadenceRule{}, false
}

// daysBetween is the whole civil days from earlier to later, both held as UTC midnight.
func daysBetween(earlier, later time.Time) int { return int(later.Sub(earlier) / (24 * time.Hour)) }

// latestRun is the charges ending the group at the cadence of its last gap; ok is false when no
// cadence holds that gap or the run is too short.
func latestRun(charges []store.Charge) ([]store.Charge, cadenceRule, bool) {
	last := len(charges) - 1
	if last < 1 {
		return nil, cadenceRule{}, false
	}
	rule, ok := ruleForGap(daysBetween(charges[last-1].Date, charges[last].Date))
	if !ok {
		return nil, cadenceRule{}, false
	}
	first := last
	for first > 0 && rule.holds(daysBetween(charges[first-1].Date, charges[first].Date)) {
		first--
	}
	run := charges[first:]
	return run, rule, len(run) >= rule.minCharges
}

// SeriesState is whether a series is still running.
type SeriesState int

// The states of a series.
const (
	// SeriesActive is a series whose last charge is within its cadence's quiet period.
	SeriesActive SeriesState = iota
	// SeriesEnded is a series with no charge for longer than its quiet period.
	SeriesEnded
)

// RecurringRequest is what a recurring read needs from its caller: the window of
// series to list, the clock reading that decides today, and the accounts whose charges
// to list (each an id or a name; none means every account).
type RecurringRequest struct {
	Window   store.Window
	Now      time.Time
	Accounts []string
	// Currency is the currency Amount and PerYear are listed in; Native lists every series in its own.
	Currency money.Currency
}

// Series is one detected recurring charge: its latest run of charges at one cadence.
type Series struct {
	Payee    string
	Currency string
	Cadence  Cadence
	// Amount is the latest charge, in cents; FirstAmount is the run's first. Both are in Currency.
	Amount, FirstAmount int64
	// NativeCurrency, NativeAmount and NativeFirstAmount are the series' own currency and its Amount and
	// FirstAmount in it; they equal Currency, Amount and FirstAmount when the series is not converted.
	NativeCurrency                  string
	NativeAmount, NativeFirstAmount int64
	// PerYear is Amount times the cadence's charges a year; nil for an ended series.
	PerYear *int64
	// First and Last are the dates of the run's first and latest charge.
	First, Last time.Time
	ChargeCount int
	State       SeriesState
	// New is whether the first charge falls on or after the window's start; a listed series
	// always starts by the window's end.
	New bool
	// PriceChanges are the run's steps past PriceChangeMinPct, oldest first; ChangeTenths is the first
	// charge to the latest as tenths of a percent, half away from zero.
	PriceChanges []PriceChange
	ChangeTenths int64
	// PayeeKey is the key the run was grouped by; nil when it was grouped by payee id.
	PayeeKey *string
	// Payees and Accounts are the run's distinct payees and accounts in order of first appearance.
	Payees   []SeriesPayee
	Accounts []store.Account

	key groupKey
	// unconverted is whether a CAD or USD series is listed in its own currency for want of a rate.
	unconverted bool
}

// SeriesPayee is one payee a series was charged by.
type SeriesPayee struct {
	ID, Name string
}

// RecurringTotal is the yearly cost of one currency's active series, in cents.
type RecurringTotal struct {
	Currency string
	PerYear  int64
}

// Recurring is a recurring read: the window it listed, the series and one total
// per currency with an active series.
type Recurring struct {
	Window store.Window
	Series []Series
	Totals []RecurringTotal
	// Accounts is the accounts the request named, in the order given and without repeats;
	// empty means every account.
	Accounts []store.Account
	// Currency is the currency the request asked the series to be listed in.
	Currency money.Currency
	// Unconverted counts the listed series shown in their own currency for want of a rate.
	Unconverted store.Unconverted
	// Transactions is store.Charges.Transactions: the span of the store's transactions, or of the named accounts'.
	Transactions store.TransactionRange
}

// Empty is whether no series was listed.
func (r Recurring) Empty() bool { return len(r.Series) == 0 }

// recurringCommand names recurring in its refusals; it equals the cli command word.
const recurringCommand = "recurring"

// Recurring lists the series detected over every charge dated through the day req.Now falls on,
// that were running at any time in req.Window and, when req.Accounts names any, charged in one of
// those accounts at least once. An account it cannot pick, or a store it cannot read, is a RefusalError.
func (s *Server) Recurring(ctx context.Context, req RecurringRequest) (Recurring, error) {
	accounts, accountIDs, err := s.namedAccounts(ctx, recurringCommand, req.Accounts)
	if err != nil {
		return Recurring{}, err
	}
	today := DefaultWindow(req.Now).Until
	charges, err := s.store.Charges(ctx, store.ChargeParams{Through: today, AccountIDs: accountIDs})
	if err != nil {
		return Recurring{}, s.readRefusal(ctx, recurringCommand, err)
	}
	result := Recurring{Window: req.Window, Accounts: accounts, Currency: req.Currency, Transactions: charges.Transactions}
	for _, group := range groupCharges(charges.Rows) {
		run, rule, ok := latestRun(group.charges)
		if !ok {
			continue
		}
		series := seriesOf(group.key, run, rule, today, req.Currency)
		if !series.steady() || !series.runsDuring(req.Window, today) || !series.chargedIn(accountIDs) {
			continue
		}
		series.New = !series.First.Before(req.Window.Since)
		result.Series = append(result.Series, series)
		if series.unconverted {
			result.Unconverted.Transactions++
		}
	}
	if req.Currency != money.Native {
		result.Unconverted.FirstRate = charges.FirstRate
	}
	slices.SortStableFunc(result.Series, compareSeries)
	result.Totals = yearlyTotals(result.Series)
	return result, nil
}

// chargeIn is c's amount in target, in cents; ok is false when target is a currency c has no converted amount in.
// Native, and a charge already in target, need no conversion.
func chargeIn(c store.Charge, target money.Currency) (int64, bool) {
	if target == money.Native || c.Currency == target.String() {
		return c.Amount, true
	}
	cell := c.AmountCAD
	if target == money.USD {
		cell = c.AmountUSD
	}
	if cell == nil {
		return 0, false
	}
	return *cell, true
}

// seriesOf is the series a run of charges makes as of today, listed in target; the run is judged in its
// own currency, and one whose first or latest charge cannot be converted stays in it.
func seriesOf(key groupKey, run []store.Charge, rule cadenceRule, today time.Time, target money.Currency) Series {
	latest := run[len(run)-1]
	series := Series{
		Payee:             *latest.Payee,
		Currency:          latest.Currency,
		Cadence:           rule.cadence,
		Amount:            latest.Amount,
		FirstAmount:       run[0].Amount,
		NativeCurrency:    latest.Currency,
		NativeAmount:      latest.Amount,
		NativeFirstAmount: run[0].Amount,
		First:             run[0].Date,
		Last:              latest.Date,
		ChargeCount:       len(run),
		PriceChanges:      priceChangesOf(run),
		ChangeTenths:      changeTenths(run[0].Amount, latest.Amount),
		key:               key,
	}
	series = series.listedIn(target, run[0], latest)
	series.Payees, series.Accounts = identitiesOf(run)
	if key.kind == keyByPayeeKey {
		series.PayeeKey = &key.value
	}
	if daysBetween(latest.Date, today) > rule.endedAfter {
		series.State = SeriesEnded
		return series
	}
	perYear := series.Amount * rule.perYear
	series.PerYear = &perYear
	return series
}

// listedIn is s with Amount and FirstAmount converted into target from the converted amounts of the run's
// first and latest charge. A CAD or USD series it cannot convert is marked unconverted; any other currency has no rate to want.
func (s Series) listedIn(target money.Currency, first, latest store.Charge) Series {
	amount, amountOK := chargeIn(latest, target)
	firstAmount, firstOK := chargeIn(first, target)
	if amountOK && firstOK {
		if target != money.Native {
			s.Currency = target.String()
		}
		s.Amount, s.FirstAmount = amount, firstAmount
		return s
	}
	s.unconverted = isCADOrUSD(latest.Currency)
	return s
}

// runsDuring is whether the series ran at any time in window: from its first charge to today when
// active, to its last charge when ended.
func (s Series) runsDuring(window store.Window, today time.Time) bool {
	end := s.Last
	if s.State == SeriesActive {
		end = today
	}
	return !s.First.After(window.Until) && !end.Before(window.Since)
}

// chargedIn is whether the series was charged in one of the accounts ids names; no ids names every account.
func (s Series) chargedIn(ids []string) bool {
	if len(ids) == 0 {
		return true
	}
	return slices.ContainsFunc(s.Accounts, func(a store.Account) bool { return slices.Contains(ids, a.ID) })
}

// compareSeries orders series by currency, state, yearly cost descending (ended: last charge descending),
// payee, group key, own-currency before converted, then native currency.
func compareSeries(a, b Series) int {
	return cmp.Or(
		cmp.Compare(a.Currency, b.Currency),
		cmp.Compare(a.State, b.State),
		compareStanding(a, b),
		cmp.Compare(strings.ToLower(a.Payee), strings.ToLower(b.Payee)),
		cmp.Compare(a.key.value, b.key.value),
		cmp.Compare(a.convertedRank(), b.convertedRank()),
		// Tie-breaker only: no CAD/USD input reaches it.
		cmp.Compare(a.NativeCurrency, b.NativeCurrency),
	)
}

// convertedRank is 0 for a series listed in its own currency and 1 for a converted one.
func (s Series) convertedRank() int {
	if s.NativeCurrency == s.Currency {
		return 0
	}
	return 1
}

// compareStanding puts the costlier of two active series first, and the more recent of two ended ones.
func compareStanding(a, b Series) int {
	if a.PerYear != nil && b.PerYear != nil {
		return cmp.Compare(*b.PerYear, *a.PerYear)
	}
	return b.Last.Compare(a.Last)
}

// yearlyTotals sums the yearly cost of each currency's active series; series are sorted by currency.
func yearlyTotals(series []Series) []RecurringTotal {
	var totals []RecurringTotal
	for _, s := range series {
		if s.State != SeriesActive {
			continue
		}
		if last := len(totals) - 1; last >= 0 && totals[last].Currency == s.Currency {
			totals[last].PerYear += *s.PerYear
			continue
		}
		totals = append(totals, RecurringTotal{Currency: s.Currency, PerYear: *s.PerYear})
	}
	return totals
}
