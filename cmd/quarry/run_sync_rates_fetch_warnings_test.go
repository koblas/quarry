// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fetchSyncDoc is sync's --json document, its warnings and rates decoded.
type fetchSyncDoc struct {
	Warnings []string `json:"warnings"`
	Store    struct {
		Rates json.RawMessage `json:"rates"`
	} `json:"store"`
}

// decodeFetchSyncDoc decodes stdout as sync's --json document.
func decodeFetchSyncDoc(t *testing.T, stdout string) fetchSyncDoc {
	t.Helper()
	var doc fetchSyncDoc
	require.NoError(t, json.Unmarshal([]byte(stdout), &doc))
	return doc
}

// unreachableBank is a bank whose host cannot be dialled.
func unreachableBank() http.RoundTripper {
	return failing(&net.OpError{Op: "dial", Net: "tcp", Err: errConnectionRefused})
}

// warningPrefix is how a warning starts on stderr.
const warningPrefix = "quarry: warning: "

func Test_run_sync_json_carries_the_fetch_reason_in_warnings_and_rates(t *testing.T) {
	for _, c := range fetchFailureCases() {
		t.Run(c.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			bundle := writeChequingBundle(t, filepath.Join(home, "Documents"), c.days...)
			for _, earlier := range c.earlier {
				syncThrough(t, earlier, "--quicken", bundle.Dir)
			}

			exitCode, stdout, stderr := syncCapturing(t, c.bank, "--quicken", bundle.Dir, "--json")

			require.Equal(t, 0, exitCode, stderr)
			doc := decodeFetchSyncDoc(t, stdout)
			assert.Equal(t, []string{c.warning}, doc.Warnings)
			assert.JSONEq(t, c.ratesJSON, string(doc.Store.Rates))
			assert.Equal(t, warningPrefix+c.warning+"\n", stderr)
		})
	}
}

// fetchAfterUnreadableRates syncs once, damages the stored fx_rates with ddl, then syncs again through a bank
// that cannot be reached, returning that second sync's exit code, stdout and stderr.
func fetchAfterUnreadableRates(t *testing.T, ddl string, args ...string) (int, string, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	bundle := writeChequingBundle(t, filepath.Join(home, "Documents"), januaryDay(3))
	syncThrough(t, januaryBank(), "--quicken", bundle.Dir)
	editStore(t, home, ddl)
	return syncCapturing(t, unreachableBank(), append([]string{"--quicken", bundle.Dir}, args...)...)
}

func Test_run_sync_prints_the_carry_warning_then_the_fetch_warning(t *testing.T) {
	exitCode, stdout, stderr := fetchAfterUnreadableRates(t, repeatedRatesTable)

	require.Equal(t, 0, exitCode, stderr)
	assert.Equal(t, warningPrefix+ratesRepeatWarning+"\n"+warningPrefix+noRatesPrefix+cannotReach+nothingStoredTail+"\n", stderr)
	assert.Contains(t, strings.Split(stdout, "\n"), notFetchedLine)
}

func Test_run_sync_json_lists_the_carry_warning_then_the_fetch_warning(t *testing.T) {
	exitCode, stdout, stderr := fetchAfterUnreadableRates(t, repeatedRatesTable, "--json")

	require.Equal(t, 0, exitCode, stderr)
	doc := decodeFetchSyncDoc(t, stdout)
	assert.Equal(t, []string{ratesRepeatWarning, noRatesPrefix + cannotReach + nothingStoredTail}, doc.Warnings)
	assert.JSONEq(t, ratesJSON("", "", 0, cannotReach), string(doc.Store.Rates))
}

func Test_run_sync_prints_only_the_fetch_warning_when_a_v4_store_is_synced_while_the_bank_is_unreachable(t *testing.T) {
	exitCode, stdout, stderr := fetchAfterUnreadableRates(t, noRatesTable)

	require.Equal(t, 0, exitCode, stderr)
	assert.Equal(t, warningPrefix+noRatesPrefix+cannotReach+nothingStoredTail+"\n", stderr)
	assert.Contains(t, strings.Split(stdout, "\n"), notFetchedLine)
}

func Test_run_sync_json_lists_only_the_fetch_warning_when_a_v4_store_is_synced_while_the_bank_is_unreachable(t *testing.T) {
	exitCode, stdout, stderr := fetchAfterUnreadableRates(t, noRatesTable, "--json")

	require.Equal(t, 0, exitCode, stderr)
	doc := decodeFetchSyncDoc(t, stdout)
	assert.Equal(t, []string{noRatesPrefix + cannotReach + nothingStoredTail}, doc.Warnings)
	assert.JSONEq(t, ratesJSON("", "", 0, cannotReach), string(doc.Store.Rates))
}

// fetchAfterUnreadableStore syncs once, replaces the store with a file that is not a database,
// then syncs again through a bank that cannot be reached.
func fetchAfterUnreadableStore(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	bundle := writeChequingBundle(t, filepath.Join(home, "Documents"), januaryDay(3))
	syncThrough(t, januaryBank(), "--quicken", bundle.Dir)
	require.NoError(t, os.WriteFile(storePathUnder(home), []byte("this is not a database"), 0o600))
	return syncCapturing(t, unreachableBank(), append([]string{"--quicken", bundle.Dir}, args...)...)
}

func Test_run_sync_prints_the_combined_carry_line_then_the_fetch_warning(t *testing.T) {
	exitCode, stdout, stderr := fetchAfterUnreadableStore(t)

	require.Equal(t, 0, exitCode, stderr)
	assert.Equal(t, warningPrefix+combinedCarryWarning+"\n"+warningPrefix+noRatesPrefix+cannotReach+nothingStoredTail+"\n", stderr)
	assert.Contains(t, strings.Split(stdout, "\n"), notFetchedLine)
}

func Test_run_sync_json_lists_the_combined_carry_line_then_the_fetch_warning(t *testing.T) {
	exitCode, stdout, stderr := fetchAfterUnreadableStore(t, "--json")

	require.Equal(t, 0, exitCode, stderr)
	assert.Equal(t, []string{combinedCarryWarning, noRatesPrefix + cannotReach + nothingStoredTail}, decodeFetchSyncDoc(t, stdout).Warnings)
}

// fetchWithRefusedPrune syncs a bundle through an unreachable bank while the oldest of a full snapshots folder
// cannot be deleted, returning the refused snapshot's ID, then exit code, stdout and stderr.
func fetchWithRefusedPrune(t *testing.T, args ...string) (string, int, string, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	bundle := writeChequingBundle(t, filepath.Join(home, "Documents"), januaryDay(3))
	fixtures := oldSnapshots(keptSnapshots)
	writeSnapshots(t, home, fixtures...)

	exitCode, stdout, stderr := syncCapturingRemoving(t, unreachableBank(), refusingRemove(fixtures[0].id+".sqlite"),
		append([]string{"--quicken", bundle.Dir}, args...)...)

	return fixtures[0].id, exitCode, stdout, stderr
}

func Test_run_sync_prints_the_fetch_warning_before_the_prune_warning(t *testing.T) {
	id, exitCode, _, stderr := fetchWithRefusedPrune(t)

	require.Equal(t, 0, exitCode, stderr)
	assert.Equal(t, warningPrefix+noRatesPrefix+cannotReach+nothingStoredTail+"\n"+
		warningPrefix+"cannot delete snapshot "+id+": permission denied; run quarry snapshots prune to try again\n", stderr)
}

func Test_run_sync_json_lists_the_fetch_warning_before_the_prune_warning(t *testing.T) {
	id, exitCode, stdout, stderr := fetchWithRefusedPrune(t, "--json")

	require.Equal(t, 0, exitCode, stderr)
	assert.Equal(t, []string{
		noRatesPrefix + cannotReach + nothingStoredTail,
		"cannot delete snapshot " + id + ": permission denied; run quarry snapshots prune to try again",
	}, decodeFetchSyncDoc(t, stdout).Warnings)
}

// firstAnswers503ThenNotAList answers its first request 503 and every later one with text that is not a list.
func firstAnswers503ThenNotAList() http.RoundTripper {
	var calls atomic.Int32
	return roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if calls.Add(1) == 1 {
			return valetResponse(req, http.StatusServiceUnavailable, ""), nil
		}
		return valetResponse(req, http.StatusOK, "<html>maintenance</html>"), nil
	})
}

// syncWhereTwoSpansFail syncs once so rates are stored, then syncs a bundle whose earlier
// transaction asks for a span before them and a span after them, both failing differently.
func syncWhereTwoSpansFail(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	first := writeChequingBundle(t, filepath.Join(home, "Documents"), januaryDay(3))
	syncThrough(t, januaryBank(), "--quicken", first.Dir)
	second := writeChequingBundle(t, filepath.Join(home, "Earlier"), januaryDay(3).AddDate(0, 0, -4), januaryDay(3))
	return syncCapturing(t, firstAnswers503ThenNotAList(), append([]string{"--quicken", second.Dir}, args...)...)
}

func Test_run_sync_prints_one_fetch_warning_with_the_first_reason_when_two_spans_fail_differently(t *testing.T) {
	exitCode, stdout, stderr := syncWhereTwoSpansFail(t)

	require.Equal(t, 0, exitCode, stderr)
	assert.Equal(t, warningPrefix+noRatesPrefix+answered503+nothingNewTail+"\n", stderr)
	assert.Contains(t, strings.Split(stdout, "\n"), notRefreshedLine)
}

func Test_run_sync_json_lists_one_fetch_warning_with_the_first_reason_when_two_spans_fail_differently(t *testing.T) {
	exitCode, stdout, stderr := syncWhereTwoSpansFail(t, "--json")

	require.Equal(t, 0, exitCode, stderr)
	doc := decodeFetchSyncDoc(t, stdout)
	assert.Equal(t, []string{noRatesPrefix + answered503 + nothingNewTail}, doc.Warnings)
	assert.JSONEq(t, ratesJSON("2017-01-03", "2017-01-04", 0, answered503), string(doc.Store.Rates))
}

// syncFromAfterFailedFetch syncs a bundle, then rebuilds the store from its snapshot through a bank that times out.
func syncFromAfterFailedFetch(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	bundle := writeChequingBundle(t, filepath.Join(home, "Documents"), januaryDay(3))
	syncThrough(t, januaryBank(), "--quicken", bundle.Dir)
	id := snapshotID(onlyFileWithSuffix(t, snapshotsDirUnder(home), ".sqlite"))
	return syncCapturing(t, failing(context.DeadlineExceeded), append([]string{"--from", id}, args...)...)
}

func Test_run_sync_from_warns_when_the_rate_fetch_fails(t *testing.T) {
	exitCode, stdout, stderr := syncFromAfterFailedFetch(t)

	require.Equal(t, 0, exitCode, stderr)
	assert.Equal(t, warningPrefix+noRatesPrefix+noAnswerInTime+nothingNewTail+"\n", stderr)
	assert.Contains(t, strings.Split(stdout, "\n"), notRefreshedLine)
	assert.Equal(t, "rates_fetch_error\n"+noAnswerInTime+"\n", lastFetchError(t))
}

func Test_run_sync_from_json_lists_the_fetch_warning_and_reason(t *testing.T) {
	exitCode, stdout, stderr := syncFromAfterFailedFetch(t, "--json")

	require.Equal(t, 0, exitCode, stderr)
	doc := decodeFetchSyncDoc(t, stdout)
	assert.Equal(t, []string{noRatesPrefix + noAnswerInTime + nothingNewTail}, doc.Warnings)
	assert.JSONEq(t, ratesJSON("2017-01-03", "2017-01-04", 0, noAnswerInTime), string(doc.Store.Rates))
}

// syncTwiceAroundALegacyOutage syncs a bundle dated before the current series while the legacy series is down,
// then syncs it again with the bank whole, returning the second sync's stdout.
func syncTwiceAroundALegacyOutage(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	bundle := writeChequingBundle(t, filepath.Join(home, "Documents"), time.Date(2016, time.December, 30, 0, 0, 0, 0, time.UTC))
	current := januaryBank()
	syncThrough(t, legacyDown(current), "--quicken", bundle.Dir)
	whole := fakeValet{"FXUSDCAD": current["FXUSDCAD"], "IEXE0101": {"2016-12-30": "1.3400", "2017-01-02": "1.3410"}}

	return syncThrough(t, whole, "--quicken", bundle.Dir)
}

func Test_run_sync_again_fetches_the_rates_a_legacy_outage_left_out(t *testing.T) {
	stdout := syncTwiceAroundALegacyOutage(t)

	assert.Contains(t, strings.Split(stdout, "\n"), "Rates     USD/CAD 2016-12-30 to 2017-01-04 (2 new)")
	assert.Equal(t, "date\n2016-12-30\n2017-01-02\n2017-01-03\n2017-01-04\n", storedRateDates(t))
}
