// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/fx"
	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/koblas/quarry/internal/snapshot"
	"github.com/koblas/quarry/internal/store"
	"github.com/koblas/quarry/internal/store/duckstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeValet is a Bank of Canada Valet stand-in: series name to date to published rate.
// It answers only requests inside start_date..end_date and never reaches the network.
type fakeValet map[string]map[string]string

func (f fakeValet) RoundTrip(req *http.Request) (*http.Response, error) {
	series := strings.TrimSuffix(strings.TrimPrefix(req.URL.Path, "/valet/observations/"), "/json")
	published, ok := f[series]
	if !ok {
		return valetResponse(req, http.StatusNotFound, "{}"), nil
	}
	query := req.URL.Query()
	var observations []string
	for _, date := range slices.Sorted(maps.Keys(published)) {
		if date >= query.Get("start_date") && date <= query.Get("end_date") {
			observations = append(observations, fmt.Sprintf(`{"d":%q,%q:{"v":%q}}`, date, series, published[date]))
		}
	}
	return valetResponse(req, http.StatusOK, `{"observations":[`+strings.Join(observations, ",")+`]}`), nil
}

func valetResponse(req *http.Request, status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Status:     http.StatusText(status),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    req,
	}
}

func Test_run_sync_back_fills_rates_from_the_earliest_transaction(t *testing.T) {
	home := newHome(t)
	b := v9fixture.NewBuilder()
	chequingPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	earliest := time.Date(2005, 3, 1, 0, 0, 0, 0, time.UTC)
	latest := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	for _, day := range []*time.Time{&earliest, &latest} {
		txn := b.Transaction(v9fixture.TransactionRow{Account: chequingPK, Amount: "-5.00", PostedDate: day})
		b.Entry(v9fixture.EntryRow{Parent: txn, Amount: "-5.00"})
	}
	bundle := b.WriteBundle(t, filepath.Join(home, "Documents"))
	valet := fakeValet{
		"IEXE0101": {"2005-03-01": "1.2345", "2005-03-02": "1.2400", "2017-01-02": "1.3427"},
		"FXUSDCAD": {"2017-01-03": "1.3435", "2017-01-04": "1.3315"},
	}
	var stdout, stderr bytes.Buffer
	env := testEnv(&stdout, &stderr)
	env.NewServer = newServerFactory(duckstore.WithRates(fx.NewServer(fx.WithHTTPClient(&http.Client{Transport: valet}))))

	exitCode := runWith(context.Background(), []string{"sync", "--quicken", bundle.Dir}, env)

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Contains(t, strings.Split(stdout.String(), "\n"), "Rates     USD/CAD 2005-03-01 to 2017-01-04 (5 new)")
	var sqlOut, sqlErr bytes.Buffer
	sqlCode := run(context.Background(), []string{"sql", "--csv", "SELECT date, usd_cad, series FROM fx_rates ORDER BY date"}, &sqlOut, &sqlErr)
	require.Equal(t, 0, sqlCode, sqlErr.String())
	assert.Equal(t, ""+
		"date,usd_cad,series\n"+
		"2005-03-01,1.234500,IEXE0101\n"+
		"2005-03-02,1.240000,IEXE0101\n"+
		"2017-01-02,1.342700,IEXE0101\n"+
		"2017-01-03,1.343500,FXUSDCAD\n"+
		"2017-01-04,1.331500,FXUSDCAD\n",
		sqlOut.String())
}

// syncWithValet syncs a one-account bundle, with a transaction on each date in days, against valet and returns exit code, stdout and stderr.
func syncWithValet(t *testing.T, valet fakeValet, days []time.Time, extraArgs ...string) (int, string, string) {
	t.Helper()
	home := newHome(t)
	b := v9fixture.NewBuilder()
	chequingPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	for _, day := range days {
		txn := b.Transaction(v9fixture.TransactionRow{Account: chequingPK, Amount: "-5.00", PostedDate: &day})
		b.Entry(v9fixture.EntryRow{Parent: txn, Amount: "-5.00"})
	}
	bundle := b.WriteBundle(t, filepath.Join(home, "Documents"))
	var stdout, stderr bytes.Buffer
	env := testEnv(&stdout, &stderr)
	env.NewServer = newServerFactory(duckstore.WithRates(fx.NewServer(fx.WithHTTPClient(&http.Client{Transport: valet}))))

	exitCode := runWith(context.Background(), append([]string{"sync", "--quicken", bundle.Dir}, extraArgs...), env)

	return exitCode, stdout.String(), stderr.String()
}

func Test_run_sync_json_reports_the_rates_it_fetched(t *testing.T) {
	valet := fakeValet{"FXUSDCAD": {"2017-01-03": "1.3435", "2017-01-04": "1.3315"}}

	exitCode, stdout, stderr := syncWithValet(t, valet, []time.Time{time.Date(2017, 1, 3, 0, 0, 0, 0, time.UTC)}, "--json")

	require.Equal(t, 0, exitCode, stderr)
	var doc struct {
		Store struct {
			Rates struct {
				First      *string `json:"first"`
				Last       *string `json:"last"`
				Added      int     `json:"added"`
				FetchError *string `json:"fetch_error"`
			} `json:"rates"`
		} `json:"store"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &doc))
	require.NotNil(t, doc.Store.Rates.First)
	require.NotNil(t, doc.Store.Rates.Last)
	assert.Equal(t, "2017-01-03", *doc.Store.Rates.First)
	assert.Equal(t, "2017-01-04", *doc.Store.Rates.Last)
	assert.Equal(t, 2, doc.Store.Rates.Added)
	assert.Nil(t, doc.Store.Rates.FetchError)
}

func Test_run_sync_says_the_bank_has_no_rates_when_it_answers_empty_for_the_transaction_dates(t *testing.T) {
	valet := fakeValet{"FXUSDCAD": {}, "IEXE0101": {}}

	exitCode, stdout, stderr := syncWithValet(t, valet, []time.Time{time.Date(2017, 1, 3, 0, 0, 0, 0, time.UTC)})

	require.Equal(t, 0, exitCode, stderr)
	assert.Empty(t, stderr)
	assert.Contains(t, strings.Split(stdout, "\n"), "Rates     none (the Bank of Canada has no rates for your transaction dates)")
}

func Test_run_sync_says_there_are_no_transactions_to_convert_when_the_file_has_none(t *testing.T) {
	exitCode, stdout, stderr := syncWithValet(t, fakeValet{}, nil)

	require.Equal(t, 0, exitCode, stderr)
	assert.Contains(t, strings.Split(stdout, "\n"), "Rates     none (no transactions to convert)")
}

// errConnectionRefused is the dial fault of an unreachable bank.
var errConnectionRefused = errors.New("connection refused")

// roundTripFunc is an http.RoundTripper made from a function.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

// failing answers every request with err, as a transport that never reaches the bank.
func failing(err error) http.RoundTripper {
	return roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, err })
}

// answering answers every request with status and body.
func answering(status int, body string) http.RoundTripper {
	return roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return valetResponse(req, status, body), nil
	})
}

// legacyDown publishes FXUSDCAD from current and answers 503 to every IEXE0101 request.
func legacyDown(current fakeValet) http.RoundTripper {
	return roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if strings.HasSuffix(req.URL.Path, "/IEXE0101/json") {
			return valetResponse(req, http.StatusServiceUnavailable, ""), nil
		}
		return current.RoundTrip(req)
	})
}

// syncCapturing runs quarry sync with args, fetching rates through bank, and returns exit code, stdout and stderr.
func syncCapturing(t *testing.T, bank http.RoundTripper, args ...string) (int, string, string) {
	t.Helper()
	return syncCapturingRemoving(t, bank, os.Remove, args...)
}

// syncCapturingRemoving is syncCapturing with the Server deleting snapshots through remove.
func syncCapturingRemoving(t *testing.T, bank http.RoundTripper, remove func(string) error, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	env := testEnv(&stdout, &stderr)
	base := newServerFactory(duckstore.WithRates(fx.NewServer(fx.WithHTTPClient(&http.Client{Transport: bank}))))
	env.NewServer = func(ctx context.Context, opts ...snapshot.Option) (*snapshot.Server, error) {
		return base(ctx, append(opts, snapshot.WithRemove(remove))...)
	}

	exitCode := runWith(context.Background(), append([]string{"sync"}, args...), env)

	return exitCode, stdout.String(), stderr.String()
}

// lastFetchError is quarry sql's CSV of the newest import run's rates_fetch_error.
func lastFetchError(t *testing.T) string {
	t.Helper()
	var stdout, stderr bytes.Buffer
	require.Equal(t, 0, run(context.Background(),
		[]string{"sql", "--csv", "SELECT rates_fetch_error FROM import_runs ORDER BY id DESC LIMIT 1"}, &stdout, &stderr), stderr.String())
	return stdout.String()
}

const (
	cannotReach       = "cannot reach www.bankofcanada.ca"
	noAnswerInTime    = "no answer from www.bankofcanada.ca within 30 seconds"
	answered503       = "www.bankofcanada.ca answered 503 Service Unavailable"
	notAList          = "www.bankofcanada.ca sent an answer that is not a list of exchange rates"
	nothingStoredTail = "; the store has no rates, so reports list amounts in each account's own currency; run quarry sync again to retry"
	nothingNewTail    = "; the store has rates from 2017-01-03 to 2017-01-04, and later dates convert at the 2017-01-04 rate; run quarry sync again to retry"
	partialTail       = "; the store has rates from 2017-01-03 to 2017-01-04, and later dates convert at the 2017-01-04 rate; run quarry sync again to fetch the rest"
	noRatesPrefix     = "could not fetch exchange rates from the Bank of Canada: "
	partialPrefix     = "could not fetch every exchange rate from the Bank of Canada: "
	rateDates         = "date\n2017-01-03\n2017-01-04\n"
	noRateDates       = "date\n"
	notRefreshedLine  = "Rates     USD/CAD 2017-01-03 to 2017-01-04 (not refreshed; see warning)"
	partialLine       = "Rates     USD/CAD 2017-01-03 to 2017-01-04 (2 new, not all fetched; see warning)"
	notFetchedLine    = "Rates     none (not fetched; see warning)"
)

// januaryBank publishes FXUSDCAD for 2017-01-03 and 2017-01-04.
func januaryBank() fakeValet {
	return fakeValet{"FXUSDCAD": {"2017-01-03": "1.3435", "2017-01-04": "1.3315"}}
}

// fetchFailureCase is one way the rate fetch falls short, with what a sync prints and records for it.
type fetchFailureCase struct {
	name       string
	days       []time.Time
	earlier    []fakeValet // banks of syncs that succeed before the failing one
	bank       http.RoundTripper
	ratesLine  string
	warning    string
	reason     string
	ratesJSON  string // the store.rates member of --json
	storedDate string
}

// ratesJSON is the store.rates member for a store holding first to last, "" for none.
func ratesJSON(first, last string, added int, reason string) string {
	quoted := func(s string) string {
		if s == "" {
			return "null"
		}
		return `"` + s + `"`
	}
	return fmt.Sprintf(`{"first":%s,"last":%s,"added":%d,"fetch_error":%q}`, quoted(first), quoted(last), added, reason)
}

func fetchFailureCases() []fetchFailureCase {
	return []fetchFailureCase{
		{
			name: "bank unreachable, nothing stored", days: []time.Time{januaryDay(3)},
			bank:      failing(&net.OpError{Op: "dial", Net: "tcp", Err: errConnectionRefused}),
			ratesLine: notFetchedLine, warning: noRatesPrefix + cannotReach + nothingStoredTail, reason: cannotReach,
			ratesJSON: ratesJSON("", "", 0, cannotReach), storedDate: noRateDates,
		},
		{
			name: "request timed out, earlier rates kept", days: []time.Time{januaryDay(3)}, earlier: []fakeValet{januaryBank()},
			bank:      failing(context.DeadlineExceeded),
			ratesLine: notRefreshedLine, warning: noRatesPrefix + noAnswerInTime + nothingNewTail, reason: noAnswerInTime,
			ratesJSON: ratesJSON("2017-01-03", "2017-01-04", 0, noAnswerInTime), storedDate: rateDates,
		},
		{
			name: "HTTP 503, earlier rates kept", days: []time.Time{januaryDay(3)}, earlier: []fakeValet{januaryBank()},
			bank:      answering(http.StatusServiceUnavailable, ""),
			ratesLine: notRefreshedLine, warning: noRatesPrefix + answered503 + nothingNewTail, reason: answered503,
			ratesJSON: ratesJSON("2017-01-03", "2017-01-04", 0, answered503), storedDate: rateDates,
		},
		{
			name: "answer is not a list of rates, nothing stored", days: []time.Time{januaryDay(3)},
			bank:      answering(http.StatusOK, "<html>maintenance</html>"),
			ratesLine: notFetchedLine, warning: noRatesPrefix + notAList + nothingStoredTail, reason: notAList,
			ratesJSON: ratesJSON("", "", 0, notAList), storedDate: noRateDates,
		},
		{
			name: "partial range, the current series kept", days: []time.Time{time.Date(2016, time.December, 30, 0, 0, 0, 0, time.UTC)},
			bank:      legacyDown(januaryBank()),
			ratesLine: partialLine, warning: partialPrefix + answered503 + partialTail, reason: answered503,
			ratesJSON: ratesJSON("2017-01-03", "2017-01-04", 2, answered503), storedDate: rateDates,
		},
	}
}

func Test_run_sync_warns_and_swaps_the_store_in_when_the_rate_fetch_fails(t *testing.T) {
	for _, c := range fetchFailureCases() {
		t.Run(c.name, func(t *testing.T) {
			home := newHome(t)
			bundle := writeChequingBundle(t, filepath.Join(home, "Documents"), c.days...)
			for _, earlier := range c.earlier {
				syncThrough(t, earlier, "--quicken", bundle.Dir)
			}

			exitCode, stdout, stderr := syncCapturing(t, c.bank, "--quicken", bundle.Dir)

			require.Equal(t, 0, exitCode, stderr)
			assert.Equal(t, "quarry: warning: "+c.warning+"\n", stderr)
			assert.Equal(t, c.storedDate, storedRateDates(t))
			assert.Equal(t, "rates_fetch_error\n"+c.reason+"\n", lastFetchError(t))
			assert.Contains(t, strings.Split(stdout, "\n"), c.ratesLine)
		})
	}
}

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
			home := newHome(t)
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
	home := newHome(t)
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
	home := newHome(t)
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
	home := newHome(t)
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
	home := newHome(t)
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
	home := newHome(t)
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
	home := newHome(t)
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
	home := newHome(t)
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
	home := newHome(t)
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
	home := newHome(t)
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
	home := newHome(t)
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
	home := newHome(t)
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
	home := newHome(t)
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

// recordingValet logs each request ("<series> <start_date>") before answering it through next.
type recordingValet struct {
	next http.RoundTripper

	mu       sync.Mutex
	requests []string
}

func (r *recordingValet) RoundTrip(req *http.Request) (*http.Response, error) {
	series := strings.TrimSuffix(strings.TrimPrefix(req.URL.Path, "/valet/observations/"), "/json")
	r.mu.Lock()
	r.requests = append(r.requests, series+" "+req.URL.Query().Get("start_date"))
	r.mu.Unlock()
	return r.next.RoundTrip(req)
}

// requested returns the requests logged so far.
func (r *recordingValet) requested() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.requests...)
}

// januaryDay is the given day of January 2017.
func januaryDay(d int) time.Time { return time.Date(2017, time.January, d, 0, 0, 0, 0, time.UTC) }

// writeChequingBundle writes a one-account Quicken bundle under dir with a transaction on each of days.
func writeChequingBundle(t *testing.T, dir string, days ...time.Time) v9fixture.Bundle {
	t.Helper()
	b := v9fixture.NewBuilder()
	chequingPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	for _, day := range days {
		txn := b.Transaction(v9fixture.TransactionRow{Account: chequingPK, Amount: "-5.00", PostedDate: &day})
		b.Entry(v9fixture.EntryRow{Parent: txn, Amount: "-5.00"})
	}
	return b.WriteBundle(t, dir)
}

// syncThrough runs quarry sync with args, fetching rates through valet, and returns its stdout.
func syncThrough(t *testing.T, valet http.RoundTripper, args ...string) string {
	t.Helper()
	var stdout, stderr bytes.Buffer
	env := testEnv(&stdout, &stderr)
	env.NewServer = newServerFactory(duckstore.WithRates(fx.NewServer(fx.WithHTTPClient(&http.Client{Transport: valet}))))

	exitCode := runWith(context.Background(), append([]string{"sync"}, args...), env)

	require.Equal(t, 0, exitCode, stderr.String())
	return stdout.String()
}

// storedRateDates is quarry sql's CSV of the dates held in fx_rates.
func storedRateDates(t *testing.T) string {
	t.Helper()
	var stdout, stderr bytes.Buffer
	require.Equal(t, 0, run(context.Background(), []string{"sql", "--csv", "SELECT date FROM fx_rates ORDER BY date"}, &stdout, &stderr), stderr.String())
	return stdout.String()
}

func Test_run_sync_from_an_older_snapshot_keeps_every_carried_rate(t *testing.T) {
	home := newHome(t)
	bundleA := writeChequingBundle(t, filepath.Join(home, "A"), januaryDay(3))
	bundleB := writeChequingBundle(t, filepath.Join(home, "B"), januaryDay(3), januaryDay(10))
	syncThrough(t, fakeValet{"FXUSDCAD": {"2017-01-03": "1.3435", "2017-01-04": "1.3315"}}, "--quicken", bundleA.Dir)
	idA := snapshotID(onlyFileWithSuffix(t, snapshotsDirUnder(home), ".sqlite"))
	syncThrough(t, fakeValet{"FXUSDCAD": {"2017-01-05": "1.3300", "2017-01-06": "1.3350"}}, "--quicken", bundleB.Dir)
	source := &recordingValet{next: fakeValet{"FXUSDCAD": {}, "IEXE0101": {}}}

	stdout := syncThrough(t, source, "--from", idA)

	assert.Equal(t, "date\n2017-01-03\n2017-01-04\n2017-01-05\n2017-01-06\n", storedRateDates(t))
	assert.Equal(t, []string{"FXUSDCAD 2017-01-07"}, source.requested())
	assert.Contains(t, strings.Split(stdout, "\n"), "Rates     USD/CAD 2017-01-03 to 2017-01-06 (up to date)")
}

func Test_run_sync_asks_only_for_the_dates_after_the_last_stored_rate(t *testing.T) {
	home := newHome(t)
	bundle := writeChequingBundle(t, filepath.Join(home, "Documents"), januaryDay(3))
	syncThrough(t, fakeValet{"FXUSDCAD": {"2017-01-03": "1.3435", "2017-01-04": "1.3315"}}, "--quicken", bundle.Dir)
	source := &recordingValet{next: fakeValet{"FXUSDCAD": {
		"2017-01-03": "1.3435", "2017-01-04": "1.3315", "2017-01-05": "1.3300", "2017-01-06": "1.3350",
	}}}

	stdout := syncThrough(t, source, "--quicken", bundle.Dir)

	assert.Equal(t, []string{"FXUSDCAD 2017-01-05"}, source.requested())
	assert.Equal(t, "date\n2017-01-03\n2017-01-04\n2017-01-05\n2017-01-06\n", storedRateDates(t))
	assert.Contains(t, strings.Split(stdout, "\n"), "Rates     USD/CAD 2017-01-03 to 2017-01-06 (2 new)")
}

// countingRates is a RatesSource that counts the times it is asked and answers with no rates.
type countingRates struct{ calls int }

func (c *countingRates) Refresh(context.Context, store.RatesRequest) (store.RatesRefresh, error) {
	c.calls++
	return store.RatesRefresh{}, nil
}

// countedSync is what quarry sync printed and how many rate requests it made.
type countedSync struct {
	requests, exitCode int
	stdout, stderr     string
}

// syncCountingRateRequests runs quarry sync on bundle with a rates source that counts its calls.
func syncCountingRateRequests(bundle v9fixture.Bundle, args ...string) countedSync {
	var out, errOut bytes.Buffer
	counter := &countingRates{}
	env := testEnv(&out, &errOut)
	env.NewServer = newServerFactory(duckstore.WithRates(counter))

	exitCode := runWith(context.Background(), append([]string{"sync", "--quicken", bundle.Dir}, args...), env)

	return countedSync{requests: counter.calls, exitCode: exitCode, stdout: out.String(), stderr: errOut.String()}
}

func closedWALBundle(t *testing.T, dir string) v9fixture.Bundle {
	t.Helper()
	return v9fixture.ClosedWALBundle(t, dir)
}

func missingSchemaBundle(t *testing.T, dir string) v9fixture.Bundle {
	t.Helper()
	return v9fixture.MissingSchemaBundle(t, dir)
}

// failsBeforeTheSwap is a bundle sync refuses before the store is swapped in.
type failsBeforeTheSwap struct {
	name  string
	write func(*testing.T, string) v9fixture.Bundle
}

func failuresBeforeTheSwap() []failsBeforeTheSwap {
	return []failsBeforeTheSwap{
		{"the bundle is not open in Quicken", closedWALBundle},
		{"validation fails", unreconciledBundle},
		{"the schema changed", missingSchemaBundle},
	}
}

func hasRatesLine(stdout string) bool {
	for line := range strings.SplitSeq(stdout, "\n") {
		if strings.HasPrefix(line, "Rates ") {
			return true
		}
	}
	return false
}

func Test_run_sync_makes_no_rate_request_when_it_fails_before_the_swap(t *testing.T) {
	for _, c := range failuresBeforeTheSwap() {
		t.Run(c.name, func(t *testing.T) {
			home := newHome(t)
			bundle := c.write(t, filepath.Join(home, "Documents"))

			got := syncCountingRateRequests(bundle)

			assert.Equal(t, 1, got.exitCode)
			assert.Zero(t, got.requests)
			assert.False(t, hasRatesLine(got.stdout), got.stdout)
		})
	}
}

func Test_run_sync_makes_one_rate_request_when_the_store_is_built(t *testing.T) {
	home := newHome(t)
	bundle := writeChequingBundle(t, filepath.Join(home, "Documents"), januaryDay(3))

	got := syncCountingRateRequests(bundle)

	require.Equal(t, 0, got.exitCode, got.stderr)
	assert.Equal(t, 1, got.requests)
	assert.True(t, hasRatesLine(got.stdout), got.stdout)
}

func Test_run_sync_json_makes_no_rate_request_when_it_fails_before_the_swap(t *testing.T) {
	for _, c := range failuresBeforeTheSwap() {
		t.Run(c.name, func(t *testing.T) {
			home := newHome(t)
			bundle := c.write(t, filepath.Join(home, "Documents"))

			got := syncCountingRateRequests(bundle, "--json")

			assert.Equal(t, 1, got.exitCode)
			assert.Zero(t, got.requests)
		})
	}
}

func Test_run_sync_json_prints_nothing_when_the_bundle_is_not_open_in_quicken(t *testing.T) {
	home := newHome(t)
	bundle := v9fixture.ClosedWALBundle(t, filepath.Join(home, "Documents"))

	got := syncCountingRateRequests(bundle, "--json")

	assert.Empty(t, got.stdout)
}

func Test_run_sync_json_reports_no_store_when_the_schema_changed(t *testing.T) {
	home := newHome(t)
	bundle := v9fixture.MissingSchemaBundle(t, filepath.Join(home, "Documents"))

	got := syncCountingRateRequests(bundle, "--json")

	var doc map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(got.stdout), &doc))
	assert.JSONEq(t, "null", string(doc["store"]))
}

func Test_run_sync_json_reports_no_rates_when_validation_fails(t *testing.T) {
	home := newHome(t)
	bundle := unreconciledBundle(t, filepath.Join(home, "Documents"))

	got := syncCountingRateRequests(bundle, "--json")

	var doc struct {
		Store map[string]json.RawMessage `json:"store"`
	}
	require.NoError(t, json.Unmarshal([]byte(got.stdout), &doc))
	assert.JSONEq(t, "null", string(doc.Store["rates"]))
}

func Test_run_sync_json_makes_one_rate_request_and_reports_rates_when_the_store_is_built(t *testing.T) {
	home := newHome(t)
	bundle := writeChequingBundle(t, filepath.Join(home, "Documents"), januaryDay(3))

	got := syncCountingRateRequests(bundle, "--json")

	require.Equal(t, 0, got.exitCode, got.stderr)
	assert.Equal(t, 1, got.requests)
	var doc struct {
		Store struct {
			Rates json.RawMessage `json:"rates"`
		} `json:"store"`
	}
	require.NoError(t, json.Unmarshal([]byte(got.stdout), &doc))
	assert.JSONEq(t, `{"first":null,"last":null,"added":0,"fetch_error":null}`, string(doc.Store.Rates))
}
