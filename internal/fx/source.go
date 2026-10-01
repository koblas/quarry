package fx

import (
	"context"
	"time"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/store"
)

// Observation is one published day's rate in a series.
type Observation struct {
	Date time.Time
	Rate money.Rate
}

// Source returns a series' published observations within span. An error is any failure to get an answer, and
// Server turns it into the sync's fetch reason; Valet refuses a body past 16 MiB.
type Source interface {
	Observations(ctx context.Context, series string, span store.DateSpan) ([]Observation, error)
}
