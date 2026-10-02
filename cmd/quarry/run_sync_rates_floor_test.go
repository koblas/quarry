// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ratesDoc is the rates member of sync's --json store object.
type ratesDoc struct {
	First      *string `json:"first"`
	Last       *string `json:"last"`
	Added      int     `json:"added"`
	FetchError *string `json:"fetch_error"`
}

// decodeSyncRates decodes stdout as sync's --json document and returns its store.rates object.
func decodeSyncRates(t *testing.T, stdout string) ratesDoc {
	t.Helper()
	var doc struct {
		Store struct {
			Rates ratesDoc `json:"rates"`
		} `json:"store"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &doc))
	return doc.Store.Rates
}

// emptyValet is a Bank of Canada that publishes nothing.
func emptyValet() fakeValet { return fakeValet{"FXUSDCAD": {}, "IEXE0101": {}} }

// dayOf is the civil day y-m-d at UTC midnight.
func dayOf(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

// syncTailAnsweredEmpty syncs a 2017-01-03 transaction against a bank that publishes 01-03 and 01-04, then syncs
// again against a bank that publishes nothing more, with extraArgs; it returns the second sync's stdout and requests.
func syncTailAnsweredEmpty(t *testing.T, extraArgs ...string) (string, []string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	bundle := writeChequingBundle(t, filepath.Join(home, "Documents"), januaryDay(3))
	syncThrough(t, fakeValet{"FXUSDCAD": {"2017-01-03": "1.3435", "2017-01-04": "1.3315"}}, "--quicken", bundle.Dir)
	source := &recordingValet{next: emptyValet()}

	stdout := syncThrough(t, source, append([]string{"--quicken", bundle.Dir}, extraArgs...)...)

	return stdout, source.requested()
}

// syncWithCoveringRate syncs a 2017-01-03 transaction over a store holding a rate on every day from 2017-01-02 to
// 2099-12-31, with extraArgs; it returns stdout and the requests the bank saw.
func syncWithCoveringRate(t *testing.T, extraArgs ...string) (string, []string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	rates := []store.Rate{{Date: dayOf(2017, time.January, 2), USDCAD: money.Rate(1_340_000), Series: "IEXE0101"}}
	for d := dayOf(2017, time.January, 3); !d.After(dayOf(2099, time.December, 31)); d = d.AddDate(0, 0, 1) {
		rates = append(rates, store.Rate{Date: d, USDCAD: money.Rate(1_250_000), Series: "FXUSDCAD"})
	}
	replaceStoreWithRates(t, home, spendRows([]store.Account{chequingAccount("acct-cad", 1)}), rates...)
	bundle := writeChequingBundle(t, filepath.Join(home, "Documents"), januaryDay(3))
	source := &recordingValet{next: emptyValet()}

	stdout := syncThrough(t, source, append([]string{"--quicken", bundle.Dir}, extraArgs...)...)

	return stdout, source.requested()
}

// syncLaterBundleOverEarlierRates syncs a 2017-01-03 bundle against a bank publishing 01-03 and 01-04, then a bundle
// whose earliest transaction is 2017-01-10 against an empty bank, with extraArgs. It returns the second sync's stdout and requests.
func syncLaterBundleOverEarlierRates(t *testing.T, extraArgs ...string) (string, []string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	early := writeChequingBundle(t, filepath.Join(home, "Early"), januaryDay(3))
	later := writeChequingBundle(t, filepath.Join(home, "Later"), januaryDay(10))
	syncThrough(t, fakeValet{"FXUSDCAD": {"2017-01-03": "1.3435", "2017-01-04": "1.3315"}}, "--quicken", early.Dir)
	source := &recordingValet{next: emptyValet()}

	stdout := syncThrough(t, source, append([]string{"--quicken", later.Dir}, extraArgs...)...)

	return stdout, source.requested()
}

// syncFutureDatedBundle syncs a bundle whose only transaction is dated 2099-01-01 over a store holding the 2017-01-03 and 01-04 rates,
// with extraArgs; it returns stdout and the requests the bank saw.
func syncFutureDatedBundle(t *testing.T, extraArgs ...string) (string, []string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	replaceStoreWithRates(t, home, spendRows([]store.Account{chequingAccount("acct-cad", 1)}),
		store.Rate{Date: januaryDay(3), USDCAD: money.Rate(1_343_500), Series: "FXUSDCAD"},
		store.Rate{Date: januaryDay(4), USDCAD: money.Rate(1_331_500), Series: "FXUSDCAD"},
	)
	bundle := writeChequingBundle(t, filepath.Join(home, "Documents"), dayOf(2099, time.January, 1))
	source := &recordingValet{next: emptyValet()}

	stdout := syncThrough(t, source, append([]string{"--quicken", bundle.Dir}, extraArgs...)...)

	return stdout, source.requested()
}

// syncNoTransactionsWithRates syncs a bundle with no transactions over a store holding 2017-01-03 and 01-04 rates,
// with extraArgs; it returns stdout and the requests the bank saw.
func syncNoTransactionsWithRates(t *testing.T, extraArgs ...string) (string, []string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	replaceStoreWithRates(t, home, spendRows([]store.Account{chequingAccount("acct-cad", 1)}),
		store.Rate{Date: januaryDay(3), USDCAD: money.Rate(1_343_500), Series: "FXUSDCAD"},
		store.Rate{Date: januaryDay(4), USDCAD: money.Rate(1_331_500), Series: "FXUSDCAD"},
	)
	bundle := writeChequingBundle(t, filepath.Join(home, "Documents"))
	source := &recordingValet{next: emptyValet()}

	stdout := syncThrough(t, source, append([]string{"--quicken", bundle.Dir}, extraArgs...)...)

	return stdout, source.requested()
}

// syncAfterAnOlderDateWasAnsweredEmpty syncs a 2017-01-10 bundle, then one reaching back to 01-03 against an empty bank,
// then that bundle again with extraArgs. It returns the middle sync's requests, then the last sync's stdout and requests.
func syncAfterAnOlderDateWasAnsweredEmpty(t *testing.T, extraArgs ...string) ([]string, string, []string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	later := writeChequingBundle(t, filepath.Join(home, "Later"), januaryDay(10))
	earlier := writeChequingBundle(t, filepath.Join(home, "Earlier"), januaryDay(3), januaryDay(10))
	syncThrough(t, fakeValet{"FXUSDCAD": {"2017-01-10": "1.3300", "2017-01-11": "1.3350"}}, "--quicken", later.Dir)
	reachBack := &recordingValet{next: emptyValet()}
	syncThrough(t, reachBack, "--quicken", earlier.Dir)
	again := &recordingValet{next: emptyValet()}

	stdout := syncThrough(t, again, append([]string{"--quicken", earlier.Dir}, extraArgs...)...)

	return reachBack.requested(), stdout, again.requested()
}

func Test_run_sync_prints_up_to_date_when_the_tail_was_asked_and_answered_empty(t *testing.T) {
	stdout, requests := syncTailAnsweredEmpty(t)

	assert.Equal(t, []string{"FXUSDCAD 2017-01-05"}, requests)
	assert.Contains(t, outputLines(stdout), "Rates     USD/CAD 2017-01-03 to 2017-01-04 (up to date)")
}

func Test_run_sync_json_reports_nothing_added_when_the_tail_was_asked_and_answered_empty(t *testing.T) {
	stdout, _ := syncTailAnsweredEmpty(t, "--json")

	assert.Equal(t, ratesDoc{First: new("2017-01-03"), Last: new("2017-01-04")}, decodeSyncRates(t, stdout))
}

func Test_run_sync_asks_for_nothing_and_prints_up_to_date_when_a_carried_rate_covers_the_need(t *testing.T) {
	stdout, requests := syncWithCoveringRate(t)

	assert.Empty(t, requests)
	assert.Contains(t, outputLines(stdout), "Rates     USD/CAD 2017-01-02 to 2099-12-31 (up to date)")
}

func Test_run_sync_json_reports_nothing_added_when_a_carried_rate_covers_the_need(t *testing.T) {
	stdout, _ := syncWithCoveringRate(t, "--json")

	assert.Equal(t, ratesDoc{First: new("2017-01-02"), Last: new("2099-12-31")}, decodeSyncRates(t, stdout))
}

func Test_run_sync_keeps_the_carried_rates_and_prints_up_to_date_when_the_file_has_no_transactions(t *testing.T) {
	stdout, requests := syncNoTransactionsWithRates(t)

	assert.Empty(t, requests)
	assert.Contains(t, outputLines(stdout), "Rates     USD/CAD 2017-01-03 to 2017-01-04 (up to date)")
}

func Test_run_sync_json_keeps_the_carried_rates_when_the_file_has_no_transactions(t *testing.T) {
	stdout, _ := syncNoTransactionsWithRates(t, "--json")

	assert.Equal(t, ratesDoc{First: new("2017-01-03"), Last: new("2017-01-04")}, decodeSyncRates(t, stdout))
}

func Test_run_sync_sends_no_head_request_after_an_older_date_was_answered_empty(t *testing.T) {
	reachedBack, stdout, requests := syncAfterAnOlderDateWasAnsweredEmpty(t)

	require.Contains(t, reachedBack, "FXUSDCAD 2017-01-03")
	assert.Equal(t, []string{"FXUSDCAD 2017-01-12"}, requests)
	assert.Contains(t, outputLines(stdout), "Rates     USD/CAD 2017-01-10 to 2017-01-11 (up to date)")
}

func Test_run_sync_json_reports_nothing_added_after_an_older_date_was_answered_empty(t *testing.T) {
	_, stdout, _ := syncAfterAnOlderDateWasAnsweredEmpty(t, "--json")

	assert.Equal(t, ratesDoc{First: new("2017-01-10"), Last: new("2017-01-11")}, decodeSyncRates(t, stdout))
}

func Test_run_sync_continues_from_the_day_after_the_last_stored_rate_when_the_earliest_transaction_is_later(t *testing.T) {
	stdout, requests := syncLaterBundleOverEarlierRates(t)

	assert.Equal(t, []string{"FXUSDCAD 2017-01-05"}, requests)
	assert.Contains(t, outputLines(stdout), "Rates     USD/CAD 2017-01-03 to 2017-01-04 (up to date)")
}

func Test_run_sync_asks_for_nothing_when_every_transaction_is_dated_after_today(t *testing.T) {
	stdout, requests := syncFutureDatedBundle(t)

	assert.Empty(t, requests)
	assert.Contains(t, outputLines(stdout), "Rates     USD/CAD 2017-01-03 to 2017-01-04 (up to date)")
}

func Test_run_sync_json_reports_nothing_added_when_every_transaction_is_dated_after_today(t *testing.T) {
	stdout, _ := syncFutureDatedBundle(t, "--json")

	assert.Equal(t, ratesDoc{First: new("2017-01-03"), Last: new("2017-01-04")}, decodeSyncRates(t, stdout))
}

func outputLines(stdout string) []string { return strings.Split(stdout, "\n") }
