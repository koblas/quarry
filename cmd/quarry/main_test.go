package main

import (
	"os"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/store"
	"github.com/koblas/quarry/internal/store/duckstore"
)

// fakeRatesText is the Rates line's text after a sync over the one rate TestMain's source returns.
const fakeRatesText = "USD/CAD 2026-01-02 to 2026-01-02 (1 new)"

// realRatesSource is the source quarry ships, kept before TestMain replaces it.
var realRatesSource = newRatesSource

// TestMain gives every sync a fixed-rate source, so no test in this package reaches the network.
func TestMain(m *testing.M) {
	newRatesSource = func() duckstore.RatesSource {
		return fakeRates{rates: []store.Rate{{Date: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), USDCAD: money.Rate(1_250_000), Series: "FXUSDCAD"}}}
	}
	os.Exit(m.Run())
}
