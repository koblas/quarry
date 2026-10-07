// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/koblas/quarry/internal/store"
	"github.com/koblas/quarry/internal/store/duckstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
