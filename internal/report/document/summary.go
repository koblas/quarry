package document

import "github.com/koblas/quarry/internal/report"

// Summary is summary's --json document; Currency is the reporting currency ("native" lists every amount in its
// own). Every array is [] rather than null when it holds nothing.
type Summary struct {
	Month     string           `json:"month"`
	Since     string           `json:"since"`
	Until     string           `json:"until"`
	Currency  string           `json:"currency"`
	Snapshot  SummarySnapshot  `json:"snapshot"`
	Dates     StatusDates      `json:"dates"`
	Findings  StatusFindings   `json:"findings"`
	Anomalies SummaryAnomalies `json:"anomalies"`
	Recurring SummaryRecurring `json:"recurring"`
	NetWorth  SummaryNetWorth  `json:"net_worth"`
	Warnings  []string         `json:"warnings"`
}

// SummarySnapshot is the "snapshot" object; TakenAt is null when the manifest did not record it, and
// CoversMonth then too.
type SummarySnapshot struct {
	ID          string  `json:"id"`
	TakenAt     *string `json:"taken_at"`
	CoversMonth *bool   `json:"covers_month"`
}

// SummaryAnomalies is the "anomalies" object: the month's anomalies as `quarry anomalies` lists them.
type SummaryAnomalies struct {
	Checked   int       `json:"checked"`
	NotJudged int       `json:"not_judged"`
	Charges   []Anomaly `json:"charges"`
}

// SummaryRecurring is the "recurring" object: the series new in the month and their yearly totals.
type SummaryRecurring struct {
	Series []RecurringSeries `json:"series"`
	Totals []RecurringTotal  `json:"totals"`
}

// SummaryNetWorth is the "net_worth" object: the month end before and the month's end, and the change.
type SummaryNetWorth struct {
	Dates   []NetWorthDate `json:"dates"`
	Changes SummaryChanges `json:"changes"`
}

// SummaryChanges is "changes": how net worth moved between the two month ends; both arrays are empty when the
// first month end holds no balance.
type SummaryChanges struct {
	Types  []SummaryChangeType  `json:"types"`
	Totals []SummaryChangeTotal `json:"totals"`
}

// SummaryChangeType is one entry of "types"; Value is null when a rate is missing.
type SummaryChangeType struct {
	Type     string  `json:"type"`
	Currency string  `json:"currency"`
	Value    *string `json:"value"`
}

// SummaryChangeTotal is one entry of the changes' "totals"; Value is null when a rate is missing.
type SummaryChangeTotal struct {
	Currency string  `json:"currency"`
	Value    *string `json:"value"`
}

// NewSummary converts s into summary's document with the findings tally and warnings; every array is []
// rather than null when s holds none.
func NewSummary(s report.Summary, findings FindingsTally, warnings []string) Summary {
	run := s.Status.Run
	return Summary{
		Month:    s.Month.String(),
		Since:    s.Month.Start.Format(DateLayout),
		Until:    s.Month.End.Format(DateLayout),
		Currency: s.Currency.String(),
		Snapshot: SummarySnapshot{
			ID:          report.SnapshotID(run.Snapshot.Path),
			TakenAt:     nullTimestamp(run.Snapshot.TakenAt),
			CoversMonth: coversMonth(s.Coverage),
		},
		Dates:     StatusDates{First: nullDate(s.Status.FirstDate), Last: nullDate(s.Status.LastDate)},
		Findings:  newStatusFindings(findings),
		Anomalies: summaryAnomalies(s.Anomalies),
		Recurring: summaryRecurring(s.Recurring),
		NetWorth:  SummaryNetWorth{Dates: netWorthDates(s.NetWorth), Changes: summaryChanges(s.Change)},
		Warnings:  append([]string{}, warnings...),
	}
}

// coversMonth is the verdict as the document's "covers_month": null when the snapshot's time is unknown.
func coversMonth(c report.SnapshotCoverage) *bool {
	switch c {
	case report.SnapshotCovers:
		return new(true)
	case report.SnapshotPredatesMonthEnd:
		return new(false)
	case report.SnapshotTimeUnknown:
		return nil
	}
	return nil
}

// summaryAnomalies converts a into the "anomalies" object.
func summaryAnomalies(a report.Anomalies) SummaryAnomalies {
	return SummaryAnomalies{Checked: a.Checked, NotJudged: a.NotJudged, Charges: anomalyEntries(a)}
}

// summaryRecurring converts r into the "recurring" object.
func summaryRecurring(r report.Recurring) SummaryRecurring {
	return SummaryRecurring{Series: recurringSeriesEntries(r), Totals: recurringTotalEntries(r)}
}

// summaryChanges converts c into "changes"; nil gives both arrays empty.
func summaryChanges(c *report.NetWorthChange) SummaryChanges {
	changes := SummaryChanges{Types: []SummaryChangeType{}, Totals: []SummaryChangeTotal{}}
	if c == nil {
		return changes
	}
	for _, t := range c.Types {
		changes.Types = append(changes.Types, SummaryChangeType{Type: t.Type, Currency: t.Currency, Value: nullableMoney(t.Value)})
	}
	for _, t := range c.Totals {
		changes.Totals = append(changes.Totals, SummaryChangeTotal{Currency: t.Currency, Value: nullableMoney(t.Value)})
	}
	return changes
}
