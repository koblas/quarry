package importer

import (
	"fmt"
	"sort"
	"time"

	"github.com/koblas/quarry/internal/platform/humanize"
)

// s4ClassOrder is the S4 reporting order: 7, then 1-6, then 11, then 8-10.
var s4ClassOrder = map[int]int{7: 0, 1: 1, 2: 2, 3: 3, 4: 4, 5: 5, 6: 6, 11: 7, 8: 8, 9: 9, 10: 10}

// offender is one row Import cannot map, in one S4 class. reason is that
// row's rendered text, without " (and N more)". Undated offenders (an
// account or category, identified by name) sort before every dated one (a
// transaction or split, identified by date); within each group the spec's
// own ordering applies.
type offender struct {
	class    int
	reason   string
	dated    bool
	date     time.Time
	account  string
	name     string
	sourceID int64
}

// offenders accumulates every row Import cannot map, across every
// validation step, so a refusal reports every offender, not just the
// first.
type offenders struct {
	items []offender
}

func (o *offenders) add(item offender) {
	o.items = append(o.items, item)
}

func (o *offenders) empty() bool { return len(o.items) == 0 }

// firstError picks the lowest-ordered S4 class present (s4ClassOrder),
// that class's first offender by lessOffender, and returns an
// *UnmappableError whose reason appends " (and N more)" for the class's
// other offenders.
func (o *offenders) firstError() error {
	if o.empty() {
		return nil
	}

	best := o.items[0].class
	for _, it := range o.items[1:] {
		if s4ClassOrder[it.class] < s4ClassOrder[best] {
			best = it.class
		}
	}

	var inClass []offender
	for _, it := range o.items {
		if it.class == best {
			inClass = append(inClass, it)
		}
	}
	sort.SliceStable(inClass, func(i, j int) bool { return lessOffender(inClass[i], inClass[j]) })

	reason := inClass[0].reason
	if extra := len(inClass) - 1; extra > 0 {
		reason = fmt.Sprintf("%s (and %s more)", reason, humanize.Thousands(extra))
	}
	return &UnmappableError{Reason: reason}
}

// lessOffender implements the spec's ordering: undated rows first (by
// name, then source id), then dated rows (by date, account, source id).
func lessOffender(a, b offender) bool {
	if a.dated != b.dated {
		return !a.dated
	}
	if a.dated {
		if !a.date.Equal(b.date) {
			return a.date.Before(b.date)
		}
		if a.account != b.account {
			return a.account < b.account
		}
		return a.sourceID < b.sourceID
	}
	if a.name != b.name {
		return a.name < b.name
	}
	return a.sourceID < b.sourceID
}
