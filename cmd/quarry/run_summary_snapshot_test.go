package main

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// summaryClockEDT is October 6 noon in the pinned local zone: summaryClock is UTC, so it would print the
// warning's time in UTC while the Snapshot row prints EDT.
func summaryClockEDT() time.Time {
	return time.Date(2026, time.October, 6, 12, 0, 0, 0, time.FixedZone("EDT", -4*60*60))
}

// septemberTimeUnknownWarning is what a September summary says about a snapshot whose manifest records no time.
const septemberTimeUnknownWarning = "quarry: warning: cannot tell whether the store holds all of September 2026: " +
	"its snapshot's manifest does not record when it was taken; " +
	"open your Quicken file and run quarry sync to take a new snapshot\n"

func Test_run_summary_warns_when_the_snapshot_was_taken_before_the_month_ended(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	pinLocalZone(t)
	rows := summaryRows(true)
	rows.ImportRuns[0].Snapshot.TakenAt = time.Date(2026, time.September, 28, 14, 2, 0, 0, time.FixedZone("EDT", -4*60*60))
	replaceStore(t, home, rows)
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"summary"}, spendEnvAt(&stdout, &stderr, summaryClockEDT()))

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Contains(t, stdout.String(), "Summary of September 2026 (2026-09-01 to 2026-09-30), amounts in CAD\n\n"+
		"Snapshot  20261001T130512Z, taken 2026-09-28 14:02 EDT (7 days ago)\n")
	assert.Equal(t, "quarry: warning: the store was built from a snapshot taken 2026-09-28 14:02 EDT, before September 2026 ended, "+
		"so transactions from the rest of the month are missing; open your Quicken file, run quarry sync, "+
		"then run quarry summary again\n", stderr.String())
}

func Test_run_summary_warns_when_the_snapshot_records_no_time(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	pinLocalZone(t)
	rows := summaryRows(true)
	rows.ImportRuns[0].Snapshot.TakenAt = time.Time{}
	replaceStore(t, home, rows)
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"summary"}, spendEnvAt(&stdout, &stderr, summaryClockEDT()))

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Contains(t, stdout.String(), "Snapshot  20261001T130512Z, time taken not recorded in its manifest\n")
	assert.Equal(t, septemberTimeUnknownWarning, stderr.String())
}
