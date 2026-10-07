package cli_test

import (
	"bytes"
	"encoding/json"
	"io"
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

func executeRecurring(t *testing.T, fake fakeReportStore, stdout, stderr io.Writer, args ...string) error {
	t.Helper()
	env := reportEnv(fake, stdout, stderr, atSpendNow)
	return cli.Execute(t.Context(), append([]string{"recurring"}, args...), env)
}

func monthlyCharges(payee string, cents ...int64) store.Charges {
	rows := make([]store.Charge, len(cents))
	for i, c := range cents {
		rows[i] = store.Charge{
			TransactionID: "txn", SourceID: int64(i + 1),
			Date:    time.Date(2026, time.Month(2+i), 12, 0, 0, 0, 0, time.UTC),
			Account: store.Account{ID: "acct-1", Name: "Chequing", Currency: "CAD"},
			PayeeID: new("payee-1"), Payee: &payee, Currency: "CAD", Amount: c, ExpenseSplits: 1,
		}
	}
	return store.Charges{Rows: rows}
}

func Test_recurring_json_prints_the_document_and_no_table(t *testing.T) {
	var stdout, stderr bytes.Buffer
	fake := fakeReportStore{charges: monthlyCharges("Netflix.com", 999, 999, 999, 999)}

	err := executeRecurring(t, fake, &stdout, &stderr, "--since", "2000", "--json")

	require.NoError(t, err)
	assert.Empty(t, stderr.String())
	var doc struct {
		Since  string `json:"since"`
		Series []struct {
			Payee  string `json:"payee"`
			Amount string `json:"amount"`
		} `json:"series"`
	}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &doc), stdout.String())
	assert.Equal(t, "2000-01-01", doc.Since)
	require.Len(t, doc.Series, 1)
	assert.Equal(t, "Netflix.com", doc.Series[0].Payee)
	assert.Equal(t, "9.99", doc.Series[0].Amount)
	assert.NotContains(t, stdout.String(), "Recurring charges")
}

func Test_recurring_without_json_prints_the_table(t *testing.T) {
	var stdout, stderr bytes.Buffer
	fake := fakeReportStore{charges: monthlyCharges("Netflix.com", 999, 999, 999, 999)}

	err := executeRecurring(t, fake, &stdout, &stderr, "--since", "2000")

	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "Recurring charges 2000-01-01 to 2026-09-29 in all accounts")
	assert.NotContains(t, stdout.String(), `"series"`)
}

func Test_recurring_json_returns_the_report_fault_with_nothing_on_stdout(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := executeRecurring(t, fakeReportStore{err: errStoreRead}, &stdout, &stderr, "--json")

	require.ErrorIs(t, err, errStoreRead)
	assert.Empty(t, stdout.String())
	assert.Empty(t, stderr.String())
}

func Test_recurring_refuses_a_window_before_opening_the_report(t *testing.T) {
	var stdout, stderr bytes.Buffer
	env := failingReportEnv(errStoreRead, &stdout, &stderr, atWallClock)

	err := cli.Execute(t.Context(), []string{"recurring", "--since", "2024-13"}, env)

	var usage cli.UsageError
	require.ErrorAs(t, err, &usage)
	require.EqualError(t, err, `--since "2024-13" is not a date; use YYYY, YYYY-MM or YYYY-MM-DD`)
}

func Test_recurring_help_says_what_recurring_lists(t *testing.T) {
	const long = `List charges that repeat on a schedule: the same payee and currency every
week, month, quarter or year, at a steady amount. quarry finds them in all
your history, with the rules of quarry spend: expense splits only, without
transfers, refunds or accounts left out of reports. A transaction counts
once, with all its splits. Payees whose names differ only in store or
reference numbers count as one payee. Charges dated after today are left
out, even with a later --until.

Series are found in each account's own currency, so a change in the
exchange rate is never a price change, and a payee that charges in both
CAD and USD has two series. Amount and Per year are converted to the
reporting currency (--currency, else reporting.currency in the config
file, else CAD) at the rate on the latest charge's date; price changes
stay in the series' own currency. With --currency native nothing is
converted.

A charge that comes off schedule starts the series again. A series has
ended when no charge has come for 14 days (weekly), 45 days (monthly), 120
days (quarterly) or 400 days (yearly). Bills whose amount changes most
times, such as hydro, are not listed; see quarry spend --by payee.

--since and --until choose which series to list: those running at any
time in the period. A series whose first charge falls in the period is
marked new. A price change is a step of more than 5% from one charge to
the next, in the series' own currency. Per year is the latest amount
times the charges in a year, for active series only.
`
	var stdout, stderr bytes.Buffer

	err := executeRecurring(t, fakeReportStore{}, &stdout, &stderr, "--help")

	require.NoError(t, err)
	assert.Contains(t, stdout.String(), long)
}

func Test_recurring_reports_a_failed_stdout_write(t *testing.T) {
	err := executeRecurring(t, fakeReportStore{}, failingWriter{err: errNoSpace}, io.Discard)

	require.EqualError(t, err, "cannot write the result to stdout: write /dev/stdout: no space left on device")
	assert.ErrorIs(t, err, errNoSpace)
}

const recurringEmpty = "no recurring charges from 2026-01-01 to 2026-09-29"

// chargesOn is four monthly Netflix.com charges, all on the account with id.
func chargesOn(id string) store.Charges {
	charges := monthlyCharges("Netflix.com", 999, 999, 999, 999)
	for i := range charges.Rows {
		charges.Rows[i].Account = store.Account{ID: id, Name: "Chequing", Currency: "CAD"}
	}
	return charges
}

// withCharges is fake answering the recurring read with charges.
func withCharges(fake fakeReportStore, charges store.Charges) fakeReportStore {
	fake.charges = charges
	return fake
}

func Test_recurring_captions_the_named_accounts_and_passes_their_ids_to_the_report(t *testing.T) {
	var got store.ChargeParams
	var stdout, stderr bytes.Buffer
	fake := withCharges(namedAccounts(), chargesOn(visaID))
	fake.gotCharges = &got

	err := executeRecurring(t, fake, &stdout, &stderr, "--since", "2000", "--account", "visa infinite", "--account", chequingID)

	require.NoError(t, err)
	assert.Equal(t, []string{visaID, chequingID}, got.AccountIDs)
	assert.Contains(t, stdout.String(), "Recurring charges 2000-01-01 to 2026-09-29 in Visa Infinite, Chequing, amounts in CAD\n\n")
}

func Test_recurring_json_names_the_accounts_it_was_limited_to(t *testing.T) {
	var stdout, stderr bytes.Buffer
	fake := withCharges(namedAccounts(), chargesOn(chequingID))

	err := executeRecurring(t, fake, &stdout, &stderr, "--since", "2000", "--json", "--account", "chequing")

	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "\"account_filter\": [\n    {\n      \"id\": \""+chequingID+"\",\n      \"name\": \"Chequing\"\n    }\n  ],\n")
}

func Test_recurring_prints_each_warning_on_stderr_and_in_the_json_warnings(t *testing.T) {
	cases := []struct {
		name    string
		charges store.Charges
		args    []string
		want    []string
	}{
		{
			name:    "an account not in reports and one under linked tracking, each once in the order named",
			charges: chargesOn(chequingID),
			args:    []string{"--account", linkedID, "--account", "Old Card", "--account", chequingID, "--account", bothID, "--account", "old card"},
			want:    []string{linkedTrackingWarning("recurring", "Netskope 401(k)"), leftOutWarning("recurring", "Old Card"), linkedTrackingWarning("recurring", "Old 401(k)")},
		},
		{
			name:    "the store's span when no series runs in the period",
			charges: store.Charges{Transactions: span(t, "2003-01-04", "2026-09-26")},
			want:    []string{recurringEmpty + "; the store's transactions run 2003-01-04 to 2026-09-26"},
		},
		{
			name: "an empty store",
			want: []string{recurringEmpty + "; the store has no transactions"},
		},
		{
			name:    "the named accounts' span when none of their series runs in the period",
			charges: store.Charges{Transactions: span(t, "2019-03-02", "2024-11-30")},
			args:    []string{"--account", chequingID},
			want:    []string{recurringEmpty + " in the named accounts; their transactions run 2019-03-02 to 2024-11-30"},
		},
		{
			name: "named accounts with no transactions",
			args: []string{"--account", chequingID},
			want: []string{recurringEmpty + " in the named accounts; they have no transactions"},
		},
		{
			name:    "series that run only in other accounts than the one named",
			charges: chargesOn(visaID),
			args:    []string{"--since", "2000", "--account", chequingID},
			want:    []string{"no recurring charges from 2000-01-01 to 2026-09-29 in the named accounts; they have no transactions"},
		},
		{
			name:    "the left-out warnings before the empty note when a reported account is also named",
			charges: store.Charges{Transactions: span(t, "2019-03-02", "2024-11-30")},
			args:    []string{"--account", "Old Card", "--account", chequingID},
			want: []string{
				leftOutWarning("recurring", "Old Card"),
				recurringEmpty + " in the named accounts; their transactions run 2019-03-02 to 2024-11-30",
			},
		},
		{
			name:    "only the left-out warnings when every named account is left out",
			charges: store.Charges{Transactions: span(t, "2019-03-02", "2024-11-30")},
			args:    []string{"--account", linkedID, "--account", "Old Card"},
			want:    []string{linkedTrackingWarning("recurring", "Netskope 401(k)"), leftOutWarning("recurring", "Old Card")},
		},
		{
			name:    "only the left-out warning when every named account is left out and the store is empty",
			charges: store.Charges{},
			args:    []string{"--account", oldBankID},
			want:    []string{leftOutWarning("recurring", "Old Bank")},
		},
		{
			name:    "no note when a series is listed",
			charges: chargesOn(chequingID),
			args:    []string{"--since", "2000", "--account", chequingID},
			want:    []string{},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			fake := withCharges(namedAccounts(), c.charges)

			err := executeRecurring(t, fake, &stdout, &stderr, append([]string{"--json"}, c.args...)...)

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

// warningLines is the stderr text of warnings: each on its own prefixed line.
func warningLines(warnings []string) string {
	var text strings.Builder
	for _, w := range warnings {
		text.WriteString("quarry: warning: " + w + "\n")
	}
	return text.String()
}

func Test_recurring_reads_the_charges_once_and_takes_the_empty_note_from_that_read(t *testing.T) {
	var reads int
	var stdout, stderr bytes.Buffer
	fake := fakeReportStore{charges: store.Charges{Transactions: span(t, "2003-01-04", "2026-09-26")}, chargesReads: &reads}

	err := executeRecurring(t, fake, &stdout, &stderr)

	require.NoError(t, err)
	assert.Equal(t, 1, reads)
	assert.Equal(t, "quarry: warning: "+recurringEmpty+"; the store's transactions run 2003-01-04 to 2026-09-26\n", stderr.String())
}

func Test_recurring_text_prints_caption_and_header_only_for_an_empty_period(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := executeRecurring(t, fakeReportStore{}, &stdout, &stderr)

	require.NoError(t, err)
	assert.Equal(t, "Recurring charges 2026-01-01 to 2026-09-29 in all accounts, amounts in CAD\n\n"+
		"Payee  Currency  Every  Amount  Per year  First  Last  Status  Price changes\n", stdout.String())
}

// executeRecurringIn runs recurring over fake with a config whose reporting.currency is cfg.
func executeRecurringIn(t *testing.T, cfg money.Currency, fake fakeReportStore, stdout, stderr *bytes.Buffer, args ...string) error {
	t.Helper()
	env := reportEnv(fake, stdout, stderr, atSpendNow, withConfig(config.Config{Currency: cfg}))
	return cli.Execute(t.Context(), append([]string{"recurring", "--since", "2000"}, args...), env)
}

// cadSeriesWithUSDCells is a monthly CAD series of 10.00 whose charges all carry the USD cell 7.50 (rate 1.3333).
func cadSeriesWithUSDCells() store.Charges {
	c := monthlyCharges("Gym", 1000, 1000, 1000, 1000)
	for i := range c.Rows {
		c.Rows[i].AmountCAD = new(int64(1000))
		c.Rows[i].AmountUSD = new(int64(750))
		c.Rows[i].USDCAD = money.Rate(1_333_333)
	}
	c.FirstRate = time.Date(2026, time.January, 2, 0, 0, 0, 0, time.UTC)
	return c
}

func Test_recurring_lists_amounts_in_the_currency_the_config_names(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := executeRecurringIn(t, money.USD, fakeReportStore{charges: cadSeriesWithUSDCells()}, &stdout, &stderr)

	require.NoError(t, err)
	assert.Empty(t, stderr.String())
	assert.Contains(t, stdout.String(), "Recurring charges 2000-01-01 to 2026-09-29 in all accounts, amounts in USD\n\n")
	assert.Contains(t, stdout.String(), "Gym    USD (CAD)  month    7.50")
}

func Test_recurring_flag_beats_the_config_and_native_converts_nothing(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := executeRecurringIn(t, money.USD, fakeReportStore{charges: cadSeriesWithUSDCells()}, &stdout, &stderr, "--currency", "native")

	require.NoError(t, err)
	assert.Empty(t, stderr.String())
	assert.Contains(t, stdout.String(), "in all accounts\n\n")
	assert.NotContains(t, stdout.String(), "amounts in")
	assert.Contains(t, stdout.String(), "Gym    CAD       month   10.00")
}

func Test_recurring_warns_of_a_series_dated_before_the_first_rate_in_the_config_currency(t *testing.T) {
	charges := monthlyCharges("Gym", 1000, 1000, 1000, 1000)
	charges.FirstRate = time.Date(2026, time.April, 1, 0, 0, 0, 0, time.UTC)
	var stdout, stderr bytes.Buffer

	err := executeRecurringIn(t, money.USD, fakeReportStore{charges: charges}, &stdout, &stderr)

	require.NoError(t, err)
	assert.Equal(t, "quarry: warning: 1 series with a charge dated before 2026-04-01, the first exchange rate in the store, "+
		"is listed in CAD, not converted to USD\n", stderr.String())
	assert.Contains(t, stdout.String(), "Gym    CAD       month   10.00")
}

func Test_recurring_prints_the_left_out_account_warning_before_the_unconverted_series_warning(t *testing.T) {
	charges := chargesOn(chequingID)
	for i := range charges.Rows {
		charges.Rows[i].Currency = "USD"
	}
	charges.FirstRate = time.Date(2026, time.April, 1, 0, 0, 0, 0, time.UTC)
	want := []string{
		leftOutWarning("recurring", "Old Card"),
		"1 series with a charge dated before 2026-04-01, the first exchange rate in the store, is listed in USD, not converted to CAD",
	}
	args := []string{"--since", "2000", "--account", "Old Card", "--account", chequingID}

	t.Run("text prints them on stderr in that order", func(t *testing.T) {
		var stdout, stderr bytes.Buffer

		err := executeRecurring(t, withCharges(namedAccounts(), charges), &stdout, &stderr, args...)

		require.NoError(t, err)
		assert.Equal(t, warningLines(want), stderr.String())
	})

	t.Run("json lists them in that order in warnings", func(t *testing.T) {
		var stdout, stderr bytes.Buffer

		err := executeRecurring(t, withCharges(namedAccounts(), charges), &stdout, &stderr, append([]string{"--json"}, args...)...)

		require.NoError(t, err)
		var doc struct {
			Warnings []string `json:"warnings"`
		}
		require.NoError(t, json.Unmarshal(stdout.Bytes(), &doc), stdout.String())
		assert.Equal(t, want, doc.Warnings)
	})
}
