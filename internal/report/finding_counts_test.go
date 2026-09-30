package report_test

import (
	"context"
	"testing"

	"github.com/koblas/quarry/internal/finding"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func findingCountsOf(t *testing.T, ignore []string, fs ...store.Finding) finding.Counts {
	t.Helper()
	srv := report.NewServer(report.WithStore(fakeStore{findings: store.FindingList{Findings: fs}}))
	got, err := srv.FindingCounts(t.Context(), ignore)
	require.NoError(t, err)
	return got
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

func Test_findingCounts_returns_the_store_fault(t *testing.T) {
	srv := report.NewServer(report.WithStore(fakeStore{err: errDiskRead}))

	_, err := srv.FindingCounts(t.Context(), nil)

	require.ErrorIs(t, err, errDiskRead)
}

func Test_findingCounts_refuses_a_missing_store_naming_status(t *testing.T) {
	openErr := &store.OpenError{Fault: store.OpenFaultMissing, Path: storePath}
	srv := report.NewServer(report.WithStore(fakeStore{err: openErr}), report.WithHome(refusalHome))

	_, err := srv.FindingCounts(t.Context(), nil)

	assert.EqualError(t, err, "no store at ~/Library/Application Support/quarry/quarry.duckdb yet; run quarry sync to build it")
}

func Test_findingCounts_is_interrupted_when_its_context_is_done(t *testing.T) {
	srv := report.NewServer(report.WithStore(fakeStore{err: errDiskRead}))
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := srv.FindingCounts(ctx, nil)

	assert.EqualError(t, err, "status interrupted")
}
