// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	findingsRepeatWarning = "cannot carry findings forward from the previous store " +
		"(its findings table repeats an id); findings history starts again with this sync"
	importRunsTooLargeWarning = "cannot carry import history forward from the previous store " +
		"(its import_runs table has an id too large to follow); import_runs starts again with this sync"
	importRunsIDTooLarge = "UPDATE import_runs SET id = 9223372036854775807"
)

// findingsTableRepeating replaces the findings table with one that has no
// PRIMARY KEY and holds id twice.
func findingsTableRepeating(id string) string {
	return fmt.Sprintf(`DROP TABLE findings;
CREATE TABLE findings (id VARCHAR, type VARCHAR NOT NULL, first_found_at TIMESTAMP NOT NULL, fixed_at TIMESTAMP);
INSERT INTO findings VALUES
	('%[1]s', 'uncategorized', TIMESTAMP '2026-03-01 00:00:00', NULL),
	('%[1]s', 'uncategorized', TIMESTAMP '2026-03-01 00:00:00', NULL)`, id)
}

// syncNewBundle writes b under home/dir and runs sync --quicken on it with extra args.
func syncNewBundle(t *testing.T, home, dir string, b *v9fixture.Builder, extra ...string) (int, string, string) {
	t.Helper()
	bundle := b.WriteBundle(t, filepath.Join(home, dir))
	exitCode, stdout, stderr := runCapture(context.Background(), append([]string{"sync", "--quicken", bundle.Dir}, extra...))
	return exitCode, stdout.String(), stderr.String()
}

// syncWithFaultedFindings syncs an open uncategorized finding for one payee, damages the store with
// stmt built from that finding's id, then syncs a bundle whose only open finding is the other payee's.
func syncWithFaultedFindings(t *testing.T, home string, stmt func(findingID string) string, extra ...string) (int, string, string) {
	t.Helper()
	first, xPK := twoPayeeBundle(false, true)
	second, _ := twoPayeeBundle(true, false)
	syncFindingsBundleIn(t, home, "DocumentsA", first)
	editStore(t, home, stmt(fmt.Sprintf("uncategorized:payee-%d", xPK)))
	return syncNewBundle(t, home, "DocumentsB", second, extra...)
}

func Test_run_sync_from_warns_and_restarts_findings_history_when_the_previous_findings_table_repeats_an_id(t *testing.T) {
	home := newHome(t)
	id, _ := syncThenWrite(t, home)
	editStore(t, home, findingsTableRepeating("uncategorized:payee-1"))

	exitCode, _, stderr := runSyncFrom(t, id)

	require.Equal(t, 0, exitCode, stderr)
	assert.Equal(t, "quarry: warning: "+findingsRepeatWarning+"\n", stderr)
}

func Test_run_sync_after_a_findings_fault_prints_the_findings_line_without_new_or_fixed_clauses(t *testing.T) {
	home := newHome(t)

	exitCode, stdout, stderr := syncWithFaultedFindings(t, home, findingsTableRepeating)

	require.Equal(t, 0, exitCode, stderr)
	assert.Equal(t, "Findings  1 open; run quarry findings to list them", findingsLine(t, stdout))
}

func Test_run_sync_json_after_a_findings_fault_lists_the_findings_warning_and_counts_every_open_finding_as_new(t *testing.T) {
	home := newHome(t)

	exitCode, stdout, stderr := syncWithFaultedFindings(t, home, findingsTableRepeating, "--json")

	require.Equal(t, 0, exitCode, stderr)
	var parsed struct {
		Store struct {
			Findings json.RawMessage `json:"findings"`
		} `json:"store"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &parsed))
	assert.Equal(t, []string{findingsRepeatWarning}, decodeSyncDoc(t, stdout).Warnings)
	assert.JSONEq(t, `{"open":1,"ignored":0,"fixed":0,"new":1,"newly_fixed":0}`, string(parsed.Store.Findings))
}

func Test_run_sync_prints_the_import_history_line_then_the_findings_line_when_both_tables_are_faulty(t *testing.T) {
	home := newHome(t)

	exitCode, _, stderr := syncWithFaultedFindings(t, home, func(id string) string {
		return findingsTableRepeating(id) + ";" + importRunsIDTooLarge
	})

	require.Equal(t, 0, exitCode, stderr)
	assert.Equal(t, "quarry: warning: "+importRunsTooLargeWarning+"\nquarry: warning: "+findingsRepeatWarning+"\n", stderr)
}

func Test_run_sync_json_lists_the_import_history_warning_then_the_findings_warning_when_both_tables_are_faulty(t *testing.T) {
	home := newHome(t)

	exitCode, stdout, stderr := syncWithFaultedFindings(t, home, func(id string) string {
		return findingsTableRepeating(id) + ";" + importRunsIDTooLarge
	}, "--json")

	require.Equal(t, 0, exitCode, stderr)
	assert.Equal(t, []string{importRunsTooLargeWarning, findingsRepeatWarning}, decodeSyncDoc(t, stdout).Warnings)
}
