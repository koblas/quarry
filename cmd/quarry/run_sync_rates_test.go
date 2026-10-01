// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"maps"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/fx"
	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
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
	home := t.TempDir()
	t.Setenv("HOME", home)
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
	env := defaultEnv(&stdout, &stderr)
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
