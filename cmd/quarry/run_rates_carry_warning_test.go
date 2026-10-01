// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	ratesRepeatWarning = "cannot carry exchange rates forward from the previous store " +
		"(its fx_rates table repeats a date); fetching them all again"

	// repeatedRatesTable replaces fx_rates with a table that has no PRIMARY KEY and holds one date twice.
	repeatedRatesTable = `DROP TABLE fx_rates CASCADE;
CREATE TABLE fx_rates (date DATE, usd_cad DECIMAL(10, 6), series VARCHAR);
INSERT INTO fx_rates VALUES ('2026-01-02', 1.25, 'FXUSDCAD'), ('2026-01-02', 1.26, 'FXUSDCAD')`

	// noRatesTable is a v4 store's shape: no fx_rates table at all.
	noRatesTable = "DROP TABLE fx_rates CASCADE"
)

func Test_run_sync_from_warns_and_fetches_every_rate_again_when_the_previous_fx_rates_table_repeats_a_date(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	id, _ := syncThenWrite(t, home)
	editStore(t, home, repeatedRatesTable)

	exitCode, stdout, stderr := runSyncFrom(t, id)

	require.Equal(t, 0, exitCode, stderr)
	assert.Equal(t, "quarry: warning: "+ratesRepeatWarning+"\n", stderr)
	assert.Contains(t, stdout, fakeRatesText)
}

func Test_run_sync_from_json_lists_the_rates_warning_and_counts_every_rate_as_added_when_the_previous_fx_rates_table_repeats_a_date(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	id, _ := syncThenWrite(t, home)
	editStore(t, home, repeatedRatesTable)

	exitCode, stdout, stderr := runSyncFrom(t, id, "--json")

	require.Equal(t, 0, exitCode, stderr)
	var parsed struct {
		Store struct {
			Rates struct {
				Added int `json:"added"`
			} `json:"rates"`
		} `json:"store"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &parsed))
	assert.Equal(t, []string{ratesRepeatWarning}, decodeSyncDoc(t, stdout).Warnings)
	assert.Equal(t, 1, parsed.Store.Rates.Added)
}

func Test_run_sync_from_is_silent_when_the_previous_store_has_no_fx_rates_table(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	id, _ := syncThenWrite(t, home)
	editStore(t, home, noRatesTable)

	exitCode, stdout, stderr := runSyncFrom(t, id)

	require.Equal(t, 0, exitCode, stderr)
	assert.Empty(t, stderr)
	assert.Contains(t, stdout, fakeRatesText)
}

func Test_run_sync_from_json_lists_no_warning_when_the_previous_store_has_no_fx_rates_table(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	id, _ := syncThenWrite(t, home)
	editStore(t, home, noRatesTable)

	exitCode, stdout, stderr := runSyncFrom(t, id, "--json")

	require.Equal(t, 0, exitCode, stderr)
	assert.Empty(t, stderr)
	assert.Empty(t, decodeSyncDoc(t, stdout).Warnings)
}

func Test_run_sync_prints_the_history_findings_and_rates_warnings_in_that_order(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	exitCode, _, stderr := syncWithFaultedFindings(t, home, func(id string) string {
		return findingsTableRepeating(id) + ";" + importRunsIDTooLarge + ";" + repeatedRatesTable
	})

	require.Equal(t, 0, exitCode, stderr)
	assert.Equal(t, "quarry: warning: "+importRunsTooLargeWarning+"\nquarry: warning: "+findingsRepeatWarning+"\nquarry: warning: "+ratesRepeatWarning+"\n", stderr)
}

func Test_run_sync_json_lists_the_history_findings_and_rates_warnings_in_that_order(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	exitCode, stdout, stderr := syncWithFaultedFindings(t, home, func(id string) string {
		return findingsTableRepeating(id) + ";" + importRunsIDTooLarge + ";" + repeatedRatesTable
	}, "--json")

	require.Equal(t, 0, exitCode, stderr)
	assert.Equal(t, []string{importRunsTooLargeWarning, findingsRepeatWarning, ratesRepeatWarning}, decodeSyncDoc(t, stdout).Warnings)
}
