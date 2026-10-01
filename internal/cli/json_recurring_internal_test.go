// White-box: renderRecurringJSON's null/[] choices and number formats are document rules,
// driven directly over a report.Recurring and read back with encoding/json.
package cli

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recurringDocumentOf renders r with warnings and reads the document back as generic JSON.
func recurringDocumentOf(t *testing.T, r report.Recurring, warnings ...string) map[string]any {
	t.Helper()
	out, err := renderRecurringJSON(r, append([]string{}, warnings...))
	require.NoError(t, err)
	var doc map[string]any
	require.NoError(t, json.Unmarshal(out, &doc), string(out))
	return doc
}

// seriesEntryAt is the i-th entry of doc's "series".
func seriesEntryAt(t *testing.T, doc map[string]any, i int) map[string]any {
	t.Helper()
	entries, ok := doc["series"].([]any)
	require.True(t, ok)
	require.Greater(t, len(entries), i)
	entry, ok := entries[i].(map[string]any)
	require.True(t, ok)
	return entry
}

// firstSeriesOf renders a document of the one series s and returns its entry.
func firstSeriesOf(t *testing.T, s report.Series) map[string]any {
	t.Helper()
	return seriesEntryAt(t, recurringDocumentOf(t, report.Recurring{Window: spendingWindow(), Series: []report.Series{s}}), 0)
}

func Test_renderRecurringJSON_writes_empty_arrays_not_null_for_a_report_with_no_series(t *testing.T) {
	doc := recurringDocumentOf(t, report.Recurring{Window: spendingWindow()})

	for _, key := range []string{"account_filter", "series", "totals", "warnings"} {
		assert.Equal(t, []any{}, doc[key], key)
	}
}

func Test_renderRecurringJSON_writes_empty_arrays_not_null_for_a_series_with_no_lists(t *testing.T) {
	entry := firstSeriesOf(t, report.Series{Payee: "A", Currency: "CAD", Cadence: report.CadenceMonthly})

	for _, key := range []string{"payees", "accounts", "price_changes"} {
		assert.Equal(t, []any{}, entry[key], key)
	}
}

func Test_renderRecurringJSON_writes_the_window_as_since_and_until(t *testing.T) {
	doc := recurringDocumentOf(t, report.Recurring{Window: spendingWindow()})

	assert.Equal(t, "2026-01-01", doc["since"])
	assert.Equal(t, "2026-03-09", doc["until"])
}

func Test_renderRecurringJSON_writes_the_warnings_it_is_given(t *testing.T) {
	doc := recurringDocumentOf(t, report.Recurring{Window: spendingWindow()}, "first", "second")

	assert.Equal(t, []any{"first", "second"}, doc["warnings"])
}

func Test_renderRecurringJSON_writes_an_ended_new_series_without_per_year_or_a_total(t *testing.T) {
	r := report.Recurring{Window: spendingWindow(), Series: []report.Series{{
		Payee: "Disney Plus", Currency: "CAD", Cadence: report.CadenceMonthly, Amount: 1199, FirstAmount: 1199,
		First: recurringDay(time.February, 7), Last: recurringDay(time.April, 7), ChargeCount: 3,
		State: report.SeriesEnded, New: true,
	}}}

	doc := recurringDocumentOf(t, r)

	entry := seriesEntryAt(t, doc, 0)
	assert.Equal(t, "ended", entry["state"])
	assert.Nil(t, entry["per_year"])
	assert.Contains(t, entry, "per_year")
	assert.Equal(t, true, entry["new"])
	assert.Equal(t, []any{}, doc["totals"])
}

func Test_renderRecurringJSON_writes_a_null_payee_key_for_the_payee_id_fallback(t *testing.T) {
	entry := firstSeriesOf(t, report.Series{Payee: "#4411", Currency: "CAD", Cadence: report.CadenceMonthly})

	assert.Contains(t, entry, "payee_key")
	assert.Nil(t, entry["payee_key"])
}

func Test_renderRecurringJSON_writes_the_payee_key_of_a_named_series(t *testing.T) {
	entry := firstSeriesOf(t, report.Series{
		Payee: "Netflix.com", PayeeKey: new("netflix-com"), Currency: "CAD", Cadence: report.CadenceMonthly,
	})

	assert.Equal(t, "netflix-com", entry["payee_key"])
}

func Test_renderRecurringJSON_names_each_cadence(t *testing.T) {
	cases := []struct {
		cadence report.Cadence
		want    string
	}{
		{cadence: report.CadenceWeekly, want: "weekly"},
		{cadence: report.CadenceMonthly, want: "monthly"},
		{cadence: report.CadenceQuarterly, want: "quarterly"},
		{cadence: report.CadenceAnnual, want: "annual"},
	}

	for _, c := range cases {
		t.Run(c.want, func(t *testing.T) {
			entry := firstSeriesOf(t, report.Series{Payee: "A", Currency: "CAD", Cadence: c.cadence})

			assert.Equal(t, c.want, entry["cadence"])
		})
	}
}

func Test_renderRecurringJSON_writes_the_series_values_read_back(t *testing.T) {
	s := report.Series{
		Payee: "Netflix.com", PayeeKey: new("netflix-com"), Currency: "CAD", Cadence: report.CadenceMonthly,
		Amount: 1099, FirstAmount: 999, PerYear: new(int64(13188)),
		First: recurringDay(time.February, 12), Last: recurringDay(time.September, 12), ChargeCount: 24,
		State: report.SeriesActive, New: true,
		Payees:   []report.SeriesPayee{{ID: "payee-1", Name: "NETFLIX.COM 1234"}, {ID: "payee-2", Name: "Netflix.com"}},
		Accounts: []store.Account{{ID: "acct-1", Name: "Chequing"}, {ID: "acct-2", Name: "Visa"}},
		PriceChanges: []report.PriceChange{
			{Date: recurringDay(time.June, 12), From: 999, To: 1199, Tenths: 200},
			{Date: recurringDay(time.August, 12), From: 1199, To: 1099, Tenths: -83},
		},
	}

	entry := firstSeriesOf(t, s)

	assert.Equal(t, map[string]any{
		"payee": "Netflix.com", "payee_key": "netflix-com", "currency": "CAD", "cadence": "monthly",
		"amount": "10.99", "first_amount": "9.99", "per_year": "131.88",
		"first_charge": "2026-02-12", "last_charge": "2026-09-12", "charge_count": float64(24),
		"state": "active", "new": true,
		"payees": []any{
			map[string]any{"id": "payee-1", "name": "NETFLIX.COM 1234"},
			map[string]any{"id": "payee-2", "name": "Netflix.com"},
		},
		"accounts": []any{
			map[string]any{"id": "acct-1", "name": "Chequing"},
			map[string]any{"id": "acct-2", "name": "Visa"},
		},
		"price_changes": []any{
			map[string]any{"date": "2026-06-12", "from": "9.99", "to": "11.99", "change_pct": 20.0},
			map[string]any{"date": "2026-08-12", "from": "11.99", "to": "10.99", "change_pct": -8.3},
		},
	}, entry)
}

func Test_renderRecurringJSON_keeps_the_series_and_currency_totals_in_the_order_given(t *testing.T) {
	r := report.Recurring{
		Window: spendingWindow(),
		Series: []report.Series{
			{Payee: "Zed", Currency: "USD", Cadence: report.CadenceMonthly},
			{Payee: "Amy", Currency: "CAD", Cadence: report.CadenceMonthly},
		},
		Totals: []report.RecurringTotal{{Currency: "USD", PerYear: 1000}, {Currency: "CAD", PerYear: 123456}},
	}

	doc := recurringDocumentOf(t, r)

	assert.Equal(t, "Zed", seriesEntryAt(t, doc, 0)["payee"])
	assert.Equal(t, "Amy", seriesEntryAt(t, doc, 1)["payee"])
	assert.Equal(t, []any{
		map[string]any{"currency": "USD", "per_year": "10.00"},
		map[string]any{"currency": "CAD", "per_year": "1234.56"},
	}, doc["totals"])
}
