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

// FindingsAll is the FindingsRequest.Status that selects every status.
const FindingsAll finding.Status = "all"

// FindingsRequest is what a findings read needs from its caller: the ids of the findings the user
// ignores, which status to list (open when empty; finding.StatusOpen, StatusIgnored, StatusFixed or
// FindingsAll) and which type (every type when empty).
type FindingsRequest struct {
	Ignore []string
	Status finding.Status
	Type   finding.Type
}

// ListedFinding is a finding with its status; FixedAt is non-nil exactly when Status is fixed.
type ListedFinding struct {
	store.Finding

	Status finding.Status
}

// FindingsGroup is the listed findings of one type in display order: open, then ignored, then fixed.
type FindingsGroup struct {
	Type     finding.Type
	Findings []ListedFinding
}

// FindingsListing is the selected findings grouped by type in display order, the tallies over every
// finding of the requested type (every known type when none is), and the findings.ignore elements
// that name no finding of any known type, in file order. A type with nothing listed has no group.
type FindingsListing struct {
	Groups    []FindingsGroup
	Counts    finding.Counts
	Unmatched []string
}

// Findings lists the findings req selects, grouped in finding.Types order and sorted within each
// group, and tallies them by status: an id in req.Ignore is ignored unless fixed. It refuses like Status.
func (s *Server) Findings(ctx context.Context, req FindingsRequest) (FindingsListing, error) {
	list, err := s.store.Findings(ctx)
	if err != nil {
		return FindingsListing{}, s.readRefusal(ctx, findingsCommand, err)
	}
	want := cmp.Or(req.Status, finding.StatusOpen)

	known := map[finding.Type]bool{}
	for _, typ := range finding.Types() {
		known[typ] = true
	}
	var stored []store.Finding
	var states, typeStates []finding.State
	for _, f := range list.Findings {
		if !known[f.Type] {
			continue
		}
		state := finding.State{ID: f.ID, Fixed: f.FixedAt != nil, New: f.New, NewlyFixed: f.NewlyFixed}
		stored = append(stored, f)
		states = append(states, state)
		if req.Type == "" || f.Type == req.Type {
			typeStates = append(typeStates, state)
		}
	}
	// Unmatched ids are judged against every known finding, so a --type filter never hides one.
	classified := finding.Classify(states, req.Ignore)
	counts := finding.Classify(typeStates, req.Ignore).Counts

	selected := map[finding.Type][]ListedFinding{}
	for i, f := range stored {
		status := classified.Statuses[i]
		if (req.Type != "" && f.Type != req.Type) || (want != FindingsAll && status != want) {
			continue
		}
		selected[f.Type] = append(selected[f.Type], ListedFinding{Finding: f, Status: status})
	}
	var groups []FindingsGroup
	for _, typ := range finding.Types() {
		if len(selected[typ]) == 0 {
			continue
		}
		slices.SortStableFunc(selected[typ], listedOrder(typ))
		groups = append(groups, FindingsGroup{Type: typ, Findings: selected[typ]})
	}
	return FindingsListing{Groups: groups, Counts: counts, Unmatched: classified.Unmatched}, nil
}

// listedOrder is the display order within a group: open, ignored, then fixed; open and ignored
// findings in findingOrder, fixed ones by fixed_at descending then id.
func listedOrder(typ finding.Type) func(a, b ListedFinding) int {
	byType := findingOrder(typ)
	return func(a, b ListedFinding) int {
		if c := cmp.Compare(statusRank(a.Status), statusRank(b.Status)); c != 0 {
			return c
		}
		if a.Status == finding.StatusFixed {
			return cmp.Or(b.FixedAt.Compare(*a.FixedAt), cmp.Compare(a.ID, b.ID))
		}
		return byType(a.Finding, b.Finding)
	}
}

// statusRank is where a status sorts within a group.
func statusRank(s finding.Status) int {
	switch s { //nolint:exhaustive // open, the remaining status, ranks first
	case finding.StatusIgnored:
		return 1
	case finding.StatusFixed:
		return 2
	}
	return 0
}

// findingOrder is the display order of the open and ignored findings of typ (fixed ones sort apart):
// duplicate and one-sided by latest item date descending, uncategorized by item count descending
// then payee; ties and other types by id.
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
