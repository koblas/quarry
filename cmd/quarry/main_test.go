// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"context"
	"io"
	"time"

	"github.com/koblas/quarry/internal/cli"
	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/store"
	"github.com/koblas/quarry/internal/store/duckstore"
)

// fakeRatesText is the Rates line's text after a sync over the one rate testEnv's source returns.
const fakeRatesText = "USD/CAD 2026-01-02 to 2026-01-02 (1 new)"

// fixedRates is the store option that fetches one fixed rate in place of the Bank of Canada; a test building its own
// newServerFactory passes it, or another WithRates, so no sync reaches the network.
func fixedRates() duckstore.Option {
	rate := store.Rate{Date: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), USDCAD: money.Rate(1_250_000), Series: "FXUSDCAD"}
	return duckstore.WithRates(fakeRates{rates: []store.Rate{rate}})
}

// testEnv is defaultEnv with its sync fetching through fixedRates.
func testEnv(stdout, stderr io.Writer) cli.Env {
	env := defaultEnv(stdout, stderr)
	env.NewServer = newServerFactory(fixedRates())
	return env
}

// run is the command line over testEnv: what runProcess is over the real wiring.
func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	return runWith(ctx, args, testEnv(stdout, stderr))
}
