package report

import (
	"cmp"
	"context"
	"slices"
	"strings"
	"time"

	"github.com/koblas/quarry/internal/finding"
	"github.com/koblas/quarry/internal/store"
)

// findingsCommand names findings in its refusals; it equals the cli command word.
const findingsCommand = "findings"

// FindingsRequest is what a findings read needs from its caller: the ids of the findings the
// user ignores.
type FindingsRequest struct {
	Ignore []string
}

// FindingsGroup is the open findings of one type, in display order.
type FindingsGroup struct {
	Type     finding.Type
	Findings []store.Finding
}

// FindingsListing is the open findings grouped by type in display order, the tallies over every
// finding of a known type, and the findings.ignore elements that name none of them, in file order.
// A type with no open finding has no group.
type FindingsListing struct {
	Groups    []FindingsGroup
	Counts    finding.Counts
	Unmatched []string
}

// Findings lists the open findings, grouped in finding.Types order and sorted within each group,
// and counts every finding of a known type by status: an id in req.Ignore is ignored unless fixed.
// It refuses like Status.
func (s *Server) Findings(ctx context.Context, req FindingsRequest) (FindingsListing, error) {
	list, err := s.store.Findings(ctx)
	if err != nil {
		return FindingsListing{}, s.readRefusal(ctx, findingsCommand, err)
	}

	known := map[finding.Type]bool{}
	for _, typ := range finding.Types() {
		known[typ] = true
	}
	var stored []store.Finding
	var states []finding.State
	for _, f := range list.Findings {
		if !known[f.Type] {
			continue
		}
		stored = append(stored, f)
		states = append(states, finding.State{ID: f.ID, Fixed: f.FixedAt != nil, New: f.New, NewlyFixed: f.NewlyFixed})
	}
	classified := finding.Classify(states, req.Ignore)

	open := map[finding.Type][]store.Finding{}
	for i, f := range stored {
		if classified.Statuses[i] == finding.StatusOpen {
			open[f.Type] = append(open[f.Type], f)
		}
	}
	var groups []FindingsGroup
	for _, typ := range finding.Types() {
		if len(open[typ]) == 0 {
			continue
		}
		slices.SortStableFunc(open[typ], findingOrder(typ))
		groups = append(groups, FindingsGroup{Type: typ, Findings: open[typ]})
	}
	return FindingsListing{Groups: groups, Counts: classified.Counts, Unmatched: classified.Unmatched}, nil
}

// findingOrder is the display order of open findings of typ: duplicate and one-sided by latest item
// date descending, uncategorized by item count descending then payee; ties and other types by id.
func findingOrder(typ finding.Type) func(a, b store.Finding) int {
	switch typ { //nolint:exhaustive // every other type sorts by id
	case finding.Duplicate, finding.OneSidedTransfer:
		return func(a, b store.Finding) int {
			return cmp.Or(latestDate(b).Compare(latestDate(a)), cmp.Compare(a.ID, b.ID))
		}
	case finding.Uncategorized:
		return func(a, b store.Finding) int {
			return cmp.Or(
				cmp.Compare(len(b.Items), len(a.Items)),
				cmp.Compare(strings.ToLower(payeeOf(a)), strings.ToLower(payeeOf(b))),
				cmp.Compare(a.ID, b.ID),
			)
		}
	}
	return func(a, b store.Finding) int { return cmp.Compare(a.ID, b.ID) }
}

// latestDate is the date of f's latest item; the zero time when f has none.
func latestDate(f store.Finding) time.Time {
	var latest time.Time
	for _, item := range f.Items {
		if item.Date.After(latest) {
			latest = item.Date
		}
	}
	return latest
}

// payeeOf is the payee name of f's first item; every item of an uncategorized finding shares it.
func payeeOf(f store.Finding) string {
	if len(f.Items) == 0 {
		return ""
	}
	return f.Items[0].Payee
}
