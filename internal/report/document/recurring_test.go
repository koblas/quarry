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

// recurringDocumentOf renders r with warnings and reads the document back as generic JSON.
func recurringDocumentOf(tb testing.TB, r report.Recurring, warnings ...string) map[string]any {
	tb.Helper()
	out := indented(tb, document.NewRecurring(r, warnings))
	var doc map[string]any
	require.NoError(tb, json.Unmarshal([]byte(out), &doc), out)
	return doc
}

// seriesEntryAt is the i-th entry of doc's "series".
func seriesEntryAt(tb testing.TB, doc map[string]any, i int) map[string]any {
	tb.Helper()
	entries, ok := doc["series"].([]any)
	require.True(tb, ok)
	require.Greater(tb, len(entries), i)
	entry, ok := entries[i].(map[string]any)
	require.True(tb, ok)
	return entry
}

// firstSeriesOf renders a document of the one series s and returns its entry.
func firstSeriesOf(tb testing.TB, s report.Series) map[string]any {
	tb.Helper()
	return seriesEntryAt(tb, recurringDocumentOf(tb, report.Recurring{Window: window, Series: []report.Series{s}}), 0)
}

func Test_NewRecurring_writes_empty_arrays_not_null_for_a_report_with_no_series(t *testing.T) {
	doc := recurringDocumentOf(t, report.Recurring{Window: window})

	for _, key := range []string{"account_filter", "series", "totals", "warnings"} {
		assert.Equal(t, []any{}, doc[key], key)
	}
}

func Test_NewRecurring_writes_empty_arrays_not_null_for_a_series_with_no_lists(t *testing.T) {
	entry := firstSeriesOf(t, report.Series{Payee: "A", Currency: "CAD", Cadence: report.CadenceMonthly})

	for _, key := range []string{"payees", "accounts", "price_changes"} {
		assert.Equal(t, []any{}, entry[key], key)
	}
}

func Test_NewRecurring_writes_the_window_as_since_and_until(t *testing.T) {
	doc := recurringDocumentOf(t, report.Recurring{Window: window})

	assert.Equal(t, "2026-01-01", doc["since"])
	assert.Equal(t, "2026-09-29", doc["until"])
}

func Test_NewRecurring_writes_the_warnings_it_is_given(t *testing.T) {
	doc := recurringDocumentOf(t, report.Recurring{Window: window}, "first", "second")

	assert.Equal(t, []any{"first", "second"}, doc["warnings"])
}

func Test_NewRecurring_writes_an_ended_new_series_with_a_null_per_year(t *testing.T) {
	r := report.Recurring{Window: window, Series: []report.Series{{
		Payee: "Disney Plus", Currency: "CAD", Cadence: report.CadenceMonthly, Amount: 1199, FirstAmount: 1199,
		First: day(time.February, 7), Last: day(time.April, 7), ChargeCount: 3,
		State: report.SeriesEnded, New: true,
	}}}

	entry := seriesEntryAt(t, recurringDocumentOf(t, r), 0)

	assert.Equal(t, "ended", entry["state"])
	assert.Nil(t, entry["per_year"])
	assert.Contains(t, entry, "per_year")
	assert.Equal(t, true, entry["new"])
}

func Test_NewRecurring_writes_a_null_payee_key_for_the_payee_id_fallback(t *testing.T) {
	entry := firstSeriesOf(t, report.Series{Payee: "#4411", Currency: "CAD", Cadence: report.CadenceMonthly})

	assert.Contains(t, entry, "payee_key")
	assert.Nil(t, entry["payee_key"])
}

func Test_NewRecurring_writes_the_payee_key_of_a_named_series(t *testing.T) {
	entry := firstSeriesOf(t, report.Series{
		Payee: "Netflix.com", PayeeKey: new("netflix-com"), Currency: "CAD", Cadence: report.CadenceMonthly,
	})

	assert.Equal(t, "netflix-com", entry["payee_key"])
}

func Test_NewRecurring_names_each_cadence(t *testing.T) {
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

func Test_NewRecurring_writes_the_series_values_read_back(t *testing.T) {
	s := report.Series{
		Payee: "Netflix.com", PayeeKey: new("netflix-com"), Currency: "CAD", Cadence: report.CadenceMonthly,
		Amount: 1099, FirstAmount: 999, PerYear: new(int64(13188)),
		NativeCurrency: "CAD", NativeAmount: 1099, NativeFirstAmount: 999,
		First: day(time.February, 12), Last: day(time.September, 12), ChargeCount: 24,
		State: report.SeriesActive, New: true,
		Payees:   []report.SeriesPayee{{ID: "payee-1", Name: "NETFLIX.COM 1234"}, {ID: "payee-2", Name: "Netflix.com"}},
		Accounts: []store.Account{{ID: "acct-1", Name: "Chequing"}, {ID: "acct-2", Name: "Visa"}},
		PriceChanges: []report.PriceChange{
			{Date: day(time.June, 12), From: 999, To: 1199, Tenths: 200},
			{Date: day(time.August, 12), From: 1199, To: 1099, Tenths: -83},
		},
	}

	entry := firstSeriesOf(t, s)

	assert.Equal(t, map[string]any{
		"payee": "Netflix.com", "payee_key": "netflix-com", "currency": "CAD", "cadence": "monthly",
		"amount": "10.99", "first_amount": "9.99", "per_year": "131.88",
		"native_currency": "CAD", "native_amount": "10.99", "native_first_amount": "9.99",
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
			map[string]any{"date": "2026-06-12", "currency": "CAD", "from": "9.99", "to": "11.99", "change_pct": 20.0},
			map[string]any{"date": "2026-08-12", "currency": "CAD", "from": "11.99", "to": "10.99", "change_pct": -8.3},
		},
	}, entry)
}

func Test_NewRecurring_keeps_the_series_and_currency_totals_in_the_order_given(t *testing.T) {
	r := report.Recurring{
		Window: window,
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

func Test_NewRecurring_writes_the_id_and_name_of_each_named_account_in_account_filter(t *testing.T) {
	r := report.Recurring{
		Window:   window,
		Accounts: []store.Account{{ID: "acct-2", Name: "Visa"}, {ID: "acct-1", Name: "Chequing"}},
	}

	doc := recurringDocumentOf(t, r)

	assert.Equal(t, []any{
		map[string]any{"id": "acct-2", "name": "Visa"},
		map[string]any{"id": "acct-1", "name": "Chequing"},
	}, doc["account_filter"])
}

// convertedUSDSeries is a USD series listed in CAD: 15.00 USD now (12.00 at the first charge), 21.00 and 16.80 CAD.
func convertedUSDSeries() report.Series {
	return report.Series{
		Payee: "Gym", PayeeKey: new("gym"), Currency: "CAD", Cadence: report.CadenceMonthly,
		Amount: 2100, FirstAmount: 1560, PerYear: new(int64(25200)),
		NativeCurrency: "USD", NativeAmount: 1500, NativeFirstAmount: 1200,
		First: day(time.February, 12), Last: day(time.September, 12), ChargeCount: 8,
		State: report.SeriesActive, New: true, ChangeTenths: 250,
		Payees:       []report.SeriesPayee{{ID: "payee-1", Name: "Gym"}},
		Accounts:     []store.Account{{ID: "acct-usd", Name: "US Chequing"}},
		PriceChanges: []report.PriceChange{{Date: day(time.June, 12), From: 1200, To: 1500, Tenths: 250}},
	}
}

func Test_NewRecurring_orders_the_document_series_and_price_change_keys(t *testing.T) {
	out := []byte(indented(t, document.NewRecurring(report.Recurring{
		Window: window, Currency: money.CAD, Series: []report.Series{convertedUSDSeries()},
	}, []string{})))
	var doc struct {
		Series []json.RawMessage `json:"series"`
	}
	require.NoError(t, json.Unmarshal(out, &doc))
	require.Len(t, doc.Series, 1)
	var entry struct {
		PriceChanges []json.RawMessage `json:"price_changes"`
	}
	require.NoError(t, json.Unmarshal(doc.Series[0], &entry))
	require.Len(t, entry.PriceChanges, 1)

	assert.Equal(t, []string{"since", "until", "currency", "account_filter", "series", "totals", "warnings"}, topLevelKeys(t, out))
	assert.Equal(t, []string{
		"payee", "payee_key", "payees", "currency", "cadence", "amount", "first_amount", "per_year",
		"native_currency", "native_amount", "native_first_amount", "first_charge", "last_charge",
		"charge_count", "state", "new", "accounts", "price_changes",
	}, topLevelKeys(t, doc.Series[0]))
	assert.Equal(t, []string{"date", "currency", "from", "to", "change_pct"}, topLevelKeys(t, entry.PriceChanges[0]))
}

func Test_NewRecurring_keeps_the_same_keys_in_every_reporting_currency(t *testing.T) {
	cases := []struct {
		name     string
		currency money.Currency
		want     string
	}{
		{name: "CAD", currency: money.CAD, want: "CAD"},
		{name: "USD", currency: money.USD, want: "USD"},
		{name: "native", currency: money.Native, want: "native"},
	}
	_, cadKeys := documentKeysIn(t, money.CAD)

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			currency, keys := documentKeysIn(t, c.currency)

			assert.Equal(t, c.want, currency)
			assert.Equal(t, cadKeys, keys)
		})
	}
}

// documentKeysIn is the "currency" value and the ordered keys of the one series of a document rendered in currency.
func documentKeysIn(tb testing.TB, currency money.Currency) (string, []string) {
	tb.Helper()
	out := []byte(indented(tb, document.NewRecurring(report.Recurring{Window: window, Currency: currency, Series: []report.Series{convertedUSDSeries()}}, []string{})))
	var doc struct {
		Currency string            `json:"currency"`
		Series   []json.RawMessage `json:"series"`
	}
	require.NoError(tb, json.Unmarshal(out, &doc))
	require.Len(tb, doc.Series, 1)
	return doc.Currency, topLevelKeys(tb, doc.Series[0])
}

func Test_NewRecurring_reads_back_converted_native_ended_and_unchanged_series(t *testing.T) {
	ended := report.Series{
		Payee: "Old", Currency: "CAD", Cadence: report.CadenceMonthly, Amount: 1400, FirstAmount: 1300,
		NativeCurrency: "USD", NativeAmount: 1000, NativeFirstAmount: 1000,
		First: day(time.January, 5), Last: day(time.March, 5), ChargeCount: 3, State: report.SeriesEnded,
	}
	plain := report.Series{
		Payee: "Rent", Currency: "CAD", Cadence: report.CadenceMonthly, Amount: 5000, FirstAmount: 5000, PerYear: new(int64(60000)),
		NativeCurrency: "CAD", NativeAmount: 5000, NativeFirstAmount: 5000,
		First: day(time.January, 5), Last: day(time.March, 5), ChargeCount: 3, State: report.SeriesActive,
	}
	out := []byte(indented(t, document.NewRecurring(report.Recurring{
		Window: window, Currency: money.CAD,
		Series: []report.Series{convertedUSDSeries(), ended, plain},
		Totals: []report.RecurringTotal{{Currency: "CAD", PerYear: 85200}},
	}, []string{})))
	var doc struct {
		Currency string `json:"currency"`
		Series   []struct {
			Payee             string  `json:"payee"`
			Currency          string  `json:"currency"`
			Amount            string  `json:"amount"`
			FirstAmount       string  `json:"first_amount"`
			PerYear           *string `json:"per_year"`
			NativeCurrency    string  `json:"native_currency"`
			NativeAmount      string  `json:"native_amount"`
			NativeFirstAmount string  `json:"native_first_amount"`
			PriceChanges      []struct {
				Currency string `json:"currency"`
				From     string `json:"from"`
				To       string `json:"to"`
			} `json:"price_changes"`
		} `json:"series"`
	}
	require.NoError(t, json.Unmarshal(out, &doc), string(out))

	require.Len(t, doc.Series, 3)
	assert.Equal(t, "CAD", doc.Currency)
	gym := doc.Series[0]
	assert.Equal(t, []string{"CAD", "21.00", "15.60", "USD", "15.00", "12.00"},
		[]string{gym.Currency, gym.Amount, gym.FirstAmount, gym.NativeCurrency, gym.NativeAmount, gym.NativeFirstAmount})
	require.NotNil(t, gym.PerYear)
	assert.Equal(t, "252.00", *gym.PerYear)
	assert.Equal(t, "USD", gym.PriceChanges[0].Currency)
	assert.Equal(t, []string{"12.00", "15.00"}, []string{gym.PriceChanges[0].From, gym.PriceChanges[0].To})
	assert.Nil(t, doc.Series[1].PerYear)
	assert.Equal(t, "14.00", doc.Series[1].Amount)
	assert.Equal(t, "10.00", doc.Series[1].NativeAmount)
	assert.Equal(t, "CAD", doc.Series[2].NativeCurrency)
	assert.Equal(t, doc.Series[2].Amount, doc.Series[2].NativeAmount)
	assert.NotNil(t, doc.Series[2].PriceChanges)
	assert.Empty(t, doc.Series[2].PriceChanges)
	assert.Contains(t, string(out), `"price_changes": []`)
}

func Test_NewRecurring_copies_the_warnings_and_turns_nil_into_an_empty_list(t *testing.T) {
	given := []string{"first"}

	got := document.NewRecurring(report.Recurring{Window: window}, given)
	given[0] = "changed"

	assert.Equal(t, []string{"first"}, got.Warnings)
	assert.Equal(t, []string{}, document.NewRecurring(report.Recurring{Window: window}, nil).Warnings)
}

func Test_RecurringStatus_names_each_state_and_nothing_for_an_unknown_one(t *testing.T) {
	assert.Equal(t, "active", document.RecurringStatus(report.SeriesActive))
	assert.Equal(t, "ended", document.RecurringStatus(report.SeriesEnded))
	assert.Empty(t, document.RecurringStatus(report.SeriesState(99)))
}
