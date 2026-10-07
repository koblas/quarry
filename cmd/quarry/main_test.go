// Tests for the command line live in package main, since runProcess and the factories are unexported.
// run is the test wiring: runProcess's command line over testEnv, whose sync never reaches the network.
package main

import (
	"context"
	"errors"
	"io"
	"os/exec"
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

// errNoChildren is what testEnv's RunTool returns: no test may start a child process.
var errNoChildren = errors.New("test wiring starts no child process")

// testEnv is defaultEnv with its sync fetching through fixedRates and no way to find or start a child.
func testEnv(stdout, stderr io.Writer) cli.Env {
	env := defaultEnv(stdout, stderr)
	env.NewServer = newServerFactory(fixedRates())
	env.RunTool = func(context.Context, string, ...string) ([]byte, []byte, int, error) {
		return nil, nil, -1, errNoChildren
	}
	env.LookPath = func(file string) (string, error) { return "", &exec.Error{Name: file, Err: exec.ErrNotFound} }
	return env
}

// run is the command line over testEnv: what runProcess is over the real wiring.
func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	return runWith(ctx, args, testEnv(stdout, stderr))
}
