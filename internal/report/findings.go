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

	stored, states := knownFindings(list)
	var typeStates []finding.State
	for i, f := range stored {
		if req.Type == "" || f.Type == req.Type {
			typeStates = append(typeStates, states[i])
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

// CountFindings tallies the findings st carries, those of a type this binary knows, by status: an
// id in ignore is ignored unless fixed. It reads nothing, so it agrees with the rest of st.
func CountFindings(st store.Status, ignore []string) finding.Counts {
	_, states := knownFindings(store.FindingList{Findings: st.Findings})
	return finding.Classify(states, ignore).Counts
}

// knownFindings is the findings of list whose type this binary knows, with each one's finding.State
// at the same index; rows of any other type are neither listed nor counted.
func knownFindings(list store.FindingList) ([]store.Finding, []finding.State) {
	known := map[finding.Type]bool{}
	for _, typ := range finding.Types() {
		known[typ] = true
	}
	var stored []store.Finding
	var states []finding.State
	for _, f := range list.Findings {
		if known[f.Type] {
			stored = append(stored, f)
			states = append(states, finding.State{ID: f.ID, Fixed: f.FixedAt != nil, New: f.New, NewlyFixed: f.NewlyFixed})
		}
	}
	return stored, states
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

// findingOrder is the display order of typ's open and ignored findings: newest item date for transfers and
// duplicates, size then payee for uncategorized and mixed, transactions for payee-variants, splits for
// similar-categories, category path for unused-category.
func findingOrder(typ finding.Type) func(a, b store.Finding) int {
	switch typ {
	case finding.Duplicate, finding.UnlinkedTransfer, finding.OneSidedTransfer:
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
	case finding.MixedCategories:
		return func(a, b store.Finding) int {
			return cmp.Or(
				cmp.Compare(transactionsOf(b), transactionsOf(a)),
				cmp.Compare(strings.ToLower(payeeOf(a)), strings.ToLower(payeeOf(b))),
				cmp.Compare(a.ID, b.ID),
			)
		}
	case finding.PayeeVariants:
		return func(a, b store.Finding) int {
			return cmp.Or(cmp.Compare(transactionsOf(b), transactionsOf(a)), cmp.Compare(a.ID, b.ID))
		}
	case finding.SimilarCategories:
		return func(a, b store.Finding) int {
			return cmp.Or(cmp.Compare(splitsOf(b), splitsOf(a)), cmp.Compare(a.ID, b.ID))
		}
	case finding.UnusedCategory:
		return func(a, b store.Finding) int {
			return cmp.Or(cmp.Compare(strings.ToLower(categoryOf(a)), strings.ToLower(categoryOf(b))), cmp.Compare(a.ID, b.ID))
		}
	}
	return func(a, b store.Finding) int { return cmp.Compare(a.ID, b.ID) } // unreachable: the cases above cover every finding.Types() entry, and knownFindings drops rows of any other type
}

// transactionsOf is the sum of f's items' Transactions.
func transactionsOf(f store.Finding) int {
	total := 0
	for _, item := range f.Items {
		total += item.Transactions
	}
	return total
}

// splitsOf is the sum of f's items' Splits.
func splitsOf(f store.Finding) int {
	total := 0
	for _, item := range f.Items {
		total += item.Splits
	}
	return total
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

// categoryOf is the path of f's first item, the unused category itself; "" when f has none.
func categoryOf(f store.Finding) string {
	if len(f.Items) == 0 || f.Items[0].Category == nil {
		return "" // unreachable: listedOrder sends fixed findings (the only ones without items) to its fixed_at branch, and full_path is NOT NULL (duckstore/schema.go:30)
	}
	return *f.Items[0].Category
}

// payeeOf is the payee name of f's first item; every item of an uncategorized or mixed-categories finding shares it.
func payeeOf(f store.Finding) string {
	if len(f.Items) == 0 {
		return ""
	}
	return f.Items[0].Payee
}
