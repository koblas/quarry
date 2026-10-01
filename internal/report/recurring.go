package report

import (
	"cmp"
	"context"
	"slices"
	"strings"
	"time"

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

// The numbers of each cadence's rule: the days between two charges (both ends inclusive), the
// fewest charges, the most days since the last charge a series stays active, and the charges in a year.
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
// series to list and the clock reading that decides today.
type RecurringRequest struct {
	Window store.Window
	Now    time.Time
}

// Series is one detected recurring charge: its latest run of charges at one cadence.
type Series struct {
	Payee    string
	Currency string
	Cadence  Cadence
	// Amount is the latest charge, in cents.
	Amount int64
	// PerYear is Amount times the cadence's charges a year; nil for an ended series.
	PerYear *int64
	// First and Last are the dates of the run's first and latest charge.
	First, Last time.Time
	ChargeCount int
	State       SeriesState

	key groupKey
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
}

// recurringCommand names recurring in its refusals; it equals the cli command word.
const recurringCommand = "recurring"

// Recurring lists the series detected over every charge dated through the day req.Now falls on,
// that were running at any time in req.Window. A store it cannot read is a RefusalError.
func (s *Server) Recurring(ctx context.Context, req RecurringRequest) (Recurring, error) {
	today := DefaultWindow(req.Now).Until
	charges, err := s.store.Charges(ctx, store.ChargeParams{Through: today})
	if err != nil {
		return Recurring{}, s.readRefusal(ctx, recurringCommand, err)
	}
	result := Recurring{Window: req.Window}
	for _, group := range groupCharges(charges.Rows) {
		run, rule, ok := latestRun(group.charges)
		if !ok {
			continue
		}
		if series := seriesOf(group.key, run, rule, today); series.runsDuring(req.Window, today) {
			result.Series = append(result.Series, series)
		}
	}
	slices.SortStableFunc(result.Series, compareSeries)
	result.Totals = yearlyTotals(result.Series)
	return result, nil
}

// seriesOf is the series a run of charges makes as of today: its latest charge gives the payee
// and amount, and the days since that charge give the state.
func seriesOf(key groupKey, run []store.Charge, rule cadenceRule, today time.Time) Series {
	latest := run[len(run)-1]
	series := Series{
		Payee:       *latest.Payee,
		Currency:    latest.Currency,
		Cadence:     rule.cadence,
		Amount:      latest.Amount,
		First:       run[0].Date,
		Last:        latest.Date,
		ChargeCount: len(run),
		key:         key,
	}
	if daysBetween(latest.Date, today) > rule.endedAfter {
		series.State = SeriesEnded
		return series
	}
	perYear := latest.Amount * rule.perYear
	series.PerYear = &perYear
	return series
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

// compareSeries orders series by currency, active before ended, yearly cost descending (ended:
// last charge descending), lower-cased payee, then group key.
func compareSeries(a, b Series) int {
	return cmp.Or(
		cmp.Compare(a.Currency, b.Currency),
		cmp.Compare(a.State, b.State),
		compareStanding(a, b),
		cmp.Compare(strings.ToLower(a.Payee), strings.ToLower(b.Payee)),
		cmp.Compare(a.key.value, b.key.value),
	)
}

// compareStanding puts the costlier of two active series first, and the more recent of two ended
// ones; compareSeries calls it only after the states tie, so an ended series' nil yearly cost is never read.
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
