// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/fx"
	"github.com/koblas/quarry/internal/snapshot"
	"github.com/koblas/quarry/internal/store/duckstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
