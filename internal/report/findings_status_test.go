package report_test

import (
	"testing"
	"time"

	"github.com/koblas/quarry/internal/finding"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
