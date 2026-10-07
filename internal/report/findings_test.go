package report_test

import (
	"context"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/finding"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	march1  = time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	march2  = time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC)
	march3  = time.Date(2026, 3, 3, 0, 0, 0, 0, time.UTC)
	fixedAt = time.Date(2026, 9, 27, 14, 30, 5, 0, time.UTC)
)

// dated is an open finding of typ with one item per date.
func dated(id string, typ finding.Type, dates ...time.Time) store.Finding {
	f := store.Finding{ID: id, Type: typ}
	for _, d := range dates {
		f.Items = append(f.Items, store.FindingItem{Date: d})
	}
	return f
}

// paid is an open uncategorized finding of payee with the given number of items.
func paid(id, payee string, items int) store.Finding {
	f := store.Finding{ID: id, Type: finding.Uncategorized}
	for range items {
		f.Items = append(f.Items, store.FindingItem{Payee: payee})
	}
	return f
}

// mixed is an open mixed-categories finding of payee with one item per transaction count.
func mixed(id, payee string, counts ...int) store.Finding {
	f := store.Finding{ID: id, Type: finding.MixedCategories}
	for _, n := range counts {
		f.Items = append(f.Items, store.FindingItem{Payee: payee, Transactions: n})
	}
	return f
}

// variants is an open payee-variants finding with one item per transaction count.
func variants(id string, counts ...int) store.Finding {
	f := store.Finding{ID: id, Type: finding.PayeeVariants}
	for _, n := range counts {
		f.Items = append(f.Items, store.FindingItem{Transactions: n})
	}
	return f
}

// similar is an open similar-categories finding with one item per split count.
func similar(id string, counts ...int) store.Finding {
	f := store.Finding{ID: id, Type: finding.SimilarCategories}
	for _, n := range counts {
		f.Items = append(f.Items, store.FindingItem{Splits: n})
	}
	return f
}

// unused is an open unused-category finding whose first item is the category at path.
func unused(id, path string) store.Finding {
	return store.Finding{ID: id, Type: finding.UnusedCategory, Items: []store.FindingItem{{Category: &path}}}
}

func findingsOf(t *testing.T, fs ...store.Finding) report.FindingsListing {
	t.Helper()
	srv := report.NewServer(report.WithStore(fakeStore{findings: store.FindingList{Findings: fs}}))
	got, err := srv.Findings(t.Context(), report.FindingsRequest{})
	require.NoError(t, err)
	return got
}

func idsOf(g report.FindingsGroup) []string {
	ids := make([]string, len(g.Findings))
	for i, f := range g.Findings {
		ids[i] = f.ID
	}
	return ids
}

func Test_findings_groups_open_findings_by_type_in_display_order_and_omits_empty_groups(t *testing.T) {
	got := findingsOf(t,
		dated("uncategorized:payee-1", finding.Uncategorized, march1),
		dated("duplicate:txn-1+txn-2", finding.Duplicate, march1, march1),
		dated("one-sided-transfer:xfer-3", finding.OneSidedTransfer, march1))

	types := make([]finding.Type, len(got.Groups))
	for i, g := range got.Groups {
		types[i] = g.Type
	}
	assert.Equal(t, []finding.Type{finding.Duplicate, finding.OneSidedTransfer, finding.Uncategorized}, types)
}

func Test_findings_sorts_a_duplicate_group_by_the_pairs_later_date_descending_then_id(t *testing.T) {
	got := findingsOf(t,
		dated("duplicate:txn-9+txn-10", finding.Duplicate, march1, march3),
		dated("duplicate:txn-5+txn-6", finding.Duplicate, march2, march2),
		dated("duplicate:txn-7+txn-8", finding.Duplicate, march1, march3),
		dated("duplicate:txn-1+txn-2", finding.Duplicate, march1, march1))

	assert.Equal(t, []string{
		"duplicate:txn-7+txn-8", "duplicate:txn-9+txn-10", "duplicate:txn-5+txn-6", "duplicate:txn-1+txn-2",
	}, idsOf(got.Groups[0]))
}

func Test_findings_sorts_an_unlinked_transfer_group_by_the_pairs_later_date_descending_then_id(t *testing.T) {
	got := findingsOf(t,
		dated("unlinked-transfer:txn-9+txn-10", finding.UnlinkedTransfer, march3, march1),
		dated("unlinked-transfer:txn-5+txn-6", finding.UnlinkedTransfer, march2, march2),
		dated("unlinked-transfer:txn-7+txn-8", finding.UnlinkedTransfer, march1, march3),
		dated("unlinked-transfer:txn-1+txn-2", finding.UnlinkedTransfer, march1, march1))

	assert.Equal(t, []string{
		"unlinked-transfer:txn-7+txn-8", "unlinked-transfer:txn-9+txn-10", "unlinked-transfer:txn-5+txn-6", "unlinked-transfer:txn-1+txn-2",
	}, idsOf(got.Groups[0]))
}

func Test_findings_sorts_a_one_sided_group_by_date_descending_then_id(t *testing.T) {
	got := findingsOf(t,
		dated("one-sided-transfer:xfer-9", finding.OneSidedTransfer, march1),
		dated("one-sided-transfer:xfer-2", finding.OneSidedTransfer, march2),
		dated("one-sided-transfer:xfer-1", finding.OneSidedTransfer, march1))

	assert.Equal(t, []string{"one-sided-transfer:xfer-2", "one-sided-transfer:xfer-1", "one-sided-transfer:xfer-9"}, idsOf(got.Groups[0]))
}

func Test_findings_sorts_an_uncategorized_group_by_item_count_descending_then_payee_ignoring_case_then_id(t *testing.T) {
	got := findingsOf(t,
		paid("uncategorized:payee-1", "amazon", 2),
		paid("uncategorized:payee-2", "Zed", 3),
		paid("uncategorized:payee-4", "Bakery", 2),
		paid("uncategorized:payee-3", "Bakery", 2),
		paid("uncategorized:payee-5", "Bakery", 1))

	assert.Equal(t, []string{
		"uncategorized:payee-2", "uncategorized:payee-1", "uncategorized:payee-3", "uncategorized:payee-4", "uncategorized:payee-5",
	}, idsOf(got.Groups[0]))
}

func Test_findings_sorts_a_mixed_categories_group_by_transaction_sum_descending_then_payee_ignoring_case_then_id(t *testing.T) {
	got := findingsOf(t,
		mixed("mixed-categories:payee-1", "Zed", 10),
		mixed("mixed-categories:payee-2", "amazon", 10),
		mixed("mixed-categories:payee-3", "Bakery", 3, 3),
		mixed("mixed-categories:payee-4", "Bakery", 3, 3),
		mixed("mixed-categories:payee-5", "Cafe", 40),
		mixed("mixed-categories:payee-6", "Dairy", 30, 30))

	assert.Equal(t, []string{
		"mixed-categories:payee-6", "mixed-categories:payee-5", "mixed-categories:payee-2", "mixed-categories:payee-1",
		"mixed-categories:payee-3", "mixed-categories:payee-4",
	}, idsOf(got.Groups[0]))
}

func Test_findings_sorts_a_payee_variants_group_by_transaction_sum_descending_then_id(t *testing.T) {
	got := findingsOf(t,
		variants("payee-variants:a", 3),
		variants("payee-variants:c", 10),
		variants("payee-variants:b", 6, 4),
		variants("payee-variants:d", 40))

	assert.Equal(t, []string{"payee-variants:d", "payee-variants:b", "payee-variants:c", "payee-variants:a"}, idsOf(got.Groups[0]))
}

func Test_findings_sorts_a_similar_categories_group_by_split_sum_descending_then_id(t *testing.T) {
	got := findingsOf(t,
		similar("similar-categories:a", 3),
		similar("similar-categories:c", 10),
		similar("similar-categories:b", 6, 4),
		similar("similar-categories:d", 40))

	assert.Equal(t, []string{"similar-categories:d", "similar-categories:b", "similar-categories:c", "similar-categories:a"}, idsOf(got.Groups[0]))
}

func Test_findings_sorts_unused_categories_by_path_ignoring_case_then_id(t *testing.T) {
	got := findingsOf(t,
		unused("unused-category:cat-1", "Bank"),
		unused("unused-category:cat-9", "auto:parking"),
		unused("unused-category:cat-20", "Zoo"),
		unused("unused-category:cat-10", "Auto:Parking"))

	assert.Equal(t, []string{"unused-category:cat-10", "unused-category:cat-9", "unused-category:cat-1", "unused-category:cat-20"}, idsOf(got.Groups[0]))
}

func Test_findings_counts_ignored_and_fixed_findings_without_listing_them(t *testing.T) {
	fixed := store.Finding{ID: "uncategorized:payee-2", Type: finding.Uncategorized, FixedAt: &fixedAt}
	srv := report.NewServer(report.WithStore(fakeStore{findings: store.FindingList{Findings: []store.Finding{
		dated("duplicate:txn-1+txn-2", finding.Duplicate, march1, march1),
		dated("duplicate:txn-3+txn-4", finding.Duplicate, march1, march1),
		dated("one-sided-transfer:xfer-3", finding.OneSidedTransfer, march1),
		fixed,
	}}}))

	got, err := srv.Findings(t.Context(), report.FindingsRequest{Ignore: []string{"duplicate:txn-3+txn-4"}})

	require.NoError(t, err)
	assert.Equal(t, finding.Counts{Open: 2, Ignored: 1, Fixed: 1}, got.Counts)
	assert.Equal(t, []string{"duplicate:txn-1+txn-2"}, idsOf(got.Groups[0]))
	assert.Equal(t, []string{"one-sided-transfer:xfer-3"}, idsOf(got.Groups[1]))
	assert.Len(t, got.Groups, 2)
}

func Test_findings_counts_a_fixed_finding_that_is_also_ignored_as_fixed(t *testing.T) {
	fixed := store.Finding{ID: "uncategorized:payee-2", Type: finding.Uncategorized, FixedAt: &fixedAt}
	srv := report.NewServer(report.WithStore(fakeStore{findings: store.FindingList{Findings: []store.Finding{fixed}}}))

	got, err := srv.Findings(t.Context(), report.FindingsRequest{Ignore: []string{"uncategorized:payee-2"}})

	require.NoError(t, err)
	assert.Equal(t, finding.Counts{Fixed: 1}, got.Counts)
}

func Test_findings_counts_new_among_open_and_newly_fixed_among_fixed_findings(t *testing.T) {
	newOpen := dated("duplicate:txn-1+txn-2", finding.Duplicate, march1, march1)
	newOpen.New = true
	newIgnored := dated("duplicate:txn-3+txn-4", finding.Duplicate, march1, march1)
	newIgnored.New = true
	oldOpen := dated("one-sided-transfer:xfer-3", finding.OneSidedTransfer, march1)
	justFixed := store.Finding{ID: "uncategorized:payee-2", Type: finding.Uncategorized, FixedAt: &fixedAt, NewlyFixed: true}
	earlierFixed := store.Finding{ID: "uncategorized:payee-3", Type: finding.Uncategorized, FixedAt: &fixedAt}
	srv := report.NewServer(report.WithStore(fakeStore{findings: store.FindingList{
		Findings: []store.Finding{newOpen, newIgnored, oldOpen, justFixed, earlierFixed},
	}}))

	got, err := srv.Findings(t.Context(), report.FindingsRequest{Ignore: []string{"duplicate:txn-3+txn-4"}})

	require.NoError(t, err)
	assert.Equal(t, finding.Counts{Open: 2, Ignored: 1, Fixed: 2, New: 1, NewlyFixed: 1}, got.Counts)
}

func Test_findings_lists_nothing_for_a_store_with_no_findings(t *testing.T) {
	got := findingsOf(t)

	assert.Equal(t, report.FindingsListing{}, got)
}

func Test_findings_refuses_with_the_store_refusal_copy(t *testing.T) {
	openErr := &store.OpenError{Fault: store.OpenFaultMissing, Path: storePath}
	srv := report.NewServer(report.WithStore(fakeStore{err: openErr}), report.WithHome(refusalHome))

	_, err := srv.Findings(t.Context(), report.FindingsRequest{})

	assert.EqualError(t, err, "no store at ~/Library/Application Support/quarry/quarry.duckdb yet; run quarry sync to build it")
}

func Test_findings_returns_a_store_fault_that_is_not_a_refusal_unchanged(t *testing.T) {
	srv := report.NewServer(report.WithStore(fakeStore{err: errDiskRead}), report.WithHome(refusalHome))

	got, err := srv.Findings(t.Context(), report.FindingsRequest{})

	assert.Equal(t, errDiskRead, err)
	assert.Equal(t, report.FindingsListing{}, got)
}

func Test_findings_reports_an_interrupt_before_any_store_refusal(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	srv := report.NewServer(report.WithStore(fakeStore{err: errDiskRead}))

	_, err := srv.Findings(ctx, report.FindingsRequest{})

	assert.EqualError(t, err, "findings interrupted")
}

func Test_findings_sorts_an_uncategorized_finding_with_no_items_after_those_with_items_and_by_id(t *testing.T) {
	got := findingsOf(t,
		paid("uncategorized:payee-2", "", 0),
		paid("uncategorized:payee-3", "Bakery", 1),
		paid("uncategorized:payee-1", "", 0))

	assert.Equal(t, []string{"uncategorized:payee-3", "uncategorized:payee-1", "uncategorized:payee-2"}, idsOf(got.Groups[0]))
}

func Test_findings_neither_counts_nor_lists_a_finding_of_a_type_it_does_not_know(t *testing.T) {
	got := findingsOf(t,
		dated("duplicate:txn-1+txn-2", finding.Duplicate, march1, march1),
		dated("mystery:thing-1", finding.Type("mystery"), march1))

	assert.Equal(t, 1, got.Counts.Open)
	require.Len(t, got.Groups, 1)
	assert.Equal(t, []string{"duplicate:txn-1+txn-2"}, idsOf(got.Groups[0]))
}

func Test_findings_lists_the_ignore_ids_that_match_no_finding(t *testing.T) {
	srv := report.NewServer(report.WithStore(fakeStore{findings: store.FindingList{Findings: []store.Finding{
		dated("duplicate:txn-1+txn-2", finding.Duplicate, march1, march1),
		dated("mystery:thing-1", finding.Type("mystery"), march1),
	}}}))
	ignore := []string{"uncategorized:payee-999", "duplicate:txn-1+txn-2", "mystery:thing-1"}

	got, err := srv.Findings(t.Context(), report.FindingsRequest{Ignore: ignore})

	require.NoError(t, err)
	assert.Equal(t, []string{"uncategorized:payee-999", "mystery:thing-1"}, got.Unmatched)
	assert.Equal(t, finding.Counts{Ignored: 1}, got.Counts)
}

const (
	openDuplicate    = "duplicate:txn-1+txn-2"
	ignoredDuplicate = "duplicate:txn-3+txn-4"
	fixedDuplicate   = "duplicate:txn-5+txn-6"
	openPayee        = "uncategorized:payee-1"
	fixedPayee       = "uncategorized:payee-2"
)

// gone is a fixed finding of typ: it has a fixed_at and no items.
func gone(id string, typ finding.Type, at time.Time) store.Finding {
	return store.Finding{ID: id, Type: typ, FixedAt: &at}
}

// mixedFindings holds an open, an ignored (by ignoreDuplicate) and a fixed duplicate plus an open
// and a fixed uncategorized payee; the ignored pair is later than the open one.
func mixedFindings() []store.Finding {
	return []store.Finding{
		gone(fixedDuplicate, finding.Duplicate, fixedAt),
		dated(ignoredDuplicate, finding.Duplicate, march3, march3),
		dated(openDuplicate, finding.Duplicate, march1, march1),
		gone(fixedPayee, finding.Uncategorized, fixedAt),
		paid(openPayee, "Amazon", 1),
	}
}

func listIDs(listing report.FindingsListing) map[finding.Type][]string {
	ids := map[finding.Type][]string{}
	for _, g := range listing.Groups {
		ids[g.Type] = idsOf(g)
	}
	return ids
}

func findingsWith(t *testing.T, req report.FindingsRequest, fs ...store.Finding) report.FindingsListing {
	t.Helper()
	srv := report.NewServer(report.WithStore(fakeStore{findings: store.FindingList{Findings: fs}}))
	got, err := srv.Findings(t.Context(), req)
	require.NoError(t, err)
	return got
}

func Test_findings_lists_the_findings_of_the_requested_status(t *testing.T) {
	cases := []struct {
		name   string
		status finding.Status
		want   map[finding.Type][]string
	}{
		{name: "no status means open", status: "", want: map[finding.Type][]string{
			finding.Duplicate: {openDuplicate}, finding.Uncategorized: {openPayee},
		}},
		{name: "open", status: finding.StatusOpen, want: map[finding.Type][]string{
			finding.Duplicate: {openDuplicate}, finding.Uncategorized: {openPayee},
		}},
		{name: "ignored", status: finding.StatusIgnored, want: map[finding.Type][]string{
			finding.Duplicate: {ignoredDuplicate},
		}},
		{name: "fixed", status: finding.StatusFixed, want: map[finding.Type][]string{
			finding.Duplicate: {fixedDuplicate}, finding.Uncategorized: {fixedPayee},
		}},
		{name: "all", status: report.FindingsAll, want: map[finding.Type][]string{
			finding.Duplicate:     {openDuplicate, ignoredDuplicate, fixedDuplicate},
			finding.Uncategorized: {openPayee, fixedPayee},
		}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := findingsWith(t, report.FindingsRequest{Status: c.status, Ignore: []string{ignoredDuplicate}}, mixedFindings()...)

			assert.Equal(t, c.want, listIDs(got))
		})
	}
}

func Test_findings_gives_each_listed_finding_its_status(t *testing.T) {
	got := findingsWith(t, report.FindingsRequest{Status: report.FindingsAll, Ignore: []string{ignoredDuplicate}}, mixedFindings()...)

	require.Len(t, got.Groups, 2)
	statuses := make([]finding.Status, len(got.Groups[0].Findings))
	for i, f := range got.Groups[0].Findings {
		statuses[i] = f.Status
	}
	assert.Equal(t, []finding.Status{finding.StatusOpen, finding.StatusIgnored, finding.StatusFixed}, statuses)
}

func Test_findings_lists_only_the_requested_type(t *testing.T) {
	got := findingsWith(t, report.FindingsRequest{Status: report.FindingsAll, Type: finding.Uncategorized}, mixedFindings()...)

	assert.Equal(t, map[finding.Type][]string{finding.Uncategorized: {openPayee, fixedPayee}}, listIDs(got))
}

func Test_findings_counts_follow_the_type_filter_not_the_status_filter(t *testing.T) {
	req := report.FindingsRequest{Status: finding.StatusIgnored, Type: finding.Duplicate, Ignore: []string{ignoredDuplicate}}

	got := findingsWith(t, req, mixedFindings()...)

	assert.Equal(t, finding.Counts{Open: 1, Ignored: 1, Fixed: 1}, got.Counts)
}

func Test_findings_counts_every_type_when_none_is_requested(t *testing.T) {
	req := report.FindingsRequest{Status: finding.StatusIgnored, Ignore: []string{ignoredDuplicate}}

	got := findingsWith(t, req, mixedFindings()...)

	assert.Equal(t, finding.Counts{Open: 2, Ignored: 1, Fixed: 2}, got.Counts)
}

func Test_findings_reports_unmatched_ignore_ids_even_when_type_filters_their_finding_out(t *testing.T) {
	req := report.FindingsRequest{Type: finding.Duplicate, Ignore: []string{openPayee, "duplicate:txn-99+txn-100"}}

	got := findingsWith(t, req, mixedFindings()...)

	assert.Equal(t, []string{"duplicate:txn-99+txn-100"}, got.Unmatched)
}

func Test_findings_status_ignored_omits_an_ignored_finding_that_is_also_fixed(t *testing.T) {
	req := report.FindingsRequest{Status: finding.StatusIgnored, Ignore: []string{ignoredDuplicate, fixedDuplicate}}

	got := findingsWith(t, req, mixedFindings()...)

	assert.Equal(t, map[finding.Type][]string{finding.Duplicate: {ignoredDuplicate}}, listIDs(got))
}

func Test_findings_status_fixed_lists_an_ignored_finding_that_is_also_fixed(t *testing.T) {
	req := report.FindingsRequest{Status: finding.StatusFixed, Ignore: []string{fixedDuplicate}}

	got := findingsWith(t, req, gone(fixedDuplicate, finding.Duplicate, fixedAt))

	assert.Equal(t, map[finding.Type][]string{finding.Duplicate: {fixedDuplicate}}, listIDs(got))
}

func Test_findings_sorts_fixed_findings_by_fixed_at_descending_then_id(t *testing.T) {
	earlier, later := fixedAt.Add(-time.Hour), fixedAt.Add(time.Hour)
	req := report.FindingsRequest{Status: finding.StatusFixed}

	got := findingsWith(t, req,
		gone("duplicate:txn-5+txn-6", finding.Duplicate, earlier),
		gone("duplicate:txn-3+txn-4", finding.Duplicate, later),
		gone("duplicate:txn-1+txn-2", finding.Duplicate, earlier))

	assert.Equal(t, []string{"duplicate:txn-3+txn-4", "duplicate:txn-1+txn-2", "duplicate:txn-5+txn-6"}, idsOf(got.Groups[0]))
}

func Test_findings_lists_a_group_open_first_then_ignored_then_fixed_each_in_its_own_order(t *testing.T) {
	req := report.FindingsRequest{Status: report.FindingsAll, Ignore: []string{"duplicate:txn-7+txn-8", "duplicate:txn-9+txn-10"}}

	got := findingsWith(t, req,
		gone(fixedDuplicate, finding.Duplicate, fixedAt),
		dated("duplicate:txn-9+txn-10", finding.Duplicate, march1, march1),
		dated("duplicate:txn-7+txn-8", finding.Duplicate, march3, march3),
		dated("duplicate:txn-11+txn-12", finding.Duplicate, march1, march1),
		dated(openDuplicate, finding.Duplicate, march2, march2))

	assert.Equal(t, []string{
		openDuplicate, "duplicate:txn-11+txn-12", "duplicate:txn-7+txn-8", "duplicate:txn-9+txn-10", fixedDuplicate,
	}, idsOf(got.Groups[0]))
}

func Test_findings_neither_counts_nor_lists_a_finding_of_an_unknown_type_under_a_filter(t *testing.T) {
	req := report.FindingsRequest{Status: report.FindingsAll, Type: finding.Duplicate}

	got := findingsWith(t, req,
		dated(openDuplicate, finding.Duplicate, march1, march1),
		dated("mystery:thing-1", finding.Type("mystery"), march1))

	assert.Equal(t, finding.Counts{Open: 1}, got.Counts)
	assert.Equal(t, map[finding.Type][]string{finding.Duplicate: {openDuplicate}}, listIDs(got))
}

func findingCountsOf(t *testing.T, ignore []string, fs ...store.Finding) finding.Counts {
	t.Helper()
	return report.CountFindings(store.Status{Findings: fs}, ignore, report.Classification{})
}

// unclassifiedCounts counts the findings over accounts under c, with nothing stored and nothing ignored.
func unclassifiedCounts(ignore []string, c report.Classification, accounts ...store.Account) finding.Counts {
	return report.CountFindings(store.Status{Accounts: accounts}, ignore, c)
}

func newFinding(id string, typ finding.Type) store.Finding {
	f := dated(id, typ, march1)
	f.New = true
	return f
}

func newlyFixed(id string, typ finding.Type) store.Finding {
	f := gone(id, typ, fixedAt)
	f.NewlyFixed = true
	return f
}

func Test_findingCounts_tallies_each_status_and_the_new_and_newly_fixed_findings(t *testing.T) {
	got := findingCountsOf(t, []string{ignoredDuplicate},
		dated(openDuplicate, finding.Duplicate, march1),
		newFinding(openPayee, finding.Uncategorized),
		dated(ignoredDuplicate, finding.Duplicate, march3),
		newlyFixed(fixedDuplicate, finding.Duplicate))

	assert.Equal(t, finding.Counts{Open: 2, Ignored: 1, Fixed: 1, New: 1, NewlyFixed: 1}, got)
}

func Test_findingCounts_counts_an_ignored_fixed_finding_as_fixed(t *testing.T) {
	got := findingCountsOf(t, []string{fixedDuplicate}, gone(fixedDuplicate, finding.Duplicate, fixedAt))

	assert.Equal(t, finding.Counts{Fixed: 1}, got)
}

func Test_findingCounts_does_not_count_an_ignored_new_finding_as_new(t *testing.T) {
	got := findingCountsOf(t, []string{openPayee}, newFinding(openPayee, finding.Uncategorized))

	assert.Equal(t, finding.Counts{Ignored: 1}, got)
}

func Test_findingCounts_counts_every_finding_as_open_when_nothing_is_ignored(t *testing.T) {
	got := findingCountsOf(t, nil,
		dated(openDuplicate, finding.Duplicate, march1),
		dated(ignoredDuplicate, finding.Duplicate, march3))

	assert.Equal(t, finding.Counts{Open: 2}, got)
}

func Test_findingCounts_ignores_an_id_that_names_no_finding(t *testing.T) {
	got := findingCountsOf(t, []string{"duplicate:txn-98+txn-99"}, dated(openDuplicate, finding.Duplicate, march1))

	assert.Equal(t, finding.Counts{Open: 1}, got)
}

func Test_findingCounts_skips_a_finding_of_a_type_this_binary_does_not_know(t *testing.T) {
	got := findingCountsOf(t, nil,
		dated(openDuplicate, finding.Duplicate, march1),
		dated("mystery:thing-1", finding.Type("mystery"), march1))

	assert.Equal(t, finding.Counts{Open: 1}, got)
}

func Test_findingCounts_counts_an_unclassified_account_open_and_never_new(t *testing.T) {
	got := unclassifiedCounts(nil, report.Classification{}, brokerage("acct-1", "TFSA"), brokerage("acct-2", "Margin"))

	assert.Equal(t, finding.Counts{Open: 2}, got)
}

func Test_findingCounts_leaves_out_an_account_the_config_or_its_type_classifies(t *testing.T) {
	cases := []struct {
		name    string
		account store.Account
		class   report.Classification
	}{
		{name: "listed registered", account: brokerage("acct-1", "TFSA"), class: report.Classification{Registered: []string{"acct-1"}}},
		{name: "listed non-registered", account: brokerage("acct-1", "Margin"), class: report.Classification{NonRegistered: []string{"acct-1"}}},
		{name: "not an investment account", account: store.Account{ID: "acct-1", Type: "chequing"}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, finding.Counts{}, unclassifiedCounts(nil, c.class, c.account))
		})
	}
}

func Test_findingCounts_counts_an_ignored_unclassified_account_as_ignored(t *testing.T) {
	got := unclassifiedCounts([]string{unclassifiedOne}, report.Classification{}, brokerage("acct-1", "TFSA"), brokerage("acct-2", "Margin"))

	assert.Equal(t, finding.Counts{Open: 1, Ignored: 1}, got)
}

func Test_findingCounts_adds_an_unclassified_account_to_the_stored_findings(t *testing.T) {
	st := store.Status{
		Findings: []store.Finding{dated(openDuplicate, finding.Duplicate, march1)},
		Accounts: []store.Account{brokerage("acct-1", "TFSA")},
	}

	got := report.CountFindings(st, nil, report.Classification{})

	assert.Equal(t, finding.Counts{Open: 2}, got)
}
