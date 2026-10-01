// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"bytes"
	"context"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/fx"
	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/koblas/quarry/internal/store/duckstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
	env := defaultEnv(&stdout, &stderr)
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
	home := t.TempDir()
	t.Setenv("HOME", home)
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
	home := t.TempDir()
	t.Setenv("HOME", home)
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
