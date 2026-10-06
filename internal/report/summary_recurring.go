package report

import (
	"sort"
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

// resumes is whether run, the group's latest, starts again a steady series that had not ended before it
// and that a recurring read could have listed as of its own last charge.
func resumes(group, run []store.Charge) bool {
	earlier, rule, ok := earlierRun(group[:len(group)-len(run)], run[0].Date)
	if !ok || !steadyCharges(earlier) {
		return false
	}
	last := earlier[len(earlier)-1].Date
	return daysBetween(last, run[0].Date) <= rule.endedAfter && listableAt(group, last)
}

// listableAt is whether the group's charges dated on or before day end in a run a recurring read lists.
func listableAt(group []store.Charge, day time.Time) bool {
	end := sort.Search(len(group), func(i int) bool { return group[i].Date.After(day) })
	_, _, ok := latestRun(group[:end])
	return ok
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
