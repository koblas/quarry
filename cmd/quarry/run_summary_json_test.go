package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// summarySnapshotJSON is the document's "snapshot".
type summarySnapshotJSON struct {
	ID          string  `json:"id"`
	TakenAt     *string `json:"taken_at"`
	CoversMonth *bool   `json:"covers_month"`
}

// summaryDatesJSON is the document's "dates".
type summaryDatesJSON struct {
	First *string `json:"first"`
	Last  *string `json:"last"`
}

// summaryFindingsJSON is the document's "findings".
type summaryFindingsJSON struct {
	Open       int  `json:"open"`
	Ignored    *int `json:"ignored"`
	Fixed      int  `json:"fixed"`
	New        int  `json:"new"`
	NewlyFixed int  `json:"newly_fixed"`
}

// summaryAnomaliesJSON is the document's "anomalies".
type summaryAnomaliesJSON struct {
	Checked   int           `json:"checked"`
	NotJudged int           `json:"not_judged"`
	Charges   []anomalyJSON `json:"charges"`
}

// summaryRecurringJSON is the document's "recurring".
type summaryRecurringJSON struct {
	Series []recurringSeriesJSON `json:"series"`
	Totals []recurringTotalJSON  `json:"totals"`
}

// summaryBalanceJSON is one entry of a net-worth date's "balances".
type summaryBalanceJSON struct {
	Type             string  `json:"type"`
	Currency         string  `json:"currency"`
	Balance          string  `json:"balance"`
	ConvertedBalance *string `json:"converted_balance"`
}

// summaryMoneyJSON is one entry of "totals", in a net-worth date or in the changes.
type summaryMoneyJSON struct {
	Currency string  `json:"currency"`
	Value    *string `json:"value"`
}

// summaryNetWorthDateJSON is one entry of "net_worth.dates".
type summaryNetWorthDateJSON struct {
	Date     string               `json:"date"`
	Balances []summaryBalanceJSON `json:"balances"`
	Totals   []summaryMoneyJSON   `json:"totals"`
}

// summaryChangeTypeJSON is one entry of "net_worth.changes.types".
type summaryChangeTypeJSON struct {
	Type     string  `json:"type"`
	Currency string  `json:"currency"`
	Value    *string `json:"value"`
}

// summaryChangesJSON is "net_worth.changes".
type summaryChangesJSON struct {
	Types  []summaryChangeTypeJSON `json:"types"`
	Totals []summaryMoneyJSON      `json:"totals"`
}

// summaryNetWorthJSON is the document's "net_worth".
type summaryNetWorthJSON struct {
	Dates   []summaryNetWorthDateJSON `json:"dates"`
	Changes summaryChangesJSON        `json:"changes"`
}

// summaryJSONDoc is quarry summary --json's stdout.
type summaryJSONDoc struct {
	Month     string               `json:"month"`
	Since     string               `json:"since"`
	Until     string               `json:"until"`
	Currency  string               `json:"currency"`
	Snapshot  summarySnapshotJSON  `json:"snapshot"`
	Dates     summaryDatesJSON     `json:"dates"`
	Findings  summaryFindingsJSON  `json:"findings"`
	Anomalies summaryAnomaliesJSON `json:"anomalies"`
	Recurring summaryRecurringJSON `json:"recurring"`
	NetWorth  summaryNetWorthJSON  `json:"net_worth"`
	Warnings  []string             `json:"warnings"`
}

// decodeSummaryJSON reads stdout as the summary document, refusing keys the document does not rule.
func decodeSummaryJSON(t *testing.T, stdout string) summaryJSONDoc {
	t.Helper()
	decoder := json.NewDecoder(strings.NewReader(stdout))
	decoder.DisallowUnknownFields()
	var doc summaryJSONDoc

	require.NoError(t, decoder.Decode(&doc), stdout)
	return doc
}

func Test_run_summary_json_prints_the_ruled_document(t *testing.T) {
	seedSummaryStore(t)
	pinLocalZone(t)

	exitCode, stdout, stderr := runSpendCaptureAt(context.Background(), []string{"summary", "--json"}, summaryClock)

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, []string{
		"month", "since", "until", "currency", "snapshot", "dates", "findings", "anomalies", "recurring", "net_worth", "warnings",
	}, topLevelKeys(t, stdout.String()))
	doc := decodeSummaryJSON(t, stdout.String())
	assert.Equal(t, "2026-09", doc.Month)
	assert.Equal(t, "2026-09-01", doc.Since)
	assert.Equal(t, "2026-09-30", doc.Until)
	assert.Equal(t, "CAD", doc.Currency)
	assert.Equal(t, summarySnapshotJSON{ID: "20261001T130512Z", TakenAt: new("2026-10-01T13:05:12Z"), CoversMonth: new(true)}, doc.Snapshot)
	assert.Equal(t, summaryDatesJSON{First: new("2026-01-02"), Last: new("2026-09-15")}, doc.Dates)
	assert.Equal(t, summaryFindingsJSON{Open: 2, Ignored: new(0), Fixed: 1, New: 1, NewlyFixed: 1}, doc.Findings)
	assert.Equal(t, 2, doc.Anomalies.Checked)
	assert.Equal(t, 0, doc.Anomalies.NotJudged)
	require.Len(t, doc.Anomalies.Charges, 1)
	assert.Equal(t, new("Bell Canada"), doc.Anomalies.Charges[0].Payee)
	assert.Equal(t, "412.00", doc.Anomalies.Charges[0].Amount)
	require.Len(t, doc.Recurring.Series, 1)
	assert.Equal(t, "Crave", doc.Recurring.Series[0].Payee)
	assert.Equal(t, []recurringTotalJSON{{Currency: "CAD", PerYear: "271.08"}}, doc.Recurring.Totals)
	require.Len(t, doc.NetWorth.Dates, 2)
	assert.Equal(t, "2026-08-31", doc.NetWorth.Dates[0].Date)
	assert.Equal(t, "2026-09-30", doc.NetWorth.Dates[1].Date)
	assert.Equal(t, summaryChangesJSON{
		Types: []summaryChangeTypeJSON{
			{Type: "chequing", Currency: "CAD", Value: new("588.00")},
			{Type: "credit_card", Currency: "CAD", Value: new("-22.59")},
		},
		Totals: []summaryMoneyJSON{{Currency: "CAD", Value: new("565.41")}},
	}, doc.NetWorth.Changes)
	assert.Equal(t, []string{}, doc.Warnings)
}
