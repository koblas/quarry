package document_test

import (
	"testing"
	"time"

	"github.com/koblas/quarry/internal/finding"
	"github.com/koblas/quarry/internal/report/document"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
)

const unreadableConfig = "/home/dave/config.toml: line 3: expected a value"

func Test_StatusIgnore_keeps_the_list_and_warns_nothing_when_the_config_was_read(t *testing.T) {
	ignore := []string{"duplicate:txn-1+txn-2"}

	list, warnings := document.StatusIgnore(ignore, "")

	assert.Equal(t, ignore, list)
	assert.Empty(t, warnings)
}

func Test_StatusIgnore_drops_the_list_and_warns_once_when_the_config_is_unreadable(t *testing.T) {
	ignore := []string{"duplicate:txn-1+txn-2"}

	list, warnings := document.StatusIgnore(ignore, unreadableConfig)

	assert.Nil(t, list)
	assert.Equal(t, []string{document.CannotTellIgnored(unreadableConfig)}, warnings)
}

func Test_CannotTellIgnored_names_the_problem_and_says_ignored_findings_count_as_open(t *testing.T) {
	got := document.CannotTellIgnored("x")

	assert.Equal(t, "cannot tell which findings you ignored: x; findings you ignored are counted as open", got)
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
