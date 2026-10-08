package cli_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/cli"
	"github.com/koblas/quarry/internal/config"
	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func executeAnomalies(t *testing.T, fake fakeReportStore, stdout, stderr io.Writer, args ...string) error {
	t.Helper()
	env := reportEnv(fake, stdout, stderr, atSpendNow)
	return cli.Execute(t.Context(), append([]string{"anomalies"}, args...), env)
}

func Test_anomalies_refuses_a_window_before_opening_the_report(t *testing.T) {
	var stdout, stderr bytes.Buffer
	env := failingReportEnv(errStoreRead, &stdout, &stderr, atWallClock)

	err := cli.Execute(t.Context(), []string{"anomalies", "--since", "2024-13"}, env)

	var usage cli.UsageError
	require.ErrorAs(t, err, &usage)
	require.EqualError(t, err, `--since "2024-13" is not a date; use YYYY, YYYY-MM or YYYY-MM-DD`)
}

func Test_anomalies_takes_no_arguments(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := executeAnomalies(t, fakeReportStore{}, &stdout, &stderr, "extra")

	var usage cli.UsageError
	require.ErrorAs(t, err, &usage)
	require.EqualError(t, err, "anomalies takes no arguments")
}

func Test_anomalies_help_says_what_anomalies_lists(t *testing.T) {
	const long = `List charges that are unusually large: more than 2 times the median of the
payee's earlier charges, when there are at least 3, or else more than 5
times the median of the category's earlier charges, when there are at
least 10. Charges under 100.00 in their account's own currency are never
listed. Charges follow the rules of quarry spend, and a transaction counts
once, with all its splits; an uncategorized or split charge from a payee
with little history cannot be judged. Possible duplicates are listed by
quarry findings, not here. Charges dated after today are left out, even
with a later --until.

Charges are judged in their account's own currency, so a change in the
exchange rate never makes a charge unusual. Amount and Usual are then
shown in the reporting currency (--currency, else reporting.currency in
the config file, else CAD) at the rate on the charge's date.
With --currency native nothing is converted.

--since and --until choose which charges to list; each is compared with
every earlier charge, however old. --account lists only charges in those
accounts; the payee's charges in other accounts still count as history.
`
	var stdout, stderr bytes.Buffer

	err := executeAnomalies(t, fakeReportStore{}, &stdout, &stderr, "--help")

	require.NoError(t, err)
	assert.Contains(t, stdout.String(), long)
}

func Test_anomalies_without_charges_prints_the_empty_table_and_footer_and_names_the_stores_span(t *testing.T) {
	var stdout, stderr bytes.Buffer
	fake := fakeReportStore{charges: store.Charges{Transactions: span(t, "2003-01-04", "2026-09-26")}}

	err := executeAnomalies(t, fake, &stdout, &stderr)

	require.NoError(t, err)
	assert.Equal(t, ""+
		"Unusually large charges 2026-01-01 to 2026-09-29 in all accounts, amounts in CAD\n\n"+
		"Date  Account  Payee  Category  Amount  Usual  Times  Compared with\n\n"+
		"0 charges checked\n", stdout.String())
	assert.Equal(t, "quarry: warning: "+anomaliesEmpty+"; the store's transactions run 2003-01-04 to 2026-09-26\n", stderr.String())
}

func Test_anomalies_charges_exist_but_none_unusual_prints_no_warning(t *testing.T) {
	var stdout, stderr bytes.Buffer
	fake := fakeReportStore{charges: store.Charges{Rows: ordinaryCharge()}}

	err := executeAnomalies(t, fake, &stdout, &stderr)

	require.NoError(t, err)
	assert.Empty(t, stderr.String())
	assert.True(t, strings.HasSuffix(stdout.String(), "\n1 charge checked\n"), stdout.String())
}

func Test_anomalies_reports_a_failed_stdout_write(t *testing.T) {
	err := executeAnomalies(t, fakeReportStore{}, failingWriter{err: errNoSpace}, io.Discard)

	require.EqualError(t, err, "cannot write the result to stdout: write /dev/stdout: no space left on device")
	assert.ErrorIs(t, err, errNoSpace)
}

const anomaliesEmpty = "no unusually large charges from 2026-01-01 to 2026-09-29"

// ordinaryCharge is three 2025 charges of a payee and a 100.00 one on 2026-03-02, not above twice their median.
func ordinaryCharge() []store.Charge {
	return payeeHistory("Bell Canada", time.Date(2026, time.March, 2, 0, 0, 0, 0, time.UTC), 10000, 9000, 9300, 9605)
}

// onAccount is rows moved to the Chequing account with id.
func onAccount(rows []store.Charge, id string) []store.Charge {
	moved := make([]store.Charge, len(rows))
	for i, r := range rows {
		r.Account = store.Account{ID: id, Name: "Chequing", Currency: "CAD"}
		moved[i] = r
	}
	return moved
}

func Test_anomalies_captions_the_named_accounts_and_passes_their_ids_to_the_report(t *testing.T) {
	var got store.ChargeParams
	var stdout, stderr bytes.Buffer
	fake := namedAccounts()
	fake.charges = store.Charges{Rows: onAccount(ordinaryCharge(), visaID)}
	fake.gotCharges = &got

	err := executeAnomalies(t, fake, &stdout, &stderr, "--account", "visa infinite", "--account", chequingID)

	require.NoError(t, err)
	assert.Equal(t, []string{visaID, chequingID}, got.AccountIDs)
	assert.Contains(t, stdout.String(), "Unusually large charges 2026-01-01 to 2026-09-29 in Visa Infinite, Chequing, amounts in CAD\n\n")
}

func Test_anomalies_escapes_a_line_break_in_a_named_accounts_caption(t *testing.T) {
	var stdout, stderr bytes.Buffer
	fake := accountsStore(store.Account{ID: chequingID, Name: "Chq\nOne"})

	err := executeAnomalies(t, fake, &stdout, &stderr, "--account", chequingID)

	require.NoError(t, err)
	assert.Contains(t, stdout.String(), `in Chq\nOne, amounts in CAD`+"\n\n")
}

func Test_anomalies_json_names_the_accounts_it_was_limited_to(t *testing.T) {
	var stdout, stderr bytes.Buffer
	fake := namedAccounts()
	fake.charges = store.Charges{Rows: onAccount(ordinaryCharge(), chequingID)}

	err := executeAnomalies(t, fake, &stdout, &stderr, "--json", "--account", "chequing")

	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "\"account_filter\": [\n    {\n      \"id\": \""+chequingID+"\",\n      \"name\": \"Chequing\"\n    }\n  ],\n")
}

func Test_anomalies_prints_each_warning_on_stderr_and_in_the_json_warnings(t *testing.T) {
	inChequing := store.Charges{Rows: onAccount(ordinaryCharge(), chequingID)}
	cases := []struct {
		name    string
		charges store.Charges
		args    []string
		want    []string
	}{
		{
			name:    "an account not in reports and one under linked tracking, each once in the order named",
			charges: inChequing,
			args:    []string{"--account", linkedID, "--account", "Old Card", "--account", chequingID, "--account", bothID, "--account", "old card"},
			want:    []string{linkedTrackingWarning("anomalies", "Netskope 401(k)"), leftOutWarning("anomalies", "Old Card"), linkedTrackingWarning("anomalies", "Old 401(k)")},
		},
		{
			name:    "the store's span when no charge falls in the period",
			charges: store.Charges{Transactions: span(t, "2003-01-04", "2026-09-26")},
			want:    []string{anomaliesEmpty + "; the store's transactions run 2003-01-04 to 2026-09-26"},
		},
		{
			name: "an empty store",
			want: []string{anomaliesEmpty + "; the store has no transactions"},
		},
		{
			name:    "the named accounts' span when none of their charges falls in the period",
			charges: store.Charges{Transactions: span(t, "2019-03-02", "2024-11-30")},
			args:    []string{"--account", chequingID},
			want:    []string{anomaliesEmpty + " in the named accounts; their transactions run 2019-03-02 to 2024-11-30"},
		},
		{
			name: "named accounts with no transactions",
			args: []string{"--account", chequingID},
			want: []string{anomaliesEmpty + " in the named accounts; they have no transactions"},
		},
		{
			name:    "charges that fall only in other accounts than the one named",
			charges: store.Charges{Rows: onAccount(ordinaryCharge(), visaID)},
			args:    []string{"--account", chequingID},
			want:    []string{anomaliesEmpty + " in the named accounts; they have no transactions"},
		},
		{
			name:    "the left-out warnings before the empty note when a reported account is also named",
			charges: store.Charges{Transactions: span(t, "2019-03-02", "2024-11-30")},
			args:    []string{"--account", "Old Card", "--account", chequingID},
			want: []string{
				leftOutWarning("anomalies", "Old Card"),
				anomaliesEmpty + " in the named accounts; their transactions run 2019-03-02 to 2024-11-30",
			},
		},
		{
			name:    "only the left-out warnings when every named account is left out",
			charges: store.Charges{Transactions: span(t, "2019-03-02", "2024-11-30")},
			args:    []string{"--account", linkedID, "--account", "Old Card"},
			want:    []string{linkedTrackingWarning("anomalies", "Netskope 401(k)"), leftOutWarning("anomalies", "Old Card")},
		},
		{
			name:    "only the left-out warning when every named account is left out and the store is empty",
			charges: store.Charges{},
			args:    []string{"--account", oldBankID},
			want:    []string{leftOutWarning("anomalies", "Old Bank")},
		},
		{
			name:    "no note when charges were checked but none is unusual",
			charges: inChequing,
			args:    []string{"--account", chequingID},
			want:    []string{},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			fake := namedAccounts()
			fake.charges = c.charges

			err := executeAnomalies(t, fake, &stdout, &stderr, append([]string{"--json"}, c.args...)...)

			require.NoError(t, err)
			var doc struct {
				Warnings []string `json:"warnings"`
			}
			require.NoError(t, json.Unmarshal(stdout.Bytes(), &doc), stdout.String())
			assert.Equal(t, c.want, doc.Warnings)
			assert.Equal(t, warningLines(c.want), stderr.String())
		})
	}
}

func Test_anomalies_reads_the_charges_once_and_takes_the_empty_note_from_that_read(t *testing.T) {
	var reads int
	var stdout, stderr bytes.Buffer
	fake := fakeReportStore{charges: store.Charges{Transactions: span(t, "2003-01-04", "2026-09-26")}, chargesReads: &reads}

	err := executeAnomalies(t, fake, &stdout, &stderr)

	require.NoError(t, err)
	assert.Equal(t, 1, reads)
	assert.Equal(t, "quarry: warning: "+anomaliesEmpty+"; the store's transactions run 2003-01-04 to 2026-09-26\n", stderr.String())
}

// executeAnomaliesIn runs anomalies over fake with a config whose reporting.currency is cfg.
func executeAnomaliesIn(t *testing.T, cfg money.Currency, fake fakeReportStore, stdout, stderr *bytes.Buffer, args ...string) error {
	t.Helper()
	env := reportEnv(fake, stdout, stderr, atSpendNow, withConfig(config.Config{Currency: cfg}))
	return cli.Execute(t.Context(), append([]string{"anomalies"}, args...), env)
}

// hardwareCharges is a CAD Hardware payee whose 500.00 charge on 2026-03-02 is 5 times its 100.00 history,
// every charge carrying the USD cell 375.00 / 75.00 (rate 1.3333).
func hardwareCharges(firstRate time.Time) store.Charges {
	rows := payeeHistory("Hardware", time.Date(2026, time.March, 2, 0, 0, 0, 0, time.UTC), 50000, 10000, 10000, 10000)
	for i := range rows {
		rows[i].USDCAD = money.Rate(1_333_333)
		rows[i].AmountUSD = new(int64(7500))
	}
	rows[len(rows)-1].AmountUSD = new(int64(37500))
	return store.Charges{Rows: onAccount(rows, chequingID), FirstRate: firstRate}
}

func Test_anomalies_lists_amounts_in_the_currency_the_config_names(t *testing.T) {
	var stdout, stderr bytes.Buffer
	rated := hardwareCharges(time.Date(2026, time.January, 2, 0, 0, 0, 0, time.UTC))

	err := executeAnomaliesIn(t, money.USD, fakeReportStore{charges: rated}, &stdout, &stderr)

	require.NoError(t, err)
	assert.Empty(t, stderr.String())
	assert.Contains(t, stdout.String(), "Unusually large charges 2026-01-01 to 2026-09-29 in all accounts, amounts in USD\n\n")
	assert.Contains(t, stdout.String(), "375.00  75.00")
}

func Test_anomalies_flag_beats_the_config_and_native_converts_nothing(t *testing.T) {
	var stdout, stderr bytes.Buffer
	rated := hardwareCharges(time.Date(2026, time.January, 2, 0, 0, 0, 0, time.UTC))

	err := executeAnomaliesIn(t, money.USD, fakeReportStore{charges: rated}, &stdout, &stderr, "--currency", "native")

	require.NoError(t, err)
	assert.Empty(t, stderr.String())
	assert.Contains(t, stdout.String(), "in all accounts\n\n")
	assert.NotContains(t, stdout.String(), "amounts in")
	assert.Contains(t, stdout.String(), "500.00  100.00")
}

func Test_anomalies_warns_of_a_charge_dated_before_the_first_rate_in_the_config_currency(t *testing.T) {
	var stdout, stderr bytes.Buffer
	early := hardwareCharges(time.Date(2026, time.April, 1, 0, 0, 0, 0, time.UTC))
	for i := range early.Rows {
		early.Rows[i].AmountUSD, early.Rows[i].USDCAD = nil, 0
	}

	err := executeAnomaliesIn(t, money.USD, fakeReportStore{charges: early}, &stdout, &stderr)

	require.NoError(t, err)
	assert.Equal(t, "quarry: warning: 1 charge dated before 2026-04-01, the first exchange rate in the store, "+
		"is listed in CAD, not converted to USD\n", stderr.String())
	assert.Contains(t, stdout.String(), "CAD 500.00  CAD 100.00")
}

func Test_anomalies_prints_the_left_out_account_warning_before_the_unconverted_charge_warning(t *testing.T) {
	early := hardwareCharges(time.Date(2026, time.April, 1, 0, 0, 0, 0, time.UTC))
	for i := range early.Rows {
		early.Rows[i].AmountUSD, early.Rows[i].USDCAD = nil, 0
	}
	want := []string{
		leftOutWarning("anomalies", "Old Card"),
		"1 charge dated before 2026-04-01, the first exchange rate in the store, is listed in CAD, not converted to USD",
	}
	args := []string{"--account", "Old Card", "--account", chequingID}

	t.Run("text prints them on stderr in that order", func(t *testing.T) {
		var stdout, stderr bytes.Buffer

		err := executeAnomaliesIn(t, money.USD, withCharges(namedAccounts(), early), &stdout, &stderr, args...)

		require.NoError(t, err)
		assert.Equal(t, warningLines(want), stderr.String())
	})

	t.Run("json lists them in that order in warnings", func(t *testing.T) {
		var stdout, stderr bytes.Buffer

		err := executeAnomaliesIn(t, money.USD, withCharges(namedAccounts(), early), &stdout, &stderr, append([]string{"--json"}, args...)...)

		require.NoError(t, err)
		var doc struct {
			Warnings []string `json:"warnings"`
		}
		require.NoError(t, json.Unmarshal(stdout.Bytes(), &doc), stdout.String())
		assert.Equal(t, want, doc.Warnings)
	})
}

func Test_anomalies_says_the_store_has_no_rates_when_a_charge_needs_one(t *testing.T) {
	var stdout, stderr bytes.Buffer
	unrated := hardwareCharges(time.Time{})
	for i := range unrated.Rows {
		unrated.Rows[i].AmountUSD, unrated.Rows[i].USDCAD = nil, 0
	}

	err := executeAnomaliesIn(t, money.USD, fakeReportStore{charges: unrated}, &stdout, &stderr)

	require.NoError(t, err)
	assert.Equal(t, "quarry: warning: the store has no exchange rates, so amounts are listed in each account's own currency; "+
		"run quarry sync to fetch them\n", stderr.String())
}

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
	assert.ElementsMatch(t, []string{"since", "until", "currency", "account_filter", "anomalies", "checked", "not_judged", "warnings"}, keysOf(raw))
	entry := listedAnomalies(t, stdout.String())[0]
	assert.ElementsMatch(t, []string{
		"transaction_id", "date", "account_id", "account", "currency", "payee", "category",
		"amount", "baseline", "usual", "native_currency", "native_amount", "native_usual", "earlier", "times",
	}, keysOf(entry))
}

func Test_anomalies_json_holds_empty_arrays_rather_than_null_when_nothing_is_listed(t *testing.T) {
	var stdout, stderr bytes.Buffer
	fake := fakeReportStore{charges: store.Charges{Rows: ordinaryCharge()}}

	err := executeAnomalies(t, fake, &stdout, &stderr, "--json")

	require.NoError(t, err)
	raw := rawAnomaliesDocument(t, stdout.String())
	assert.Equal(t, []any{}, raw["anomalies"])
	assert.Equal(t, []any{}, raw["account_filter"])
	assert.Equal(t, []any{}, raw["warnings"])
	assert.InDelta(t, 1, raw["checked"], 0)
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
