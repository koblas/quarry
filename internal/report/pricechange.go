package report

import (
	"time"

	"github.com/koblas/quarry/internal/store"
)

// PriceChangeMinPct is the percent a charge must differ from the one before it to count as a price change.
const PriceChangeMinPct = 5

// steadyChangeDivisor makes a run steady when its price changes number at most its steps over this.
const steadyChangeDivisor = 4

// tenthsPerPercent converts a percent to tenths of a percent.
const tenthsPerPercent = 10

// PriceChange is a step between two consecutive charges of a series that moved the price.
type PriceChange struct {
	// Date is the later charge's date.
	Date time.Time
	// From and To are the two charges' amounts, in cents.
	From, To int64
	// Tenths is the step as tenths of a percent of From, rounded half away from zero.
	Tenths int64
}

// priceChangesOf is the steps of run that moved the price by more than PriceChangeMinPct.
func priceChangesOf(run []store.Charge) []PriceChange {
	var changes []PriceChange
	for i := 1; i < len(run); i++ {
		from, to := run[i-1].Amount, run[i].Amount
		if abs(to-from)*100 > PriceChangeMinPct*from {
			changes = append(changes, PriceChange{Date: run[i].Date, From: from, To: to, Tenths: changeTenths(from, to)})
		}
	}
	return changes
}

// changeTenths is the change from one amount to another as tenths of a percent of from, half away from zero.
func changeTenths(from, to int64) int64 {
	scaled := (to - from) * 100 * tenthsPerPercent
	rounded := (abs(scaled)*2 + from) / (2 * from)
	if scaled < 0 {
		return -rounded
	}
	return rounded
}

// steady is whether the series' price changes number at most a quarter of its steps.
func (s Series) steady() bool { return steadyRun(s.PriceChanges, s.ChargeCount) }

// steadyRun is whether changes, the price changes of a run of charges, number at most a quarter of its steps.
func steadyRun(changes []PriceChange, charges int) bool {
	return int64(len(changes)) <= int64(charges-1)/steadyChangeDivisor
}

// steadyCharges is whether run's price changes number at most a quarter of its steps.
func steadyCharges(run []store.Charge) bool { return steadyRun(priceChangesOf(run), len(run)) }

// abs is the magnitude of n.
func abs(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}
