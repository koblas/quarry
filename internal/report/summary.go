package report

import (
	"context"
	"time"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/store"
)

// SummaryRequest is what a summary needs from its caller: the month to summarize and the currency its
// amounts are listed in.
type SummaryRequest struct {
	Month    Month
	Currency money.Currency
}

// Summary is one month read from one open of the store: the store's status, the month's unusually large
// charges, the recurring series new in the month, and net worth at the month end before and at the month's
// end, with the change between them.
type Summary struct {
	Month     Month
	Currency  money.Currency
	Status    store.Status
	Anomalies Anomalies
	Recurring Recurring
	NetWorth  NetWorth
	// Change is nil when the first month end has no balance in any currency.
	Change *NetWorthChange
	// Coverage says whether the store's snapshot was taken at or after the month ended.
	Coverage SnapshotCoverage
}

// SnapshotCoverage is whether the snapshot a store was built from holds all of a month.
type SnapshotCoverage int

// The answers: Covers is the zero value, so a Summary built without a verdict raises no warning.
const (
	// SnapshotCovers: the snapshot was taken at or after the month's end.
	SnapshotCovers SnapshotCoverage = iota
	// SnapshotPredatesMonthEnd: the snapshot was taken before the month ended, so the rest of it is missing.
	SnapshotPredatesMonthEnd
	// SnapshotTimeUnknown: the snapshot's manifest records no time it was taken.
	SnapshotTimeUnknown
)

// coverageOf judges a snapshot taken at taken against month; a zero taken is unknown.
func coverageOf(taken time.Time, month Month) SnapshotCoverage {
	switch {
	case taken.IsZero():
		return SnapshotTimeUnknown
	case !taken.Before(month.closes()):
		return SnapshotCovers
	default:
		return SnapshotPredatesMonthEnd
	}
}

// summaryCommand names summary in its refusals; it equals the cli command word.
const summaryCommand = "summary"

// Summary summarizes req.Month from one read of the store: every section is judged from that read, so a
// store replaced mid-run cannot mix two builds. It refuses like Status.
func (s *Server) Summary(ctx context.Context, req SummaryRequest) (Summary, error) {
	history := store.Window{Since: req.Month.Start.AddDate(0, -1, 0), Until: req.Month.End}
	days := monthEnds(history)
	read, err := s.store.Summary(ctx, store.SummaryParams{Through: req.Month.End, Dates: days})
	if err != nil {
		return Summary{}, s.readRefusal(ctx, summaryCommand, err)
	}
	netWorth := netWorthFrom(read.NetWorth, NetWorthRequest{AsOf: req.Month.End, Window: &history, Currency: req.Currency}, days)
	return Summary{
		Month:     req.Month,
		Currency:  req.Currency,
		Status:    read.Status,
		Anomalies: anomaliesFrom(read.Charges, nil, nil, AnomaliesRequest{Window: req.Month.Window(), Currency: req.Currency}),
		Recurring: recurringFrom(read.Charges, recurringScope{window: req.Month.Window(), today: req.Month.End, currency: req.Currency, keep: newInMonth(req.Month)}),
		NetWorth:  netWorth,
		Change:    netWorth.Change(),
		Coverage:  coverageOf(read.Status.Run.Snapshot.TakenAt, req.Month),
	}, nil
}
