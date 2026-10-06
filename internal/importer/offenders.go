package importer

import (
	"fmt"
	"sort"
	"time"

	"github.com/koblas/quarry/internal/platform/humanize"
)

// unmappableClass is what makes a row unmappable; each class has its own
// refusal wording. Constants are declared in reporting order: a lower class
// is reported ahead of any higher one.
type unmappableClass int

const (
	classMissingEntity        unmappableClass = iota // a Core Data entity quarry needs is absent
	classCurrency                                    // an account in an unsupported currency
	classAccountType                                 // an account type quarry does not map
	classTransactionPrecision                        // a transaction, investment transaction or lot value (shares, amount, commission, cost basis) beyond its decimals' snap tolerance
	classSplitPrecision                              // a split amount with more than 2 decimals beyond the snap tolerance
	classStatementPrecision                          // a statement balance with more than 2 decimals beyond the snap tolerance
	classTooLarge                                    // an amount outside quarry's range
	classNotANumber                                  // an amount stored as text or blob
	classTransactionStatus                           // a reconcile status quarry does not map
	classCategoryType                                // a category type quarry does not map
	classMissingValue                                // a required value or reference is missing, or a split ratio quarry cannot read
)

// offender is one row Import cannot map, in one unmappableClass. reason is that
// row's rendered text, without " (and N more)". Undated offenders (an
// account or category, identified by name) sort before every dated one (a
// transaction or split, identified by date); within each group the spec's
// own ordering applies.
type offender struct {
	class    unmappableClass
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

// firstError picks the lowest class present and that class's first offender
// by lessOffender, returning an *UnmappableError whose reason appends
// " (and N more)" for the class's other offenders.
func (o *offenders) firstError() error {
	if o.empty() {
		return nil
	}

	best := o.items[0].class
	for _, it := range o.items[1:] {
		if it.class < best {
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
