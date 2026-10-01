package cli_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// anomalyCharge is a charge of cents on date; the payee, category and split count are the test's to set.
func anomalyCharge(id string, srcID int64, date time.Time, payee *string, cents int64) store.Charge {
	return store.Charge{
		TransactionID: id, SourceID: srcID, Date: date,
		Account: store.Account{ID: "acct-1", Name: "Chequing", Currency: "CAD"},
		PayeeID: new("payee-" + id), Payee: payee, Currency: "CAD", Amount: cents, ExpenseSplits: 1,
	}
}

// payeeHistory is three charges of the payee in 2025, then a big one on bigDay; the big charge is the last row.
func payeeHistory(payee string, bigDay time.Time, bigCents int64, history ...int64) []store.Charge {
	rows := make([]store.Charge, 0, len(history)+1)
	for i, cents := range history {
		id := fmt.Sprintf("%s-%d", payee, i)
		c := anomalyCharge(id, int64(i+1), time.Date(2025, time.March, 3+i, 0, 0, 0, 0, time.UTC), &payee, cents)
		c.PayeeID = new("payee-" + payee)
		rows = append(rows, c)
	}
	big := anomalyCharge(payee+"-big", 100, bigDay, &payee, bigCents)
	big.PayeeID = new("payee-" + payee)
	return append(rows, big)
}

// rawAnomaliesDocument is stdout decoded as untyped JSON, so an absent key differs from a null one.
func rawAnomaliesDocument(t *testing.T, stdout string) map[string]any {
	t.Helper()
	var raw map[string]any

	require.NoError(t, json.Unmarshal([]byte(stdout), &raw), stdout)
	return raw
}

// listedAnomalies is the "anomalies" array of stdout, each entry untyped.
func listedAnomalies(t *testing.T, stdout string) []map[string]any {
	t.Helper()
	entries, ok := rawAnomaliesDocument(t, stdout)["anomalies"].([]any)
	require.True(t, ok, stdout)
	listed := make([]map[string]any, len(entries))
	for i, entry := range entries {
		listed[i], ok = entry.(map[string]any)
		require.True(t, ok, stdout)
	}
	return listed
}

func Test_anomalies_json_prints_every_ruled_key_and_no_table(t *testing.T) {
	var stdout, stderr bytes.Buffer
	rows := payeeHistory("Bell Canada", time.Date(2026, time.March, 2, 0, 0, 0, 0, time.UTC), 41200, 9000, 9300, 9605)
	fake := fakeReportStore{charges: store.Charges{Rows: rows}}

	err := executeAnomalies(t, fake, &stdout, &stderr, "--json")

	require.NoError(t, err)
	assert.Empty(t, stderr.String())
	raw := rawAnomaliesDocument(t, stdout.String())
	assert.ElementsMatch(t, []string{"since", "until", "account_filter", "anomalies", "checked", "not_judged", "warnings"}, keysOf(raw))
	entry := listedAnomalies(t, stdout.String())[0]
	assert.ElementsMatch(t, []string{
		"transaction_id", "date", "account_id", "account", "currency", "payee", "category",
		"amount", "baseline", "usual", "earlier", "times",
	}, keysOf(entry))
}

func Test_anomalies_json_holds_empty_arrays_rather_than_null_when_nothing_is_listed(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := executeAnomalies(t, fakeReportStore{}, &stdout, &stderr, "--json")

	require.NoError(t, err)
	raw := rawAnomaliesDocument(t, stdout.String())
	assert.Equal(t, []any{}, raw["anomalies"])
	assert.Equal(t, []any{}, raw["account_filter"])
	assert.Equal(t, []any{}, raw["warnings"])
	assert.InDelta(t, 0, raw["checked"], 0)
}

func Test_anomalies_json_prints_the_amounts_as_two_decimal_strings_and_times_as_a_number(t *testing.T) {
	var stdout, stderr bytes.Buffer
	rows := payeeHistory("Bell Canada", time.Date(2026, time.March, 2, 0, 0, 0, 0, time.UTC), 41200, 9000, 9300, 9605)
	fake := fakeReportStore{charges: store.Charges{Rows: rows}}

	err := executeAnomalies(t, fake, &stdout, &stderr, "--json")

	require.NoError(t, err)
	entry := listedAnomalies(t, stdout.String())[0]
	assert.Equal(t, "412.00", entry["amount"])
	assert.Equal(t, "93.00", entry["usual"])
	assert.InDelta(t, 4.4, entry["times"], 0)
	assert.InDelta(t, 3, entry["earlier"], 0)
	assert.Equal(t, "payee", entry["baseline"])
}

func Test_anomalies_json_prints_a_null_category_for_uncategorized_and_split_charges(t *testing.T) {
	var stdout, stderr bytes.Buffer
	uncategorized := payeeHistory("Bell Canada", time.Date(2026, time.March, 2, 0, 0, 0, 0, time.UTC), 41200, 9000, 9300, 9605)
	split := payeeHistory("Hydro", time.Date(2026, time.April, 2, 0, 0, 0, 0, time.UTC), 30000, 10000, 10000, 10000)
	split[len(split)-1].ExpenseSplits = 2
	sameCategoryTwice := payeeHistory("Gym", time.Date(2026, time.May, 2, 0, 0, 0, 0, time.UTC), 30000, 10000, 10000, 10000)
	sameCategoryTwice[len(sameCategoryTwice)-1].ExpenseSplits = 2
	sameCategoryTwice[len(sameCategoryTwice)-1].Category = &store.ChargeCategory{ID: "cat-1", Path: "Fitness"}
	fake := fakeReportStore{charges: store.Charges{Rows: append(append(uncategorized, split...), sameCategoryTwice...)}}

	err := executeAnomalies(t, fake, &stdout, &stderr, "--json")

	require.NoError(t, err)
	listed := listedAnomalies(t, stdout.String())
	require.Len(t, listed, 3)
	for _, entry := range listed {
		category, present := entry["category"]
		assert.True(t, present)
		assert.Nil(t, category)
	}
}

func Test_anomalies_json_prints_the_category_path_of_a_single_category_charge(t *testing.T) {
	var stdout, stderr bytes.Buffer
	rows := payeeHistory("Bell Canada", time.Date(2026, time.March, 2, 0, 0, 0, 0, time.UTC), 41200, 9000, 9300, 9605)
	rows[len(rows)-1].Category = &store.ChargeCategory{ID: "cat-1", Path: "Utilities:Phone"}
	fake := fakeReportStore{charges: store.Charges{Rows: rows}}

	err := executeAnomalies(t, fake, &stdout, &stderr, "--json")

	require.NoError(t, err)
	entry := listedAnomalies(t, stdout.String())[0]
	assert.Equal(t, "Utilities:Phone", entry["category"])
}

func Test_anomalies_json_prints_a_null_payee_for_a_charge_judged_against_its_category(t *testing.T) {
	var stdout, stderr bytes.Buffer
	category := &store.ChargeCategory{ID: "cat-1", Path: "Home:Repairs"}
	rows := make([]store.Charge, 0, 11)
	for i := range 10 {
		c := anomalyCharge(fmt.Sprintf("vendor-%d", i), int64(i+1), time.Date(2025, time.March, 3+i, 0, 0, 0, 0, time.UTC), new(fmt.Sprintf("Vendor %d", i)), 20000)
		c.Category = category
		rows = append(rows, c)
	}
	noPayee := anomalyCharge("no-payee", 100, time.Date(2026, time.August, 14, 0, 0, 0, 0, time.UTC), nil, 184210)
	noPayee.PayeeID, noPayee.Category = nil, category
	rows = append(rows, noPayee)
	fake := fakeReportStore{charges: store.Charges{Rows: rows}}

	err := executeAnomalies(t, fake, &stdout, &stderr, "--json")

	require.NoError(t, err)
	entry := listedAnomalies(t, stdout.String())[0]
	payee, present := entry["payee"]
	assert.True(t, present)
	assert.Nil(t, payee)
	assert.Equal(t, "category", entry["baseline"])
	assert.Equal(t, "Home:Repairs", entry["category"])
	assert.InDelta(t, 10, entry["earlier"], 0)
	assert.InDelta(t, 9.2, entry["times"], 0)
}

func keysOf(m map[string]any) []string { return slices.Collect(maps.Keys(m)) }
