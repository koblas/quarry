package document_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/report/document"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// anomalyOf is a payee-baseline anomaly of the given payee, category and splits: 412.00 against a
// usual 96.05, 4.3 times, from 38 earlier charges, on 2026-03-02 in Chequing (CAD).
func anomalyOf(payee *string, category *store.ChargeCategory, splits int) report.Anomaly {
	return report.Anomaly{
		Date:    time.Date(2026, time.March, 2, 0, 0, 0, 0, time.UTC),
		Account: store.Account{ID: "acct-1", Name: "Chequing", Currency: "CAD", Active: true},
		Payee:   payee, Currency: "CAD", Amount: 41200, Category: category, ExpenseSplits: splits,
		Baseline: report.BaselinePayee, Usual: 9605, Earlier: 38, TimesTenths: 43,
	}
}

func listed(anomalies ...report.Anomaly) report.Anomalies {
	return report.Anomalies{Window: window, Listed: anomalies, Checked: len(anomalies)}
}

// convertedAnomaly is anomalyOf's USD charge listed in CAD: 250.00 against a usual 50.00, shown as 340.00 and 68.00.
func convertedAnomaly() report.Anomaly {
	an := anomalyOf(new("Hardware"), &store.ChargeCategory{Path: "Home"}, 1)
	an.Account.Currency, an.Currency, an.Amount, an.Usual, an.TimesTenths = "USD", "USD", 25000, 5000, 50
	an.ListedCurrency, an.ListedAmount, an.ListedUsual = "CAD", 34000, 6800
	return an
}

// unconvertedAnomaly is a USD charge that could not be converted to CAD: it stays in USD, with no Listed fields.
func unconvertedAnomaly() report.Anomaly {
	an := convertedAnomaly()
	an.ListedCurrency, an.ListedAmount, an.ListedUsual = "", 0, 0
	return an
}

// anomaliesInJSON renders listed anomalies reported in currency and returns the document.
func anomaliesInJSON(tb testing.TB, currency money.Currency, anomalies ...report.Anomaly) []byte {
	tb.Helper()
	a := listed(anomalies...)
	a.Currency = currency
	return []byte(indented(tb, document.NewAnomalies(a, []string{})))
}

// firstEntry is the first element of the document's "anomalies", still raw.
func firstEntry(tb testing.TB, doc []byte) []byte {
	tb.Helper()
	var parsed struct {
		Anomalies []json.RawMessage `json:"anomalies"`
	}
	require.NoError(tb, json.Unmarshal(doc, &parsed))
	require.NotEmpty(tb, parsed.Anomalies)
	return parsed.Anomalies[0]
}

func Test_NewAnomalies_puts_currency_after_until_and_native_fields_after_usual_in_every_mode(t *testing.T) {
	wantTop := []string{"since", "until", "currency", "account_filter", "anomalies", "checked", "not_judged", "warnings"}
	wantEntry := []string{
		"transaction_id", "date", "account_id", "account", "currency", "payee", "category",
		"amount", "baseline", "usual", "native_currency", "native_amount", "native_usual", "earlier", "times",
	}
	cases := []struct {
		name     string
		currency money.Currency
		anomaly  report.Anomaly
	}{
		{name: "converted", currency: money.CAD, anomaly: convertedAnomaly()},
		{name: "unconverted", currency: money.CAD, anomaly: unconvertedAnomaly()},
		{name: "native", currency: money.Native, anomaly: anomalyOf(new("Rogers"), nil, 0)},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := anomaliesInJSON(t, c.currency, c.anomaly)

			assert.Equal(t, wantTop, topLevelKeys(t, got))
			assert.Equal(t, wantEntry, topLevelKeys(t, firstEntry(t, got)))
		})
	}
}

// anomaliesReadBack is the document as a client decodes it.
type anomaliesReadBack struct {
	Currency  string `json:"currency"`
	Anomalies []struct {
		Currency       string  `json:"currency"`
		Payee          *string `json:"payee"`
		Category       *string `json:"category"`
		Amount         string  `json:"amount"`
		Usual          string  `json:"usual"`
		NativeCurrency string  `json:"native_currency"`
		NativeAmount   string  `json:"native_amount"`
		NativeUsual    string  `json:"native_usual"`
		Times          float64 `json:"times"`
	} `json:"anomalies"`
	Warnings []string `json:"warnings"`
}

func Test_NewAnomalies_reads_back_converted_and_native_values_per_entry(t *testing.T) {
	categoryBaseline := anomalyOf(nil, &store.ChargeCategory{Path: "Home"}, 1)
	categoryBaseline.Baseline = report.BaselineCategory

	got := anomaliesInJSON(t, money.CAD, convertedAnomaly(), unconvertedAnomaly(), categoryBaseline)

	var doc anomaliesReadBack
	require.NoError(t, json.Unmarshal(got, &doc))
	require.Len(t, doc.Anomalies, 3)
	assert.Equal(t, "CAD", doc.Currency)
	converted, unconverted, byCategory := doc.Anomalies[0], doc.Anomalies[1], doc.Anomalies[2]
	assert.Equal(t, []string{"CAD", "340.00", "68.00", "USD", "250.00", "50.00"},
		[]string{converted.Currency, converted.Amount, converted.Usual, converted.NativeCurrency, converted.NativeAmount, converted.NativeUsual})
	assert.InDelta(t, 5.0, converted.Times, 0)
	assert.Equal(t, []string{"USD", "250.00", "50.00", "USD", "250.00", "50.00"},
		[]string{unconverted.Currency, unconverted.Amount, unconverted.Usual, unconverted.NativeCurrency, unconverted.NativeAmount, unconverted.NativeUsual})
	assert.Nil(t, byCategory.Payee)
	assert.Equal(t, "CAD", byCategory.Currency)
	assert.Equal(t, "412.00", byCategory.NativeAmount)
	assert.Equal(t, []string{}, doc.Warnings)
}

func Test_NewAnomalies_holds_native_fields_equal_to_their_twins_in_native_mode(t *testing.T) {
	got := anomaliesInJSON(t, money.Native, unconvertedAnomaly())

	var doc anomaliesReadBack
	require.NoError(t, json.Unmarshal(got, &doc))
	require.Len(t, doc.Anomalies, 1)
	entry := doc.Anomalies[0]
	assert.Equal(t, "native", doc.Currency)
	assert.Equal(t, []string{entry.NativeCurrency, entry.NativeAmount, entry.NativeUsual}, []string{entry.Currency, entry.Amount, entry.Usual})
}

func Test_NewAnomalies_holds_an_empty_array_not_null_when_nothing_is_listed(t *testing.T) {
	got := anomaliesInJSON(t, money.USD)

	assert.Contains(t, string(got), `"anomalies": []`)
	assert.Contains(t, string(got), `"currency": "USD"`)
}

func Test_NewAnomalies_copies_the_warnings_and_turns_nil_into_an_empty_list(t *testing.T) {
	given := []string{"first"}

	got := document.NewAnomalies(listed(), given)
	given[0] = "changed"

	assert.Equal(t, []string{"first"}, got.Warnings)
	assert.Equal(t, []string{}, document.NewAnomalies(listed(), nil).Warnings)
}

func Test_BaselineWord_names_each_baseline_and_nothing_for_an_unknown_one(t *testing.T) {
	assert.Equal(t, "payee", document.BaselineWord(report.BaselinePayee))
	assert.Equal(t, "category", document.BaselineWord(report.BaselineCategory))
	assert.Empty(t, document.BaselineWord(report.AnomalyBaseline(99)))
}
