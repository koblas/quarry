package report

import (
	"time"

	"github.com/koblas/quarry/internal/store"
)

// newInMonth keeps the series whose listing charge falls in month and that do not resume an earlier series.
func newInMonth(month Month) seriesFilter {
	return func(group, run []store.Charge, rule cadenceRule) bool {
		return inWindow(month.Window(), listingCharge(run, rule).Date) && !resumes(group, run)
	}
}

// listingCharge is the earliest charge of run at which the run so far was a steady series of at least the
// rule's fewest charges; run is steady, so its last charge is the latest that can be.
func listingCharge(run []store.Charge, rule cadenceRule) store.Charge {
	k := rule.minCharges - 1
	for k < len(run)-1 && !steadyCharges(run[:k+1]) {
		k++
	}
	return run[k]
}

// resumes is whether run, the group's latest, starts again a steady series that had not ended before it.
func resumes(group, run []store.Charge) bool {
	earlier, rule, ok := earlierRun(group[:len(group)-len(run)], run[0].Date)
	return ok && steadyCharges(earlier) && daysBetween(earlier[len(earlier)-1].Date, run[0].Date) <= rule.endedAfter
}

// earlierRun is the run ending before once its trailing off-schedule charges are dropped; it does not look
// back past the longest quiet period before first.
func earlierRun(before []store.Charge, first time.Time) ([]store.Charge, cadenceRule, bool) {
	for end := len(before); end > 0 && daysBetween(before[end-1].Date, first) <= annualEndedAfterDays; end-- {
		if run, rule, ok := latestRun(before[:end]); ok {
			return run, rule, true
		}
	}
	return nil, cadenceRule{}, false
}
