package report

import (
	"cmp"
	"context"
	"slices"
	"time"

	"github.com/koblas/quarry/internal/store"
)

// The thresholds a charge must pass to be listed; amounts are cents.
const (
	// AnomalyPayeeMinHistory is how many earlier charges a payee needs before it has a baseline.
	AnomalyPayeeMinHistory = 3
	// AnomalyPayeeMultiplier is how many times the payee's median a charge must exceed to be listed.
	AnomalyPayeeMultiplier = 2
	// AnomalyMinAmount is the amount below which a charge is never listed (100.00).
	AnomalyMinAmount int64 = 10000
)

// anomaliesCommand names anomalies in its refusals; it equals the cli command word.
const anomaliesCommand = "anomalies"

// AnomalyBaseline is what an anomaly's charge was compared with.
type AnomalyBaseline int

// The baselines an anomaly can be judged against.
const (
	// BaselinePayee compares a charge with the payee's earlier charges.
	BaselinePayee AnomalyBaseline = iota
)

// AnomaliesRequest is what an anomalies read needs from its caller: the window of charges to
// list, the clock reading that decides today, and the accounts whose charges to list (each an
// id or a name; none means every account).
type AnomaliesRequest struct {
	Window   store.Window
	Now      time.Time
	Accounts []string
}

// Anomaly is one charge unusually large for its baseline.
type Anomaly struct {
	store.Charge

	Baseline AnomalyBaseline
	// Usual is the median of the baseline's earlier charges, in cents.
	Usual int64
	// Earlier is how many earlier charges the baseline held.
	Earlier int
	// TimesTenths is the charge as a multiple of Usual, in tenths.
	TimesTenths int64
}

// Anomalies is an anomalies read: the window it listed and the charges unusually large in it.
type Anomalies struct {
	Window store.Window
	Listed []Anomaly
	// Checked is how many charges the window held; NotJudged is how many of them had no baseline.
	Checked, NotJudged int
	// Transactions is store.Charges.Transactions: the span of the store's transactions.
	Transactions store.TransactionRange
}

// Anomalies lists the charges dated in req.Window that are unusually large for their payee, judged
// against the payee's strictly earlier charges in every account. A store it cannot read is a RefusalError.
func (s *Server) Anomalies(ctx context.Context, req AnomaliesRequest) (Anomalies, error) {
	today := DefaultWindow(req.Now).Until
	charges, err := s.store.Charges(ctx, store.ChargeParams{Through: today})
	if err != nil {
		return Anomalies{}, s.readRefusal(ctx, anomaliesCommand, err)
	}
	result := Anomalies{Window: req.Window, Transactions: charges.Transactions}
	for _, c := range charges.Rows {
		if _, ok := chargeKey(c); !ok && inWindow(req.Window, c.Date) {
			result.tally(judge(c, nil))
		}
	}
	for _, group := range groupCharges(charges.Rows) {
		dayStart := 0
		for i, c := range group.charges {
			if i > 0 && !c.Date.Equal(group.charges[i-1].Date) {
				dayStart = i
			}
			if inWindow(req.Window, c.Date) {
				result.tally(judge(c, group.charges[:dayStart]))
			}
		}
	}
	slices.SortFunc(result.Listed, compareAnomalies)
	return result, nil
}

// tally counts a judged charge, and lists it when it is unusual.
func (a *Anomalies) tally(anomaly Anomaly, v verdict) {
	a.Checked++
	switch v {
	case verdictUnusual:
		a.Listed = append(a.Listed, anomaly)
	case verdictNotJudged:
		a.NotJudged++
	case verdictUsual:
	}
}

// verdict is how one charge came out.
type verdict int

const (
	// verdictUsual is a judged charge that is not unusual, one under AnomalyMinAmount included.
	verdictUsual verdict = iota
	// verdictUnusual is a charge over AnomalyPayeeMultiplier times its payee's median.
	verdictUnusual
	// verdictNotJudged is a charge of AnomalyMinAmount or more with too little history to judge.
	verdictNotJudged
)

// judge is the verdict on c against earlier, its payee's charges on strictly earlier dates, and
// the anomaly it makes when unusual.
func judge(c store.Charge, earlier []store.Charge) (Anomaly, verdict) {
	if c.Amount < AnomalyMinAmount {
		return Anomaly{}, verdictUsual
	}
	if len(earlier) < AnomalyPayeeMinHistory {
		return Anomaly{}, verdictNotJudged
	}
	amounts := make([]int64, len(earlier))
	for i, e := range earlier {
		amounts[i] = e.Amount
	}
	usual := medianCents(amounts)
	if c.Amount <= AnomalyPayeeMultiplier*usual {
		return Anomaly{}, verdictUsual
	}
	return Anomaly{
		Charge:      c,
		Baseline:    BaselinePayee,
		Usual:       usual,
		Earlier:     len(earlier),
		TimesTenths: timesTenths(c.Amount, usual),
	}, verdictUnusual
}

// tenthsPerWhole converts a whole multiple to tenths.
const tenthsPerWhole = 10

// timesTenths is amount as a multiple of usual in tenths, half away from zero.
func timesTenths(amount, usual int64) int64 {
	return (amount*tenthsPerWhole*2 + usual) / (2 * usual)
}

// inWindow is whether day falls in window, both ends included.
func inWindow(window store.Window, day time.Time) bool {
	return !day.Before(window.Since) && !day.After(window.Until)
}

// compareAnomalies orders anomalies newest first, then by descending source id.
func compareAnomalies(a, b Anomaly) int {
	return cmp.Or(b.Date.Compare(a.Date), cmp.Compare(b.SourceID, a.SourceID))
}
