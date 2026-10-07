package cli_test

import (
	"bytes"
	"encoding/json"
	"io"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/cli"
	"github.com/koblas/quarry/internal/config"
	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// utcMinus5 is a zone whose evening is already the next day in UTC.
var utcMinus5 = time.FixedZone("UTC-5", -5*60*60)

func executeSpend(t *testing.T, fake fakeReportStore, now time.Time, stdout, stderr io.Writer, args ...string) error {
	t.Helper()
	env := reportEnv(fake, stdout, stderr, atTime(now))
	return cli.Execute(t.Context(), append([]string{"spend"}, args...), env)
}

func Test_spend_reads_the_window_from_the_env_clock(t *testing.T) {
	var got store.SpendingParams
	var stdout, stderr bytes.Buffer
	now := time.Date(2026, 12, 31, 22, 0, 0, 0, utcMinus5)

	fake := withSpending(fakeReportStore{gotSpending: &got})

	err := executeSpend(t, fake, now, &stdout, &stderr)

	require.NoError(t, err)
	assert.Equal(t, store.Window{
		Since: time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC),
		Until: time.Date(2026, time.December, 31, 0, 0, 0, 0, time.UTC),
	}, got.Window)
	assert.Equal(t, store.SpendByCategory, got.By)
	assert.Equal(t, "Spending 2026-01-01 to 2026-12-31 in all accounts, amounts in CAD\n\nCategory  Currency  Spent\nTotal     CAD        1.00\n", stdout.String())
	assert.Empty(t, stderr.String())
}

func Test_spend_by_payee_reads_the_payee_grouping_and_heads_the_first_column_Payee(t *testing.T) {
	var got store.SpendingParams
	var stdout, stderr bytes.Buffer
	fake := fakeReportStore{gotSpending: &got, spending: store.Spending{
		Rows:   []store.SpendingRow{{Key: nil, Currency: "CAD", Spent: 4208}},
		Totals: []store.SpendingTotal{{Currency: "CAD", Spent: 4208}},
	}}

	err := executeSpend(t, fake, spendNow, &stdout, &stderr, "--by", "payee")

	require.NoError(t, err)
	assert.Equal(t, store.SpendByPayee, got.By)
	assert.Equal(t, "Spending 2026-01-01 to 2026-09-29 in all accounts, amounts in CAD\n\n"+
		"Payee       Currency  Spent\n"+
		"(no payee)  CAD       42.08\n"+
		"Total       CAD       42.08\n", stdout.String())
}

func Test_spend_refuses_a_by_that_names_no_grouping_before_reading_the_store(t *testing.T) {
	for _, by := range []string{"vendor", "", "Payee"} {
		t.Run(by, func(t *testing.T) {
			var stdout, stderr bytes.Buffer

			err := executeSpend(t, fakeReportStore{err: errStoreRead}, time.Now(), &stdout, &stderr, "--by", by)

			var usage cli.UsageError
			require.ErrorAs(t, err, &usage)
			require.EqualError(t, err, "--by must be category, payee, tag or month")
			assert.Empty(t, stdout.String())
			assert.Empty(t, stderr.String())
		})
	}
}

func Test_spend_refuses_a_by_that_names_no_grouping_before_opening_the_report(t *testing.T) {
	var stdout, stderr bytes.Buffer
	env := failingReportEnv(errStoreRead, &stdout, &stderr, atWallClock)

	err := cli.Execute(t.Context(), []string{"spend", "--by", "vendor"}, env)

	var usage cli.UsageError
	require.ErrorAs(t, err, &usage)
	require.EqualError(t, err, "--by must be category, payee, tag or month")
	assert.Empty(t, stdout.String())
	assert.Empty(t, stderr.String())
}

func Test_spend_json_puts_the_report_window_and_rows_in_the_document(t *testing.T) {
	var stdout, stderr bytes.Buffer
	fake := fakeReportStore{spending: store.Spending{
		Rows:   []store.SpendingRow{{Key: new("Auto:Fuel"), Currency: "CAD", Spent: 120450}},
		Totals: []store.SpendingTotal{{Currency: "CAD", Spent: 120450}},
	}}

	err := executeSpend(t, fake, spendNow, &stdout, &stderr, "--json")

	require.NoError(t, err)
	assert.JSONEq(t, `{"since":"2026-01-01","until":"2026-09-29","by":"category","currency":"CAD","account_filter":[],
		"rows":[{"category":"Auto:Fuel","currency":"CAD","spent":"1204.50"}],
		"totals":[{"currency":"CAD","spent":"1204.50"}],"warnings":[]}`, stdout.String())
	assert.Empty(t, stderr.String())
}

func Test_spend_returns_the_report_fault(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := executeSpend(t, fakeReportStore{err: errStoreRead}, time.Now(), &stdout, &stderr)

	require.ErrorIs(t, err, errStoreRead)
	assert.Empty(t, stdout.String())
	assert.Empty(t, stderr.String())
}

func Test_spend_returns_the_report_factory_fault(t *testing.T) {
	var stdout, stderr bytes.Buffer
	env := failingReportEnv(errStoreRead, &stdout, &stderr, atWallClock)

	err := cli.Execute(t.Context(), []string{"spend"}, env)

	require.ErrorIs(t, err, errStoreRead)
	assert.Empty(t, stdout.String())
}

func Test_spend_reports_a_failed_stdout_write(t *testing.T) {
	err := executeSpend(t, fakeReportStore{}, time.Now(), failingWriter{err: errNoSpace}, io.Discard)

	require.EqualError(t, err, "cannot write the result to stdout: write /dev/stdout: no space left on device")
	assert.ErrorIs(t, err, errNoSpace)
}

const (
	chequingID = "acct-chq"
	visaID     = "acct-visa"
	oldCardID  = "acct-old"
	oldBankID  = "acct-bank"
	linkedID   = "acct-401k"
	bothID     = "acct-both"
)

func accountsStore(accounts ...store.Account) fakeReportStore {
	list := make([]store.AccountBalance, len(accounts))
	for i, a := range accounts {
		list[i] = store.AccountBalance{Account: a}
	}
	return fakeReportStore{accounts: store.AccountList{Accounts: list}}
}

func namedAccounts() fakeReportStore {
	return accountsStore(
		store.Account{ID: chequingID, Name: "Chequing"},
		store.Account{ID: visaID, Name: "Visa Infinite"},
		store.Account{ID: oldCardID, Name: "Old Card", NotInReports: true},
		store.Account{ID: oldBankID, Name: "Old Bank", NotInReports: true},
		store.Account{ID: linkedID, Name: "Netskope 401(k)", LinkedTracking: true},
		store.Account{ID: bothID, Name: "Old 401(k)", NotInReports: true, LinkedTracking: true},
	)
}

// withSpending is fake answering every spend read with one CAD total, so the window is not empty.
func withSpending(fake fakeReportStore) fakeReportStore {
	fake.spending = store.Spending{Totals: []store.SpendingTotal{{Currency: "CAD", Spent: 100}}}
	return fake
}

func Test_spend_captions_the_named_accounts(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := executeSpend(t, namedAccounts(), spendNow, &stdout, &stderr, "--account", "visa infinite", "--account", chequingID)

	require.NoError(t, err)
	assert.Equal(t, "Spending 2026-01-01 to 2026-09-29 in Visa Infinite, Chequing, amounts in CAD\n\nCategory  Currency  Spent\n", stdout.String())
}

func Test_spend_json_lists_the_named_accounts_in_account_filter(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := executeSpend(t, withSpending(namedAccounts()), spendNow, &stdout, &stderr,
		"--account", "visa infinite", "--account", chequingID, "--json")

	require.NoError(t, err)
	assert.JSONEq(t, `{"since":"2026-01-01","until":"2026-09-29","by":"category","currency":"CAD",
		"account_filter":[{"id":"acct-visa","name":"Visa Infinite"},{"id":"acct-chq","name":"Chequing"}],
		"rows":[],"totals":[{"currency":"CAD","spent":"1.00"}],"warnings":[]}`, stdout.String())
}

func Test_spend_passes_every_account_flag_to_the_report(t *testing.T) {
	var got store.SpendingParams
	var stdout, stderr bytes.Buffer
	fake := namedAccounts()
	fake.gotSpending = &got

	err := executeSpend(t, fake, spendNow, &stdout, &stderr, "--account", "Old Card", "--account", visaID)

	require.NoError(t, err)
	assert.Equal(t, []string{oldCardID, visaID}, got.AccountIDs)
}

func Test_spend_json_puts_w2_in_warnings_unprefixed_before_w1(t *testing.T) {
	var stdout, stderr bytes.Buffer
	fake := namedAccounts()
	fake.spending = store.Spending{MultiTagSplits: 2}

	err := executeSpend(t, fake, spendNow, &stdout, &stderr, "--by", "tag", "--account", "Old Card", "--json")

	require.NoError(t, err)
	const w1 = "2 splits carry more than one tag, so the rows add up to more than the total"
	warnings, err := json.Marshal([]string{leftOutWarning("spend", "Old Card"), w1})
	require.NoError(t, err)
	assert.JSONEq(t, `{"since":"2026-01-01","until":"2026-09-29","by":"tag","currency":"CAD",
		"account_filter":[{"id":"acct-old","name":"Old Card"}],"rows":[],"totals":[],
		"warnings":`+string(warnings)+`}`, stdout.String())
	assert.Equal(t, "quarry: warning: "+leftOutWarning("spend", "Old Card")+"\nquarry: warning: "+w1+"\n", stderr.String())
}

func Test_spend_warns_once_per_named_account_left_out_of_reports_in_the_order_given(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := executeSpend(t, withSpending(namedAccounts()), spendNow, &stdout, &stderr,
		"--account", "Old Card", "--account", chequingID, "--account", oldBankID, "--account", "old card")

	require.NoError(t, err)
	assert.Equal(t, "quarry: warning: "+leftOutWarning("spend", "Old Card")+"\nquarry: warning: "+leftOutWarning("spend", "Old Bank")+"\n",
		stderr.String())
}

func Test_spend_warns_about_a_linked_tracking_account_in_the_order_given_among_the_left_out_warnings(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := executeSpend(t, withSpending(namedAccounts()), spendNow, &stdout, &stderr,
		"--account", "Old Card", "--account", linkedID, "--account", oldBankID, "--account", chequingID)

	require.NoError(t, err)
	assert.Equal(t, "quarry: warning: "+leftOutWarning("spend", "Old Card")+"\nquarry: warning: "+linkedTrackingWarning("spend", "Netskope 401(k)")+
		"\nquarry: warning: "+leftOutWarning("spend", "Old Bank")+"\n", stderr.String())
}

func Test_spend_warns_only_that_linked_tracking_leaves_out_an_account_that_is_also_not_in_reports(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := executeSpend(t, withSpending(namedAccounts()), spendNow, &stdout, &stderr, "--account", bothID)

	require.NoError(t, err)
	assert.Equal(t, "quarry: warning: "+linkedTrackingWarning("spend", "Old 401(k)")+"\n", stderr.String())
}

func Test_spend_json_lists_a_linked_tracking_warning_before_a_left_out_of_reports_one_unprefixed(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := executeSpend(t, withSpending(namedAccounts()), spendNow, &stdout, &stderr,
		"--account", linkedID, "--account", "Old Card", "--json")

	require.NoError(t, err)
	var doc struct {
		Warnings []string `json:"warnings"`
	}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &doc))
	assert.Equal(t, []string{linkedTrackingWarning("spend", "Netskope 401(k)"), leftOutWarning("spend", "Old Card")}, doc.Warnings)
}

func Test_spend_does_not_warn_about_an_account_in_reports(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := executeSpend(t, withSpending(namedAccounts()), spendNow, &stdout, &stderr, "--account", chequingID)

	require.NoError(t, err)
	assert.Empty(t, stderr.String())
}

func Test_spend_refuses_an_unknown_account_without_warning_about_an_excluded_one(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := executeSpend(t, namedAccounts(), spendNow, &stdout, &stderr, "--account", "Old Card", "--account", "Nowhere")

	require.EqualError(t, err, "no account named \"Nowhere\"; run quarry accounts --all to list them")
	assert.Empty(t, stderr.String())
	assert.Empty(t, stdout.String())
}

func Test_spend_refuses_an_unknown_account_without_warning_about_a_linked_tracking_one(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := executeSpend(t, namedAccounts(), spendNow, &stdout, &stderr, "--account", linkedID, "--account", "Nowhere")

	require.EqualError(t, err, "no account named \"Nowhere\"; run quarry accounts --all to list them")
	assert.Empty(t, stderr.String())
	assert.Empty(t, stdout.String())
}

func Test_spend_reads_in_the_currency_the_resolver_picks(t *testing.T) {
	cases := []struct {
		name   string
		config money.Currency
		args   []string
		want   money.Currency
	}{
		{name: "the config's currency without the flag", config: money.USD, want: money.USD},
		{name: "the flag", config: money.CAD, args: []string{"--currency", "usd"}, want: money.USD},
		{name: "the flag beats the config", config: money.USD, args: []string{"--currency", "CAD"}, want: money.CAD},
		{name: "native by flag", config: money.CAD, args: []string{"--currency", "native"}, want: money.Native},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var got store.SpendingParams
			var stdout, stderr bytes.Buffer
			env := reportEnv(fakeReportStore{gotSpending: &got}, &stdout, &stderr, atSpendNow, withConfig(config.Config{Currency: c.config}))

			err := cli.Execute(t.Context(), append([]string{"spend"}, c.args...), env)

			require.NoError(t, err)
			assert.Equal(t, c.want, got.Currency)
		})
	}
}

const (
	emptyPrefix = "quarry: warning: no spending from 2026-01-01 to 2026-09-29"
	emptyE1a    = "no spending from 2026-01-01 to 2026-09-29 in the named accounts; their transactions run 2019-03-02 to 2024-11-30"
)

func Test_spend_says_when_the_window_holds_nothing(t *testing.T) {
	cases := []struct {
		name string
		span store.TransactionRange
		args []string
		want string
	}{
		{
			name: "the store has transactions elsewhere",
			span: span(t, "2003-01-04", "2026-09-26"),
			want: emptyPrefix + "; the store's transactions run 2003-01-04 to 2026-09-26\n",
		},
		{
			name: "the store has no transactions",
			want: emptyPrefix + "; the store has no transactions\n",
		},
		{
			name: "the named accounts have transactions elsewhere",
			span: span(t, "2019-03-02", "2024-11-30"),
			args: []string{"--account", chequingID},
			want: "quarry: warning: " + emptyE1a + "\n",
		},
		{
			name: "the named accounts have no transactions",
			args: []string{"--account", chequingID},
			want: emptyPrefix + " in the named accounts; they have no transactions\n",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			fake := namedAccounts()
			fake.spending = store.Spending{Transactions: c.span}

			err := executeSpend(t, fake, spendNow, &stdout, &stderr, c.args...)

			require.NoError(t, err)
			assert.Equal(t, c.want, stderr.String())
		})
	}
}

func Test_spend_json_puts_the_empty_window_note_in_warnings_unprefixed(t *testing.T) {
	var stdout, stderr bytes.Buffer
	fake := namedAccounts()
	fake.spending = store.Spending{Transactions: span(t, "2003-01-04", "2026-09-26")}

	err := executeSpend(t, fake, spendNow, &stdout, &stderr, "--json")

	require.NoError(t, err)
	assert.JSONEq(t, `{"since":"2026-01-01","until":"2026-09-29","by":"category","currency":"CAD","account_filter":[],
		"rows":[],"totals":[],"warnings":["no spending from 2026-01-01 to 2026-09-29; the store's transactions run 2003-01-04 to 2026-09-26"]}`,
		stdout.String())
}

func Test_spend_says_nothing_of_an_empty_window_when_every_named_account_is_left_out_of_reports(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := executeSpend(t, namedAccounts(), spendNow, &stdout, &stderr, "--account", "Old Card", "--account", oldBankID)

	require.NoError(t, err)
	assert.Equal(t, "quarry: warning: "+leftOutWarning("spend", "Old Card")+"\nquarry: warning: "+leftOutWarning("spend", "Old Bank")+"\n",
		stderr.String())
}

func Test_spend_says_nothing_of_an_empty_window_when_every_named_account_is_left_out_and_one_is_linked(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := executeSpend(t, namedAccounts(), spendNow, &stdout, &stderr, "--account", "Old Card", "--account", linkedID)

	require.NoError(t, err)
	assert.Equal(t, "quarry: warning: "+leftOutWarning("spend", "Old Card")+"\nquarry: warning: "+linkedTrackingWarning("spend", "Netskope 401(k)")+"\n",
		stderr.String())
}

func Test_spend_puts_the_empty_window_note_after_the_linked_tracking_warning_when_a_reported_account_is_named(t *testing.T) {
	var stdout, stderr bytes.Buffer
	fake := namedAccounts()
	fake.spending = store.Spending{Transactions: span(t, "2019-03-02", "2024-11-30")}

	err := executeSpend(t, fake, spendNow, &stdout, &stderr, "--account", linkedID, "--account", chequingID)

	require.NoError(t, err)
	assert.Equal(t, "quarry: warning: "+linkedTrackingWarning("spend", "Netskope 401(k)")+"\nquarry: warning: "+emptyE1a+"\n", stderr.String())
}

func Test_spend_puts_the_empty_window_note_after_the_left_out_of_reports_warnings(t *testing.T) {
	var stdout, stderr bytes.Buffer
	fake := namedAccounts()
	fake.spending = store.Spending{Transactions: span(t, "2019-03-02", "2024-11-30")}

	err := executeSpend(t, fake, spendNow, &stdout, &stderr,
		"--account", "Old Card", "--account", chequingID, "--json")

	require.NoError(t, err)
	assert.Equal(t, "quarry: warning: "+leftOutWarning("spend", "Old Card")+"\nquarry: warning: "+emptyE1a+"\n", stderr.String())
	var doc struct {
		Warnings []string `json:"warnings"`
	}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &doc))
	assert.Equal(t, []string{leftOutWarning("spend", "Old Card"), emptyE1a}, doc.Warnings)
}

func Test_spend_says_nothing_of_an_empty_window_when_a_currency_nets_to_zero(t *testing.T) {
	var stdout, stderr bytes.Buffer
	fake := namedAccounts()
	fake.spending = store.Spending{Totals: []store.SpendingTotal{{Currency: "CAD", Spent: 0}}, Transactions: span(t, "2003-01-04", "2026-09-26")}

	err := executeSpend(t, fake, spendNow, &stdout, &stderr)

	require.NoError(t, err)
	assert.Empty(t, stderr.String())
}

func tagSpending(multiTagSplits int) fakeReportStore {
	return fakeReportStore{spending: store.Spending{
		Rows: []store.SpendingRow{
			{Key: nil, Currency: "CAD", Spent: 4208},
			{Key: new("trip"), Currency: "CAD", Spent: 30000},
		},
		Totals:         []store.SpendingTotal{{Currency: "CAD", Spent: 34208}},
		MultiTagSplits: multiTagSplits,
	}}
}

func Test_spend_by_tag_heads_the_first_column_Tag_and_labels_untagged_splits(t *testing.T) {
	var got store.SpendingParams
	var stdout, stderr bytes.Buffer
	fake := tagSpending(0)
	fake.gotSpending = &got

	err := executeSpend(t, fake, spendNow, &stdout, &stderr, "--by", "tag")

	require.NoError(t, err)
	assert.Equal(t, store.SpendByTag, got.By)
	assert.Equal(t, "Spending 2026-01-01 to 2026-09-29 in all accounts, amounts in CAD\n\n"+
		"Tag       Currency   Spent\n"+
		"(no tag)  CAD        42.08\n"+
		"trip      CAD       300.00\n"+
		"Total     CAD       342.08\n", stdout.String())
}

func Test_spend_by_tag_json_names_the_row_key_tag_and_carries_the_warning_unprefixed(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := executeSpend(t, tagSpending(2), spendNow, &stdout, &stderr, "--by", "tag", "--json")

	require.NoError(t, err)
	assert.JSONEq(t, `{"since":"2026-01-01","until":"2026-09-29","by":"tag","currency":"CAD","account_filter":[],
		"rows":[{"tag":null,"currency":"CAD","spent":"42.08"},{"tag":"trip","currency":"CAD","spent":"300.00"}],
		"totals":[{"currency":"CAD","spent":"342.08"}],
		"warnings":["2 splits carry more than one tag, so the rows add up to more than the total"]}`, stdout.String())
	assert.Equal(t, "quarry: warning: 2 splits carry more than one tag, so the rows add up to more than the total\n", stderr.String())
}

func Test_spend_by_tag_warns_with_the_singular_phrase_for_one_split(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := executeSpend(t, tagSpending(1), spendNow, &stdout, &stderr, "--by", "tag")

	require.NoError(t, err)
	assert.Equal(t, "quarry: warning: 1 split carries more than one tag, so the rows add up to more than the total\n", stderr.String())
}

func Test_spend_by_tag_warns_with_thousands_grouping_for_many_splits(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := executeSpend(t, tagSpending(1234), spendNow, &stdout, &stderr, "--by", "tag")

	require.NoError(t, err)
	assert.Equal(t, "quarry: warning: 1,234 splits carry more than one tag, so the rows add up to more than the total\n", stderr.String())
}

func Test_spend_by_tag_prints_no_warning_when_no_split_has_two_tags(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := executeSpend(t, tagSpending(0), spendNow, &stdout, &stderr, "--by", "tag", "--json")

	require.NoError(t, err)
	assert.Empty(t, stderr.String())
	assert.Contains(t, stdout.String(), `"warnings": []`)
}

func Test_spend_by_tag_writes_no_warning_when_stdout_fails(t *testing.T) {
	var stderr bytes.Buffer

	err := executeSpend(t, tagSpending(3), spendNow, failingWriter{err: errNoSpace}, &stderr, "--by", "tag")

	require.EqualError(t, err, "cannot write the result to stdout: write /dev/stdout: no space left on device")
	assert.Empty(t, stderr.String())
}

func Test_spend_by_category_prints_no_warning_whatever_the_multi_tag_count(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := executeSpend(t, tagSpending(5), spendNow, &stdout, &stderr, "--by", "category")

	require.NoError(t, err)
	assert.Empty(t, stderr.String())
}

func civilDay(year int, month time.Month, day int) time.Time {
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

func Test_spend_reads_the_period_its_since_and_until_flags_name(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want store.Window
	}{
		{
			name: "both flags cover their whole months",
			args: []string{"--since", "2025-03", "--until", "2025-05"},
			want: store.Window{Since: civilDay(2025, time.March, 1), Until: civilDay(2025, time.May, 31)},
		},
		{
			name: "since alone runs to today",
			args: []string{"--since", "2026-03"},
			want: store.Window{Since: civilDay(2026, time.March, 1), Until: civilDay(2026, time.September, 29)},
		},
		{
			name: "until alone starts on January 1 of this year",
			args: []string{"--until", "2026-12"},
			want: store.Window{Since: civilDay(2026, time.January, 1), Until: civilDay(2026, time.December, 31)},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var got store.SpendingParams
			var stdout, stderr bytes.Buffer

			err := executeSpend(t, fakeReportStore{gotSpending: &got}, spendNow, &stdout, &stderr, c.args...)

			require.NoError(t, err)
			assert.Equal(t, c.want, got.Window)
		})
	}
}

func Test_spend_refuses_a_period_it_cannot_use_before_reading_the_store(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{
			"a since that is not a date",
			[]string{"--since", "2024-13"},
			`--since "2024-13" is not a date; use YYYY, YYYY-MM or YYYY-MM-DD`,
		},
		{
			"an until that is not a date",
			[]string{"--until", "2024-13"},
			`--until "2024-13" is not a date; use YYYY, YYYY-MM or YYYY-MM-DD`,
		},
		{
			"a since after until",
			[]string{"--since", "2025", "--until", "2024"},
			"--since 2025 is after --until 2024",
		},
		{
			"an until before the default since",
			[]string{"--until", "2024"},
			"--until 2024 is before the default --since 2026-01-01; pass --since too",
		},
		{
			"a since after today",
			[]string{"--since", "2027"},
			"--since 2027 is after today; pass --until to include future-dated transactions",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer

			err := executeSpend(t, fakeReportStore{err: errStoreRead}, spendNow, &stdout, &stderr, c.args...)

			var usage cli.UsageError
			require.ErrorAs(t, err, &usage)
			require.EqualError(t, err, c.want)
			assert.Empty(t, stdout.String())
			assert.Empty(t, stderr.String())
		})
	}
}

func Test_spend_refuses_a_bad_period_before_opening_the_report(t *testing.T) {
	var stdout, stderr bytes.Buffer
	env := failingReportEnv(errStoreRead, &stdout, &stderr, atSpendNow)

	err := cli.Execute(t.Context(), []string{"spend", "--since", "2024-13"}, env)

	var usage cli.UsageError
	require.ErrorAs(t, err, &usage)
	assert.Empty(t, stdout.String())
	assert.Empty(t, stderr.String())
}

func Test_spend_refuses_an_empty_period_flag_as_a_bad_date(t *testing.T) {
	cases := []struct {
		flag string
		want string
	}{
		{"--since", `--since "" is not a date; use YYYY, YYYY-MM or YYYY-MM-DD`},
		{"--until", `--until "" is not a date; use YYYY, YYYY-MM or YYYY-MM-DD`},
	}

	for _, c := range cases {
		t.Run(c.flag, func(t *testing.T) {
			var stdout, stderr bytes.Buffer

			err := executeSpend(t, fakeReportStore{}, spendNow, &stdout, &stderr, c.flag, "")

			var usage cli.UsageError
			require.ErrorAs(t, err, &usage)
			require.EqualError(t, err, c.want)
		})
	}
}

func Test_spend_refuses_a_by_that_names_no_grouping_before_a_bad_period(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := executeSpend(t, fakeReportStore{}, spendNow, &stdout, &stderr, "--by", "vendor", "--since", "2024-13")

	require.EqualError(t, err, "--by must be category, payee, tag or month")
}

func Test_spend_help_shows_the_since_and_until_flags(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := executeSpend(t, fakeReportStore{}, spendNow, &stdout, &stderr, "--help")

	require.NoError(t, err)
	assert.Regexp(t, `--since date +count transactions dated on or after date `+
		`\(YYYY, YYYY-MM or YYYY-MM-DD; default January 1 this year\)`, stdout.String())
	assert.Regexp(t, `--until date +count transactions dated on or before date `+
		`\(YYYY, YYYY-MM or YYYY-MM-DD; default today\)`, stdout.String())
}

func Test_spend_help_shows_the_account_flag(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := executeSpend(t, fakeReportStore{}, spendNow, &stdout, &stderr, "--help")

	require.NoError(t, err)
	assert.Regexp(t, `--account name +count only the account with this name or id; repeat for more`, stdout.String())
}
