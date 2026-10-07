package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/cli"
	"github.com/koblas/quarry/internal/config"
	"github.com/koblas/quarry/internal/finding"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// statusStore answers Status from one build and Findings from another, as a sync between two reads would.
type statusStore struct {
	report.Store

	status   store.Status
	findings store.FindingList
}

func (f statusStore) Status(context.Context) (store.Status, error) { return f.status, nil }

func (f statusStore) Findings(context.Context) (store.FindingList, error) { return f.findings, nil }

func Test_status_counts_the_findings_from_the_same_read_as_the_rest_of_the_status(t *testing.T) {
	fake := statusStore{
		status:   store.Status{Findings: []store.Finding{{ID: "duplicate:txn-1+txn-2", Type: finding.Duplicate}}},
		findings: store.FindingList{Findings: []store.Finding{{ID: "duplicate:txn-3+txn-4", Type: finding.Duplicate}, {ID: "duplicate:txn-5+txn-6", Type: finding.Duplicate}}},
	}
	var stdout bytes.Buffer
	env := reportEnv(fake, &stdout, &bytes.Buffer{}, atWallClock, withConfig(config.Config{}))

	err := cli.Execute(t.Context(), []string{"status"}, env)

	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "Findings  1 open; run quarry findings to list them\n")
}

// ratesNow is 2026-10-01 23:30 in the pinned zone and 2026-10-02 in UTC, so an age taken in either
// the wrong zone or from a truncated duration reads one day too many.
var ratesNow = time.Date(2026, 10, 2, 3, 30, 0, 0, time.UTC)

func ratesDay(month time.Month, day int) time.Time {
	return time.Date(2026, month, day, 0, 0, 0, 0, time.UTC)
}

// runStatusWith runs quarry status (with --json when asJSON) over st at ratesNow and returns stdout.
func runStatusWith(t *testing.T, st store.Status, asJSON bool) string {
	t.Helper()
	cli.UseZone(t, time.FixedZone("EDT", -4*60*60))
	var stdout bytes.Buffer
	env := reportEnv(statusStore{status: st}, &stdout, &bytes.Buffer{}, atTime(ratesNow), withConfig(config.Config{}))
	args := []string{"status"}
	if asJSON {
		args = append(args, "--json")
	}

	require.NoError(t, cli.Execute(t.Context(), args, env))

	return stdout.String()
}

func Test_status_prints_the_rates_line_for_each_coverage_state(t *testing.T) {
	firstTransaction := ratesDay(time.January, 1)
	cases := []struct {
		name  string
		rates store.StatusRates
		want  string
		json  string
	}{
		{
			name:  "last rate today",
			rates: store.StatusRates{First: ratesDay(time.January, 1), Last: ratesDay(time.October, 1)},
			want:  "Rates     USD/CAD from the Bank of Canada, 2026-01-01 to 2026-10-01 (today)",
			json:  `{"first": "2026-01-01", "last": "2026-10-01", "fetch_error": null}`,
		},
		{
			name:  "last rate one day ago",
			rates: store.StatusRates{First: ratesDay(time.January, 1), Last: ratesDay(time.September, 30)},
			want:  "Rates     USD/CAD from the Bank of Canada, 2026-01-01 to 2026-09-30 (1 day ago)",
			json:  `{"first": "2026-01-01", "last": "2026-09-30", "fetch_error": null}`,
		},
		{
			name:  "last rate two days ago",
			rates: store.StatusRates{First: ratesDay(time.January, 1), Last: ratesDay(time.September, 29)},
			want:  "Rates     USD/CAD from the Bank of Canada, 2026-01-01 to 2026-09-29 (2 days ago)",
			json:  `{"first": "2026-01-01", "last": "2026-09-29", "fetch_error": null}`,
		},
		{
			name:  "first rate after the first transaction",
			rates: store.StatusRates{First: ratesDay(time.January, 2), Last: ratesDay(time.October, 1)},
			want:  "Rates     USD/CAD from the Bank of Canada, 2026-01-02 to 2026-10-01 (today); transactions before 2026-01-02 are not converted",
			json:  `{"first": "2026-01-02", "last": "2026-10-01", "fetch_error": null}`,
		},
		{
			name: "last fetch failed",
			rates: store.StatusRates{
				First: ratesDay(time.January, 1), Last: ratesDay(time.September, 30),
				FetchError: "www.bankofcanada.ca answered 503 Service Unavailable",
			},
			want: "Rates     USD/CAD from the Bank of Canada, 2026-01-01 to 2026-09-30 (1 day ago); " +
				"the last sync could not fetch new rates: www.bankofcanada.ca answered 503 Service Unavailable",
			json: `{"first": "2026-01-01", "last": "2026-09-30", "fetch_error": "www.bankofcanada.ca answered 503 Service Unavailable"}`,
		},
		{
			name:  "none",
			rates: store.StatusRates{},
			want:  "Rates     none, so amounts are not converted; run quarry sync to fetch them from the Bank of Canada",
			json:  `{"first": null, "last": null, "fetch_error": null}`,
		},
	}

	for _, c := range cases {
		t.Run(c.name+" in text", func(t *testing.T) {
			st := store.Status{FirstDate: firstTransaction, LastDate: ratesDay(time.September, 1), Rates: c.rates}

			out := runStatusWith(t, st, false)

			assert.Contains(t, out, "\n"+c.want+"\n")
		})
		t.Run(c.name+" in json", func(t *testing.T) {
			st := store.Status{FirstDate: firstTransaction, LastDate: ratesDay(time.September, 1), Rates: c.rates}

			out := runStatusWith(t, st, true)

			var doc struct {
				Rates json.RawMessage `json:"rates"`
			}
			require.NoError(t, json.Unmarshal([]byte(out), &doc))
			assert.JSONEq(t, c.json, string(doc.Rates))
		})
	}
}

func Test_status_rates_line_edge_rows(t *testing.T) {
	const reason = "cannot reach www.bankofcanada.ca"
	cases := []struct {
		name             string
		firstTransaction time.Time
		rates            store.StatusRates
		want             string
	}{
		{
			name:             "first rate before the first transaction adds no clause",
			firstTransaction: ratesDay(time.March, 1),
			rates:            store.StatusRates{First: ratesDay(time.January, 1), Last: ratesDay(time.October, 1)},
			want:             "Rates     USD/CAD from the Bank of Canada, 2026-01-01 to 2026-10-01 (today)",
		},
		{
			name:  "no transactions adds no clause",
			rates: store.StatusRates{First: ratesDay(time.January, 1), Last: ratesDay(time.October, 1)},
			want:  "Rates     USD/CAD from the Bank of Canada, 2026-01-01 to 2026-10-01 (today)",
		},
		{
			name:             "last rate after today reads today",
			firstTransaction: ratesDay(time.January, 1),
			rates:            store.StatusRates{First: ratesDay(time.January, 1), Last: ratesDay(time.October, 5)},
			want:             "Rates     USD/CAD from the Bank of Canada, 2026-01-01 to 2026-10-05 (today)",
		},
		{
			name:             "first rate after the first transaction and a failed fetch give both clauses in order",
			firstTransaction: ratesDay(time.January, 1),
			rates:            store.StatusRates{First: ratesDay(time.January, 2), Last: ratesDay(time.September, 29), FetchError: reason},
			want: "Rates     USD/CAD from the Bank of Canada, 2026-01-02 to 2026-09-29 (2 days ago); " +
				"transactions before 2026-01-02 are not converted; the last sync could not fetch new rates: " + reason,
		},
		{
			name:             "no rates and a failed fetch",
			firstTransaction: ratesDay(time.January, 1),
			rates:            store.StatusRates{FetchError: reason},
			want: "Rates     none, so amounts are not converted; run quarry sync to fetch them from the Bank of Canada; " +
				"the last sync could not fetch new rates: " + reason,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			st := store.Status{FirstDate: c.firstTransaction, LastDate: c.firstTransaction, Rates: c.rates}

			out := runStatusWith(t, st, false)

			assert.Contains(t, out, "\n"+c.want+"\n")
		})
	}
}

func Test_status_json_carries_the_fetch_error_when_there_are_no_rates(t *testing.T) {
	st := store.Status{Rates: store.StatusRates{FetchError: "cannot reach www.bankofcanada.ca"}}

	out := runStatusWith(t, st, true)

	var doc struct {
		Rates json.RawMessage `json:"rates"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &doc))
	assert.JSONEq(t, `{"first": null, "last": null, "fetch_error": "cannot reach www.bankofcanada.ca"}`, string(doc.Rates))
}
