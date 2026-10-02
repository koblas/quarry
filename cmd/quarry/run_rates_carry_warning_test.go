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

	// looseRatesTable replaces fx_rates with a table that has no PRIMARY KEY or CHECK, so a corrupt row can be stored.
	looseRatesTable = `DROP TABLE fx_rates CASCADE;
CREATE TABLE fx_rates (date DATE, usd_cad DECIMAL(10, 6), series VARCHAR);
`

	// repeatedRatesTable holds one date twice.
	repeatedRatesTable = looseRatesTable + `INSERT INTO fx_rates VALUES ('2026-01-02', 1.25, 'FXUSDCAD'), ('2026-01-02', 1.26, 'FXUSDCAD')`

	// noRatesTable is a v4 store's shape: no fx_rates table at all.
	noRatesTable = "DROP TABLE fx_rates CASCADE"
)

// unreadableRatesTables is, per ruled reason, a fx_rates table whose single row quarry cannot carry.
var unreadableRatesTables = []struct{ name, reason, ddl string }{
	{"a repeated date", "its fx_rates table repeats a date", repeatedRatesTable},
	{"an impossible rate", "its fx_rates table holds an impossible rate", looseRatesTable + "INSERT INTO fx_rates VALUES ('2026-01-02', 0, 'FXUSDCAD')"},
	{"an unknown series", "its fx_rates table names an unknown series", looseRatesTable + "INSERT INTO fx_rates VALUES ('2026-01-02', 1.25, 'ECB')"},
	{"a NULL cell", "its fx_rates table is incomplete", looseRatesTable + "INSERT INTO fx_rates VALUES ('2026-01-02', NULL, 'FXUSDCAD')"},
}

// ratesWarningFor is the warning line for reason, as it reads in stderr after its prefix.
func ratesWarningFor(reason string) string {
	return "cannot carry exchange rates forward from the previous store (" + reason + "); fetching them all again"
}

func Test_run_sync_from_warns_and_fetches_every_rate_again_when_the_previous_fx_rates_table_is_unreadable(t *testing.T) {
	for _, c := range unreadableRatesTables {
		t.Run(c.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			id, _ := syncThenWrite(t, home)
			editStore(t, home, c.ddl)

			exitCode, stdout, stderr := runSyncFrom(t, id)

			require.Equal(t, 0, exitCode, stderr)
			assert.Equal(t, "quarry: warning: "+ratesWarningFor(c.reason)+"\n", stderr)
			assert.Contains(t, stdout, fakeRatesText)
		})
	}
}

func Test_run_sync_from_json_lists_the_rates_warning_and_counts_every_rate_as_added_when_the_previous_fx_rates_table_is_unreadable(t *testing.T) {
	for _, c := range unreadableRatesTables {
		t.Run(c.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			id, _ := syncThenWrite(t, home)
			editStore(t, home, c.ddl)

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
			assert.Equal(t, []string{ratesWarningFor(c.reason)}, decodeSyncDoc(t, stdout).Warnings)
			assert.Equal(t, 1, parsed.Store.Rates.Added)
		})
	}
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
