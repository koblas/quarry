package document_test

import (
	"testing"
	"time"

	"github.com/koblas/quarry/internal/finding"
	"github.com/koblas/quarry/internal/report/document"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
)

func Test_CannotTellChoices_names_the_problem_and_says_what_status_counted_without_the_config(t *testing.T) {
	got := document.CannotTellChoices("x")

	assert.Equal(t, "cannot tell which findings you ignored or how you classified your accounts: x; "+
		"findings you ignored are counted as open, and every investment account is counted as unclassified", got)
}

func Test_NewStatus_reports_ignored_only_when_the_ignore_list_was_read(t *testing.T) {
	counts := finding.Counts{Open: 12, Ignored: 4, Fixed: 7, New: 2, NewlyFixed: 1}
	read := document.NewStatus(store.Status{}, document.FindingsTally{Counts: counts, IgnoreKnown: true}, nil)
	unread := document.NewStatus(store.Status{}, document.FindingsTally{Counts: counts}, nil)

	assert.Equal(t, 4, *read.Findings.Ignored)
	assert.Nil(t, unread.Findings.Ignored)
	assert.Equal(t, 12, unread.Findings.Open)
}

func Test_NewStatus_writes_times_in_UTC_and_nulls_absent_ones(t *testing.T) {
	zone := time.FixedZone("EDT", -4*60*60)
	st := store.Status{BuiltAt: time.Date(2026, 9, 27, 10, 31, 2, 0, zone)}

	got := document.NewStatus(st, document.FindingsTally{}, nil)

	assert.Equal(t, "2026-09-27T14:31:02Z", got.Store.BuiltAt)
	assert.Nil(t, got.Snapshot.TakenAt)
	assert.Nil(t, got.Dates.First)
	assert.Nil(t, got.Snapshot.Source)
}

func Test_NewStatus_lists_warnings_as_an_empty_array_when_there_are_none(t *testing.T) {
	got := document.NewStatus(store.Status{}, document.FindingsTally{}, nil)

	assert.Equal(t, []string{}, got.Warnings)
}
