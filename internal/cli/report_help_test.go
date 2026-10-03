package cli_test

import (
	"bytes"
	"regexp"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/cli"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_spend_help_says_what_spend_counts(t *testing.T) {
	const long = `Show how much you spent, grouped by category, payee, tag or month.

Amounts are in CAD unless --currency or reporting.currency in
~/Library/Application Support/quarry/config.toml names another currency.
Each split converts at the Bank of Canada rate for its date, or the latest
earlier rate on weekends, holidays and dates after the last stored rate,
and is rounded to the cent before it is added. With --currency native, CAD
and USD are listed separately, never added together. Amounts dated before
the first stored rate stay in their own currency, on rows of their own,
with a warning.

Spending is every split in an expense category, plus uncategorized splits
that take money out. Refunds in an expense category are netted against it,
so a category can come out negative. Transfers between your own accounts,
splits in Quicken's system categories and transactions marked "exclude from
reports" in Quicken are left out. So are accounts Quicken leaves out of
reports (quarry accounts marks them "not in reports") and accounts that use
Quicken's linked account tracking (marked "linked tracking"). Closed
accounts are included.

The period runs from --since to --until, both included; a bare year or month
covers all of it (--since 2024 --until 2024 is the whole of 2024). Without
them it is this year up to today, so future-dated transactions are left out
unless --until is later than today.

A split with more than one tag counts under each of them, so with --by tag
the rows can add up to more than the total.
`
	var stdout, stderr bytes.Buffer

	err := executeSpend(t, fakeReportStore{}, spendNow, &stdout, &stderr, "--help")

	require.NoError(t, err)
	assert.Contains(t, stdout.String(), long)
}

func Test_recurring_help_says_series_are_found_in_their_own_currency(t *testing.T) {
	const converted = `Series are found in each account's own currency, so a change in the
exchange rate is never a price change, and a payee that charges in both
CAD and USD has two series. Amount and Per year are converted to the
reporting currency (--currency, else reporting.currency in the config
file, else CAD) at the rate on the latest charge's date; price changes
stay in the series' own currency. With --currency native nothing is
converted.
`
	const priceChange = `the next, in the series' own currency. Per year is the latest amount
times the charges in a year, for active series only.
`
	var stdout, stderr bytes.Buffer

	err := executeRecurring(t, fakeReportStore{}, &stdout, &stderr, "--help")

	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "out, even with a later --until.\n\n"+converted+"\nA charge that comes off schedule")
	assert.Contains(t, stdout.String(), priceChange)
}

func Test_spend_help_shows_examples(t *testing.T) {
	const examples = `Examples:
  quarry spend
  quarry spend --by payee --since 2025-01 --until 2025-03
  quarry spend --since 2024 --until 2024 --json
  quarry spend --account "Visa Infinite" --account Chequing
`
	var stdout, stderr bytes.Buffer

	err := executeSpend(t, fakeReportStore{}, spendNow, &stdout, &stderr, "--help")

	require.NoError(t, err)
	assert.Contains(t, stdout.String(), examples)
}

func Test_spend_help_shows_the_by_flag_and_its_default(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := executeSpend(t, fakeReportStore{}, spendNow, &stdout, &stderr, "--help")

	require.NoError(t, err)
	assert.Regexp(t, `--by group +group spending by group: category, payee, tag or month \(default "category"\)`, stdout.String())
}

func Test_cashflow_help_says_what_cashflow_counts(t *testing.T) {
	const long = `Show income, spending and what was left over for each month or year.

Amounts are in CAD unless --currency or reporting.currency in
~/Library/Application Support/quarry/config.toml names another currency.
Each split converts at the Bank of Canada rate for its date, or the latest
earlier rate on weekends, holidays and dates after the last stored rate,
and is rounded to the cent before it is added. With --currency native, CAD
and USD are listed separately, never added together. Amounts dated before
the first stored rate stay in their own currency, on rows of their own,
with a warning.

Income and spending follow the same rules as quarry spend: transfers between
your own accounts, Quicken's system categories and transactions marked
"exclude from reports" are left out, and refunds are netted. Accounts Quicken
leaves out of reports ("not in reports" in quarry accounts) and accounts
that use Quicken's linked account tracking ("linked tracking") are left out
here too. Uncategorized splits count as income when they bring money in and
as spending when they take money out. The Spent column equals quarry spend's
total for the same period, accounts and currency.

Savings rate is net divided by income, and shows n/a when income is zero or
less. A period that --since or --until cuts short is marked partial.
`
	var stdout, stderr bytes.Buffer

	err := executeCashFlow(t, fakeReportStore{}, &stdout, &stderr, "--help")

	require.NoError(t, err)
	assert.Contains(t, stdout.String(), long)
}

func Test_cashflow_help_shows_examples(t *testing.T) {
	const examples = `Examples:
  quarry cashflow
  quarry cashflow --by year --since 2020 --until 2025
  quarry cashflow --account Chequing --json
`
	var stdout, stderr bytes.Buffer

	err := executeCashFlow(t, fakeReportStore{}, &stdout, &stderr, "--help")

	require.NoError(t, err)
	assert.Contains(t, stdout.String(), examples)
}

func Test_cashflow_help_shows_each_flag(t *testing.T) {
	cases := []struct {
		flag string
		want string
	}{
		{flag: "--by", want: `--by period +group by period: month or year \(default "month"\)`},
		{
			flag: "--since",
			want: `--since date +count transactions dated on or after date \(YYYY, YYYY-MM or YYYY-MM-DD; default January 1 this year\)`,
		},
		{
			flag: "--until",
			want: `--until date +count transactions dated on or before date \(YYYY, YYYY-MM or YYYY-MM-DD; default today\)`,
		},
		{flag: "--account", want: `--account name +count only the account with this name or id; repeat for more`},
		{flag: "--currency", want: `(?m)--currency code +` + regexp.QuoteMeta(reportCurrencyHelp) + `$`},
	}

	for _, c := range cases {
		t.Run(c.flag, func(t *testing.T) {
			var stdout, stderr bytes.Buffer

			err := executeCashFlow(t, fakeReportStore{}, &stdout, &stderr, "--help")

			require.NoError(t, err)
			assert.Regexp(t, c.want, stdout.String())
		})
	}
}

const (
	reportCurrencyHelp   = "show amounts in currency code: CAD, USD, or native for each account's own (default reporting.currency in the config file, else CAD)"
	accountsCurrencyHelp = "add a column with each balance in currency code: CAD or USD; native adds none (default reporting.currency in the config file, else CAD)"
)

func Test_each_report_shows_the_currency_flag_without_a_cobra_default(t *testing.T) {
	cases := []struct {
		command string
		help    string
	}{
		{command: "spend", help: reportCurrencyHelp},
		{command: "cashflow", help: reportCurrencyHelp},
		{command: "recurring", help: reportCurrencyHelp},
		{command: "anomalies", help: reportCurrencyHelp},
		{command: "accounts", help: accountsCurrencyHelp},
	}

	for _, c := range cases {
		t.Run(c.command, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			env := cli.Env{Stdout: &stdout, Stderr: &stderr, Now: func() time.Time { return spendNow }}

			err := cli.Execute(t.Context(), []string{c.command, "--help"}, env)

			require.NoError(t, err)
			assert.Regexp(t, `(?m)--currency code +`+regexp.QuoteMeta(c.help)+`$`, stdout.String())
		})
	}
}

func Test_each_reports_window_flags_describe_what_it_does_with_them(t *testing.T) {
	const (
		countSince   = "count transactions dated on or after date (YYYY, YYYY-MM or YYYY-MM-DD; default January 1 this year)"
		countUntil   = "count transactions dated on or before date (YYYY, YYYY-MM or YYYY-MM-DD; default today)"
		countAccount = "count only the account with this name or id; repeat for more"
	)
	cases := []struct {
		command               string
		since, until, account string
	}{
		{command: "spend", since: countSince, until: countUntil, account: countAccount},
		{command: "cashflow", since: countSince, until: countUntil, account: countAccount},
		{
			command: "recurring",
			since:   "list series running on or after date (YYYY, YYYY-MM or YYYY-MM-DD; default January 1 this year)",
			until:   "list series that started on or before date (YYYY, YYYY-MM or YYYY-MM-DD; default today)",
			account: "list only series with a charge in the account with this name or id; repeat for more",
		},
		{
			command: "anomalies",
			since:   "list charges dated on or after date (YYYY, YYYY-MM or YYYY-MM-DD; default January 1 this year)",
			until:   "list charges dated on or before date (YYYY, YYYY-MM or YYYY-MM-DD; default today)",
			account: "list only charges in the account with this name or id; repeat for more",
		},
		{
			command: "search",
			since:   "list transactions dated on or after date (YYYY, YYYY-MM or YYYY-MM-DD; default the first transaction)",
			until:   "list transactions dated on or before date (YYYY, YYYY-MM or YYYY-MM-DD; default no end, future-dated included)",
			account: "search only the account with this name or id; repeat for more",
		},
	}

	for _, c := range cases {
		t.Run(c.command, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			env := cli.Env{Stdout: &stdout, Stderr: &stderr, Now: func() time.Time { return spendNow }}

			err := cli.Execute(t.Context(), []string{c.command, "--help"}, env)

			require.NoError(t, err)
			assert.Regexp(t, `(?m)--since date +`+regexp.QuoteMeta(c.since)+`$`, stdout.String())
			assert.Regexp(t, `(?m)--until date +`+regexp.QuoteMeta(c.until)+`$`, stdout.String())
			assert.Regexp(t, `(?m)--account name +`+regexp.QuoteMeta(c.account)+`$`, stdout.String())
		})
	}
}

func Test_search_help_shows_its_long_text_and_examples(t *testing.T) {
	const long = `Find transactions by text, date, account, category or amount, newest
first. The text matches payee names, transaction memos and split memos,
ignoring letter case; every character is literal, so % and _ match only
themselves. Leave the text out to search by the flags alone, or pass
nothing at all to list the newest transactions. Text that starts with -
goes after --: quarry search -- "-50% off"

Every transaction is searched, closed accounts included. Transfers
between your own accounts and transactions Quicken's reports leave out
are listed too, flagged transfer or excluded, because quarry spend and
quarry cashflow do not count them. Excluded means the transaction is
marked "exclude from reports" in Quicken, or its account is not used in
reports or uses linked account tracking. Spend also leaves out Quicken's
system categories; those are not flagged.

Amounts are in each account's own currency and are never converted.
--min and --max compare the amount without its sign, so --min 100 finds
charges and deposits of 100.00 or more; give both the same value to find
one amount. --category matches a split in that category or in any
category under it, by full path in any letter case.

Without --since and --until every date is searched, future-dated
transactions included. At most --limit transactions are printed (500
unless set); when more match, quarry says so on stderr.

Usage:
  quarry search [text] [flags]

Examples:
  quarry search costco
  quarry search --min 42.17 --max 42.17
  quarry search "e-transfer" --account Chequing --since 2026-01
  quarry search --category Food --since 2026-09 --json
`
	var stdout, stderr bytes.Buffer
	env := cli.Env{Stdout: &stdout, Stderr: &stderr}

	err := cli.Execute(t.Context(), []string{"search", "--help"}, env)

	require.NoError(t, err)
	assert.Contains(t, stdout.String(), long)
}
