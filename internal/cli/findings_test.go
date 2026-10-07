package cli_test

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/cli"
	"github.com/koblas/quarry/internal/config"
	"github.com/koblas/quarry/internal/finding"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errFindingsFactory = errors.New("open the store: disk gone")

func executeFindings(t *testing.T, fake fakeReportStore, stdout, stderr io.Writer, args ...string) error {
	t.Helper()
	return executeFindingsIgnoring(t, fake, nil, stdout, stderr, args...)
}

// executeFindingsIgnoring runs findings with ignore as the config's findings.ignore.
func executeFindingsIgnoring(t *testing.T, fake fakeReportStore, ignore []string, stdout, stderr io.Writer, args ...string) error {
	t.Helper()
	env := reportEnv(fake, stdout, stderr, withConfig(config.Config{Ignore: ignore}))
	return cli.Execute(t.Context(), append([]string{"findings"}, args...), env)
}

func Test_findings_help_says_what_findings_lists_and_how_to_ignore_one(t *testing.T) {
	const long = `List the problems sync found in the Quicken data, as a worklist to fix in
Quicken; quarry never changes the data itself. Each finding names what it
is about and what to change. After you fix them in Quicken, run quarry
sync: findings it no longer finds leave the list, and are marked fixed
unless noted below.

quarry looks for:
  duplicate             two transactions in one account with the same
                        amount, dated within 3 days of each other, unless
                        both are reconciled
  one-sided-transfer    a transfer with no matching transaction in the
                        other account
  unlinked-transfer     two transactions in different accounts of the same
                        currency that look like one transfer (opposite
                        amounts, within 3 days) but are not linked as one
  uncategorized         splits with no category, one finding per payee;
                        quarry cashflow counts them as income or spending
  mixed-categories      a payee whose transactions go back and forth
                        between categories
  payee-variants        payees whose names differ only in case,
                        punctuation, spacing, or store and reference
                        numbers
  similar-categories    categories whose names differ only in case,
                        punctuation, spacing or a plural
  unused-category       a category no transaction uses; check that no
                        scheduled transaction or budget uses it before you
                        delete it
  unclassified-account  a brokerage or retirement account listed in neither
                        accounts.registered nor accounts.non-registered in
                        the config file
  shares-without-cost   shares added to a non-registered account with no
                        cost basis, which quarry acb needs

duplicate and unlinked-transfer compare register entries only, not buys,
sells, dividends or other investment transactions.

unclassified-account is fixed in the config file, not in Quicken: it leaves
the list as soon as the account is listed there, without a sync, and is
never marked fixed.

  [accounts]
  registered = [
    "acct-12",  # Questrade TFSA
    "acct-15",  # RBC RRSP
  ]
  non-registered = ["acct-3"]  # Questrade Margin

shares-without-cost leaves the list on the first sync after the shares' cost
is entered in Quicken, and is never marked fixed.

To keep a finding off the list after checking it, add its id to
findings.ignore in ~/Library/Application Support/quarry/config.toml:

  [findings]
  ignore = ["duplicate:txn-4410+txn-4412", "uncategorized:payee-88"]

It stays ignored across syncs; remove the id to list it again. quarry never
writes that file. Without --status, only open findings are listed; --csv
prints one row per item, for a spreadsheet.
`
	var stdout, stderr bytes.Buffer

	err := executeFindings(t, fakeReportStore{}, &stdout, &stderr, "--help")

	require.NoError(t, err)
	assert.Contains(t, stdout.String(), long)
}

func Test_findings_help_shows_examples(t *testing.T) {
	const examples = `Examples:
  quarry findings
  quarry findings --type duplicate
  quarry findings --status all --csv > findings.csv
`
	var stdout, stderr bytes.Buffer

	err := executeFindings(t, fakeReportStore{}, &stdout, &stderr, "--help")

	require.NoError(t, err)
	assert.Contains(t, stdout.String(), examples)
}

func Test_root_help_lists_findings_with_its_short_line(t *testing.T) {
	var list bytes.Buffer

	err := cli.Execute(t.Context(), []string{"--help"}, cli.Env{Stdout: &list, Stderr: io.Discard})

	require.NoError(t, err)
	assert.Regexp(t, `(?m)^  findings +List what to clean up in Quicken$`, list.String())
}

func Test_findings_help_shows_each_flag(t *testing.T) {
	cases := []struct {
		flag  string
		usage string
		help  string
	}{
		{
			flag: "--status", usage: "--status status",
			help: `show only findings whose status is status: open, ignored, fixed or all (default "open")`,
		},
		{
			flag: "--type", usage: "--type type",
			help: "show only findings of this type: duplicate, one-sided-transfer, unlinked-transfer, " +
				"uncategorized, mixed-categories, payee-variants, similar-categories, unused-category, unclassified-account or shares-without-cost",
		},
		{flag: "--csv", usage: "--csv", help: "print one row per transaction, split, payee, category, account or investment transaction as CSV"},
	}

	for _, c := range cases {
		t.Run(c.flag, func(t *testing.T) {
			var stdout, stderr bytes.Buffer

			err := executeFindings(t, fakeReportStore{}, &stdout, &stderr, "--help")

			require.NoError(t, err)
			assert.Regexp(t, `(?m)^\s*`+regexp.QuoteMeta(c.usage)+` +`+regexp.QuoteMeta(c.help)+`$`, stdout.String())
		})
	}
}

func Test_findings_rejects_bad_usage(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "a positional argument", args: []string{"duplicate:txn-1+txn-2"},
			want: "findings takes no arguments; to ignore a finding add its id to findings.ignore in " +
				"~/Library/Application Support/quarry/config.toml; Run 'quarry findings --help' for usage.",
		},
		{name: "a status that is not one of the four", args: []string{"--status", "closed"}, want: "--status must be open, ignored, fixed or all"},
		{
			name: "a type that is not a finding type", args: []string{"--type", "duplicates"},
			want: "--type must be duplicate, one-sided-transfer, unlinked-transfer, uncategorized, mixed-categories, " +
				"payee-variants, similar-categories, unused-category, unclassified-account or shares-without-cost",
		},
		{
			name: "--csv with --json", args: []string{"--csv", "--json"},
			want: "--csv and --json cannot be used together; choose one output format",
		},
		{
			name: "--json with --csv", args: []string{"--json", "--csv"},
			want: "--csv and --json cannot be used together; choose one output format",
		},
		{
			name: "a positional argument beside --csv and --json", args: []string{"extra", "--csv", "--json"},
			want: "findings takes no arguments; to ignore a finding add its id to findings.ignore in " +
				"~/Library/Application Support/quarry/config.toml; Run 'quarry findings --help' for usage.",
		},
		{
			name: "--csv with --json and a bad status", args: []string{"--csv", "--json", "--status", "maybe"},
			want: "--csv and --json cannot be used together; choose one output format",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer

			err := executeFindings(t, fakeReportStore{}, &stdout, &stderr, c.args...)

			var usage cli.UsageError
			require.ErrorAs(t, err, &usage)
			assert.Equal(t, c.want, err.Error())
			assert.Empty(t, stdout.String())
		})
	}
}

func Test_findings_accepts_every_status_and_type_value(t *testing.T) {
	cases := [][]string{
		{"--status", "open"},
		{"--status", "ignored"},
		{"--status", "fixed"},
		{"--status", "all"},
		{"--type", "duplicate"},
		{"--type", "one-sided-transfer"},
		{"--type", "unlinked-transfer"},
		{"--type", "uncategorized"},
		{"--type", "mixed-categories"},
		{"--type", "payee-variants"},
		{"--type", "similar-categories"},
		{"--type", "unused-category"},
		{"--type", "unclassified-account"},
	}

	for _, args := range cases {
		t.Run(args[0]+" "+args[1], func(t *testing.T) {
			var stdout, stderr bytes.Buffer

			err := executeFindings(t, fakeReportStore{}, &stdout, &stderr, args...)

			require.NoError(t, err)
		})
	}
}

func Test_findings_with_no_flags_says_no_open_findings_for_an_empty_store(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := executeFindings(t, fakeReportStore{}, &stdout, &stderr)

	require.NoError(t, err)
	assert.Equal(t, "No open findings\n", stdout.String())
}

func Test_findings_returns_the_report_factory_error_unchanged_and_prints_nothing(t *testing.T) {
	var stdout, stderr bytes.Buffer
	var gotName string
	env := cli.Env{
		Stdout: &stdout, Stderr: &stderr,
		LoadConfig: func(string) (config.Config, error) { return config.Config{}, nil },
		NewReport: func(_ context.Context, name string) (*report.Server, error) {
			gotName = name
			return nil, errFindingsFactory
		},
	}

	err := cli.Execute(t.Context(), []string{"findings"}, env)

	require.ErrorIs(t, err, errFindingsFactory)
	assert.NotErrorAs(t, err, new(cli.UsageError))
	assert.Equal(t, "findings", gotName)
	assert.Empty(t, stdout.String())
}

// filterFakeFindings is an open uncategorized payee and a fixed duplicate pair, enough for each filter to select a different set.
func filterFakeFindings() fakeReportStore {
	foundAt := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	fixedAt := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	return fakeReportStore{findings: store.FindingList{Findings: []store.Finding{
		{
			ID: "uncategorized:payee-7", Type: finding.Uncategorized, FirstFoundAt: foundAt,
			Items: []store.FindingItem{{TransactionID: new("txn-7"), SplitID: new("split-7"), Account: "Visa", Currency: "CAD", Active: true, Payee: "Amazon", Date: foundAt, Amount: -1000}},
		},
		{ID: "duplicate:txn-1+txn-2", Type: finding.Duplicate, FirstFoundAt: foundAt, FixedAt: &fixedAt},
	}}}
}

func Test_findings_passes_status_and_type_on_to_the_listing(t *testing.T) {
	cli.UseZone(t, time.UTC)
	cases := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "no flags lists the open finding with the hint", args: nil,
			want: "Uncategorized (1 payee, 1 split): give each payee's splits a category in Quicken\n" +
				"  uncategorized:payee-7  Amazon  1 split  2026-09-01\n\n" +
				"1 open finding; 1 fixed not shown (--status all)\n" +
				"Ignore a finding by adding its id to findings.ignore in ~/Library/Application Support/quarry/config.toml; see quarry findings --help\n",
		},
		{name: "status fixed", args: []string{"--status", "fixed"}, want: "Possible duplicates (1)\n  duplicate:txn-1+txn-2  fixed 2026-09-20\n\n1 fixed finding\n"},
		{name: "status ignored finds none", args: []string{"--status", "ignored"}, want: "No ignored findings\n"},
		{name: "type duplicate lists no open one and counts only duplicates", args: []string{"--type", "duplicate"}, want: "No open findings of type duplicate; 1 fixed not shown (--status all)\n"},
		{name: "status fixed of a type with none fixed", args: []string{"--status", "fixed", "--type", "uncategorized"}, want: "No fixed findings of type uncategorized\n"},
		{
			name: "status all type duplicate", args: []string{"--status", "all", "--type", "duplicate"},
			want: "Possible duplicates (1)\n  duplicate:txn-1+txn-2  fixed 2026-09-20\n\n1 finding: 1 fixed\n",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer

			err := executeFindings(t, filterFakeFindings(), &stdout, &stderr, c.args...)

			require.NoError(t, err)
			assert.Equal(t, c.want, stdout.String())
		})
	}
}

func Test_findings_json_carries_the_status_and_type_flags_into_the_document(t *testing.T) {
	cases := []struct {
		name       string
		args       []string
		wantStatus string
		wantType   any
	}{
		{name: "defaults", args: nil, wantStatus: "open", wantType: nil},
		{name: "status all", args: []string{"--status", "all"}, wantStatus: "all", wantType: nil},
		{name: "status ignored and a type", args: []string{"--status", "ignored", "--type", "duplicate"}, wantStatus: "ignored", wantType: "duplicate"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer

			err := executeFindings(t, filterFakeFindings(), &stdout, &stderr, append([]string{"--json"}, c.args...)...)

			require.NoError(t, err)
			var doc map[string]any
			require.NoError(t, json.Unmarshal(stdout.Bytes(), &doc))
			assert.Equal(t, c.wantStatus, doc["status"])
			assert.Equal(t, c.wantType, doc["type"])
		})
	}
}

const findingsCSVHeader = "finding_id,type,status,date,account,currency,payee,category,amount,other_account,transactions,splits,transaction_id,split_id,payee_id,category_id,fix," +
	"investment_transaction_id,security_id,security,shares\n"

const (
	duplicateFixCSV     = `"Delete the extra one in Quicken, or ignore the pair if both are real"`
	oneSidedFixCSV      = `"Re-enter it as a transfer between the two accounts in Quicken, or ignore it if the other account is not in this file"`
	uncategorizedFixCSV = "Give this payee's splits a category in Quicken"
)

func csvFindingDay(day int) time.Time { return time.Date(2026, time.August, day, 0, 0, 0, 0, time.UTC) }

func Test_findings_csv_prints_one_row_per_item_with_a_quoted_payee_and_the_leg_fields_of_a_one_sided_transfer(t *testing.T) {
	legTxn, legSplit, other := "txn-9", "split-9", "Savings"
	dupFirst, dupSecond := "txn-1", "txn-2"
	fake := fakeReportStore{findings: store.FindingList{Findings: []store.Finding{
		{
			ID: "duplicate:txn-1+txn-2", Type: finding.Duplicate, FirstFoundAt: csvFindingDay(1),
			Items: []store.FindingItem{
				{TransactionID: &dupFirst, Date: csvFindingDay(3), Account: "Chequing", Currency: "CAD", Payee: `Smith, "Jo"`, Amount: -14217},
				{TransactionID: &dupSecond, Date: csvFindingDay(5), Account: "Chequing", Currency: "CAD", Payee: "Hydro One", Amount: -14217},
			},
		},
		{
			ID: "one-sided-transfer:xfer-9", Type: finding.OneSidedTransfer, FirstFoundAt: csvFindingDay(1),
			Items: []store.FindingItem{{
				TransactionID: &legTxn, SplitID: &legSplit, Date: csvFindingDay(1), Account: "Visa", Currency: "USD",
				Payee: "Payment", Amount: -120050, OtherAccount: &other,
			}},
		},
	}}}
	var stdout, stderr bytes.Buffer

	err := executeFindings(t, fake, &stdout, &stderr, "--csv")

	require.NoError(t, err)
	assert.Equal(t, findingsCSVHeader+
		`duplicate:txn-1+txn-2,duplicate,open,2026-08-03,Chequing,CAD,"Smith, ""Jo""",,-142.17,,,,txn-1,,,,`+duplicateFixCSV+",,,,\n"+
		`duplicate:txn-1+txn-2,duplicate,open,2026-08-05,Chequing,CAD,Hydro One,,-142.17,,,,txn-2,,,,`+duplicateFixCSV+",,,,\n"+
		`one-sided-transfer:xfer-9,one-sided-transfer,open,2026-08-01,Visa,USD,Payment,,-1200.50,Savings,,,txn-9,split-9,,,`+oneSidedFixCSV+",,,,\n",
		stdout.String())
	assert.Empty(t, stderr.String())
}

func Test_findings_csv_leaves_the_payee_of_a_no_payee_item_as_an_empty_field_not_an_empty_string(t *testing.T) {
	txn, split := "txn-4", "split-4"
	fake := fakeReportStore{findings: store.FindingList{Findings: []store.Finding{{
		ID: "uncategorized:no-payee", Type: finding.Uncategorized, FirstFoundAt: csvFindingDay(1),
		Items: []store.FindingItem{{TransactionID: &txn, SplitID: &split, Date: csvFindingDay(2), Account: "Visa", Currency: "CAD", Amount: -1000}},
	}}}}
	var stdout bytes.Buffer

	err := executeFindings(t, fake, &stdout, &bytes.Buffer{}, "--csv")

	require.NoError(t, err)
	assert.Equal(t, findingsCSVHeader+
		"uncategorized:no-payee,uncategorized,open,2026-08-02,Visa,CAD,,,-10.00,,,,txn-4,split-4,,,"+uncategorizedFixCSV+",,,,\n", stdout.String())
}

func Test_findings_csv_applies_the_status_and_type_filters(t *testing.T) {
	const uncategorizedRow = "uncategorized:payee-7,uncategorized,open,2026-09-01,Visa,CAD,Amazon,,-10.00,,,,txn-7,split-7,,," + uncategorizedFixCSV + ",,,,\n"
	const fixedRow = "duplicate:txn-1+txn-2,duplicate,fixed,,,,,,,,,,,,,," + duplicateFixCSV + ",,,,\n"
	const ignoredRow = "one-sided-transfer:xfer-3,one-sided-transfer,ignored,2026-08-02,Visa,USD,Payment,,-5.00,Savings,,,txn-3,split-3,,," + oneSidedFixCSV + ",,,,\n"
	cases := []struct {
		name string
		args []string
		want string
	}{
		{name: "the default view lists open findings only", args: []string{"--csv"}, want: findingsCSVHeader + uncategorizedRow},
		{name: "status all lists every finding", args: []string{"--csv", "--status", "all"}, want: findingsCSVHeader + fixedRow + ignoredRow + uncategorizedRow},
		{name: "status ignored lists the ignored one", args: []string{"--csv", "--status", "ignored"}, want: findingsCSVHeader + ignoredRow},
		{name: "type duplicate with status fixed lists the fixed one", args: []string{"--csv", "--status", "fixed", "--type", "duplicate"}, want: findingsCSVHeader + fixedRow},
		{name: "type duplicate with none open prints the header alone", args: []string{"--csv", "--type", "duplicate"}, want: findingsCSVHeader},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fake := filterFakeFindings()
			txn, split, other := "txn-3", "split-3", "Savings"
			fake.findings.Findings = append(fake.findings.Findings, store.Finding{
				ID: "one-sided-transfer:xfer-3", Type: finding.OneSidedTransfer, FirstFoundAt: csvFindingDay(1),
				Items: []store.FindingItem{{TransactionID: &txn, SplitID: &split, Date: csvFindingDay(2), Account: "Visa", Currency: "USD", Payee: "Payment", Amount: -500, OtherAccount: &other}},
			})
			var stdout, stderr bytes.Buffer

			err := executeFindingsIgnoring(t, fake, []string{"one-sided-transfer:xfer-3"}, &stdout, &stderr, c.args...)

			require.NoError(t, err)
			assert.Equal(t, c.want, stdout.String())
			assert.Empty(t, stderr.String())
		})
	}
}

func Test_findings_csv_puts_the_payee_and_category_ids_in_their_own_columns(t *testing.T) {
	txn, payeeID, categoryID := "txn-7", "payee-7", "cat-3"
	fake := fakeReportStore{findings: store.FindingList{Findings: []store.Finding{{
		ID: "uncategorized:payee-7", Type: finding.Uncategorized, FirstFoundAt: csvFindingDay(1),
		Items: []store.FindingItem{{TransactionID: &txn, PayeeID: &payeeID, CategoryID: &categoryID, Date: csvFindingDay(2), Account: "Visa", Currency: "CAD", Payee: "Amazon", Amount: -1000}},
	}}}}
	var stdout bytes.Buffer

	err := executeFindings(t, fake, &stdout, &bytes.Buffer{}, "--csv")

	require.NoError(t, err)
	assert.Equal(t, findingsCSVHeader+
		"uncategorized:payee-7,uncategorized,open,2026-08-02,Visa,CAD,Amazon,,-10.00,,,,txn-7,,payee-7,cat-3,"+uncategorizedFixCSV+",,,,\n", stdout.String())
}

func Test_findings_csv_prints_nothing_when_the_listing_fails(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := executeFindings(t, fakeReportStore{err: errStoreRead}, &stdout, &stderr, "--csv")

	require.ErrorIs(t, err, errStoreRead)
	assert.Empty(t, stdout.String())
}

func Test_findings_csv_prints_the_header_alone_when_no_finding_is_listed(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := executeFindings(t, fakeReportStore{}, &stdout, &stderr, "--csv", "--status", "all")

	require.NoError(t, err)
	assert.Equal(t, findingsCSVHeader, stdout.String())
	assert.Empty(t, stderr.String())
}

func Test_findings_csv_keeps_config_warnings_on_stderr_and_out_of_stdout(t *testing.T) {
	var stdout, stderr bytes.Buffer
	env := reportEnv(fakeReportStore{}, &stdout, &stderr, withConfig(config.Config{Warnings: []string{"config.toml: unknown key findings.ignored"}}))

	err := cli.Execute(t.Context(), []string{"findings", "--csv"}, env)

	require.NoError(t, err)
	assert.Equal(t, findingsCSVHeader, stdout.String())
	assert.Equal(t, "quarry: warning: config.toml: unknown key findings.ignored\n", stderr.String())
}

func Test_findings_csv_refuses_a_failed_stdout_write(t *testing.T) {
	err := executeFindings(t, filterFakeFindings(), failingWriter{err: errNoSpace}, &bytes.Buffer{}, "--csv")

	require.EqualError(t, err, "cannot write the result to stdout: write /dev/stdout: no space left on device")
}

const mixedFixCSV = `"Pick one category for this payee's transactions in Quicken, or ignore it if the mix is intended"`

const variantsFixCSV = `"Rename these payees to one in Quicken and add a renaming rule, or ignore the group if they are different merchants"`

const unlinkedFixCSV = `"Make the pair one transfer between the two accounts in Quicken, or ignore it if no money moved between your accounts"`

// unlinkedFake is one unlinked-transfer finding of first and second, both in Chequing (CAD) with payee Rent.
func unlinkedFake(first, second store.FindingItem) fakeReportStore {
	first.TransactionID, second.TransactionID = new("txn-1"), new("txn-2")
	for _, item := range []*store.FindingItem{&first, &second} {
		item.Account, item.Currency, item.Payee, item.Amount = "Chequing", "CAD", "Rent", -1000
	}
	first.Date, second.Date = csvFindingDay(2), csvFindingDay(3)
	return fakeReportStore{findings: store.FindingList{Findings: []store.Finding{{
		ID: "unlinked-transfer:txn-1+txn-2", Type: finding.UnlinkedTransfer, FirstFoundAt: csvFindingDay(1), Items: []store.FindingItem{first, second},
	}}}}
}

func Test_findings_csv_puts_the_category_path_of_an_unlinked_transfer_item_in_its_category_column(t *testing.T) {
	fake := unlinkedFake(store.FindingItem{Category: new("Income:Other"), Splits: 1}, store.FindingItem{Category: new("Bills, fixed"), Splits: 1})
	var stdout bytes.Buffer

	err := executeFindings(t, fake, &stdout, &bytes.Buffer{}, "--csv")

	require.NoError(t, err)
	assert.Equal(t, findingsCSVHeader+
		"unlinked-transfer:txn-1+txn-2,unlinked-transfer,open,2026-08-02,Chequing,CAD,Rent,Income:Other,-10.00,,,,txn-1,,,,"+unlinkedFixCSV+",,,,\n"+
		`unlinked-transfer:txn-1+txn-2,unlinked-transfer,open,2026-08-03,Chequing,CAD,Rent,"Bills, fixed",-10.00,,,,txn-2,,,,`+unlinkedFixCSV+",,,,\n",
		stdout.String())
}

func Test_findings_csv_leaves_the_category_of_a_split_or_uncategorized_unlinked_transfer_item_an_empty_field_not_an_empty_string(t *testing.T) {
	fake := unlinkedFake(store.FindingItem{Splits: 2}, store.FindingItem{Splits: 1})
	var stdout bytes.Buffer

	err := executeFindings(t, fake, &stdout, &bytes.Buffer{}, "--csv")

	require.NoError(t, err)
	assert.Equal(t, findingsCSVHeader+
		"unlinked-transfer:txn-1+txn-2,unlinked-transfer,open,2026-08-02,Chequing,CAD,Rent,,-10.00,,,,txn-1,,,,"+unlinkedFixCSV+",,,,\n"+
		"unlinked-transfer:txn-1+txn-2,unlinked-transfer,open,2026-08-03,Chequing,CAD,Rent,,-10.00,,,,txn-2,,,,"+unlinkedFixCSV+",,,,\n",
		stdout.String())
}

func Test_findings_csv_puts_a_mixed_categories_item_in_its_payee_category_and_count_columns_with_the_transaction_cells_empty(t *testing.T) {
	fake := fakeReportStore{findings: store.FindingList{Findings: []store.Finding{{
		ID: "mixed-categories:payee-12", Type: finding.MixedCategories, FirstFoundAt: csvFindingDay(1),
		Items: []store.FindingItem{{PayeeID: new("payee-12"), CategoryID: new("cat-3"), Payee: "Costco", Category: new("Groceries"), Transactions: 30}},
	}}}}
	var stdout bytes.Buffer

	err := executeFindings(t, fake, &stdout, &bytes.Buffer{}, "--csv")

	require.NoError(t, err)
	assert.Equal(t, findingsCSVHeader+
		"mixed-categories:payee-12,mixed-categories,open,,,,Costco,Groceries,,,30,,,,payee-12,cat-3,"+mixedFixCSV+",,,,\n", stdout.String())
}

func Test_findings_csv_puts_a_payee_variants_item_in_its_payee_and_count_columns_with_the_transaction_cells_empty(t *testing.T) {
	fake := fakeReportStore{findings: store.FindingList{Findings: []store.Finding{{
		ID: "payee-variants:tim-hortons", Type: finding.PayeeVariants, FirstFoundAt: csvFindingDay(1),
		Items: []store.FindingItem{{PayeeID: new("payee-12"), Payee: "TIM HORTONS #1234", Transactions: 212}},
	}}}}
	var stdout bytes.Buffer

	err := executeFindings(t, fake, &stdout, &bytes.Buffer{}, "--csv")

	require.NoError(t, err)
	assert.Equal(t, findingsCSVHeader+
		"payee-variants:tim-hortons,payee-variants,open,,,,TIM HORTONS #1234,,,,212,,,,payee-12,,"+variantsFixCSV+",,,,\n", stdout.String())
}

const similarFixCSV = `"Merge these categories into one in Quicken, or ignore the group if they mean different things"`

func Test_findings_csv_puts_a_similar_categories_item_in_its_category_and_splits_columns_with_a_zero_count_as_0(t *testing.T) {
	fake := fakeReportStore{findings: store.FindingList{Findings: []store.Finding{{
		ID: "similar-categories:grocery", Type: finding.SimilarCategories, FirstFoundAt: csvFindingDay(1),
		Items: []store.FindingItem{
			{CategoryID: new("cat-3"), Category: new("Groceries"), Splits: 812},
			{CategoryID: new("cat-9"), Category: new("Grocery"), Splits: 0},
		},
	}}}}
	var stdout bytes.Buffer

	err := executeFindings(t, fake, &stdout, &bytes.Buffer{}, "--csv")

	require.NoError(t, err)
	assert.Equal(t, findingsCSVHeader+
		"similar-categories:grocery,similar-categories,open,,,,,Groceries,,,,812,,,,cat-3,"+similarFixCSV+",,,,\n"+
		"similar-categories:grocery,similar-categories,open,,,,,Grocery,,,,0,,,,cat-9,"+similarFixCSV+",,,,\n", stdout.String())
}

const unusedFixCSV = `"No transaction uses it; check that no scheduled transaction or budget does, then delete it in Quicken, or ignore it to keep it"`

func Test_findings_csv_puts_an_unused_category_item_in_its_category_columns_with_everything_else_empty(t *testing.T) {
	fake := fakeReportStore{findings: store.FindingList{Findings: []store.Finding{{
		ID: "unused-category:cat-40", Type: finding.UnusedCategory, FirstFoundAt: csvFindingDay(1),
		Items: []store.FindingItem{
			{CategoryID: new("cat-40"), Category: new("Vacation")},
			{CategoryID: new("cat-41"), Category: new("Vacation:Hotel")},
		},
	}}}}
	var stdout bytes.Buffer

	err := executeFindings(t, fake, &stdout, &bytes.Buffer{}, "--csv")

	require.NoError(t, err)
	assert.Equal(t, findingsCSVHeader+
		"unused-category:cat-40,unused-category,open,,,,,Vacation,,,,,,,,cat-40,"+unusedFixCSV+",,,,\n"+
		"unused-category:cat-40,unused-category,open,,,,,Vacation:Hotel,,,,,,,,cat-41,"+unusedFixCSV+",,,,\n", stdout.String())
}

func Test_findings_csv_puts_an_unclassified_account_in_its_account_and_currency_columns_with_everything_else_empty(t *testing.T) {
	fake := fakeReportStore{findings: store.FindingList{Accounts: []store.Account{
		{ID: "acct-12", Name: "Questrade TFSA", Type: store.AccountTypeBrokerage, Currency: "CAD"},
		{ID: "acct-31", Name: "Old RRSP", Type: store.AccountTypeRetirement, Currency: "USD", Closed: true},
	}}}
	var stdout bytes.Buffer

	err := executeFindings(t, fake, &stdout, &bytes.Buffer{}, "--csv")

	require.NoError(t, err)
	rows, err := csv.NewReader(strings.NewReader(stdout.String())).ReadAll()
	require.NoError(t, err)
	fix := finding.UnclassifiedAccount.Fix().Sentence
	assert.Equal(t, [][]string{
		strings.Split(strings.TrimSuffix(findingsCSVHeader, "\n"), ","),
		{"unclassified-account:acct-31", "unclassified-account", "open", "", "Old RRSP", "USD", "", "", "", "", "", "", "", "", "", "", fix, "", "", "", ""},
		{"unclassified-account:acct-12", "unclassified-account", "open", "", "Questrade TFSA", "CAD", "", "", "", "", "", "", "", "", "", "", fix, "", "", "", ""},
	}, rows)
}

func Test_findings_csv_gives_every_record_the_header_width_with_the_investment_cells_empty_but_for_shares_without_cost(t *testing.T) {
	txn := "txn-1"
	fixedAt := csvFindingDay(9)
	fake := fakeReportStore{findings: store.FindingList{
		Findings: []store.Finding{
			{ID: "duplicate:txn-1+txn-2", Type: finding.Duplicate, FirstFoundAt: csvFindingDay(1), FixedAt: &fixedAt},
			{
				ID: "uncategorized:no-payee", Type: finding.Uncategorized, FirstFoundAt: csvFindingDay(1),
				Items: []store.FindingItem{{TransactionID: &txn, Date: csvFindingDay(2), Account: "Visa", Currency: "CAD", Amount: -1000}},
			},
		},
		Accounts: []store.Account{{ID: "acct-1", Name: "Margin, USD", Type: store.AccountTypeBrokerage, Currency: "USD"}},
		Investments: store.Investments{
			Securities: []store.Security{{ID: "sec-4", Name: `XEQT "core"`}},
			Transactions: []store.InvestmentTransaction{{
				ID: "itxn-7", AccountID: "acct-1", SecurityID: new("sec-4"), Date: csvFindingDay(3),
				Action: store.ActionAddShares, Shares: new(int64(1_500_000)), Currency: "CAD",
			}},
		},
	}}
	var stdout bytes.Buffer
	env := reportEnv(fake, &stdout, &bytes.Buffer{}, withConfig(config.Config{NonRegistered: []string{"acct-1"}}))

	err := cli.Execute(t.Context(), []string{"findings", "--csv", "--status", "all"}, env)

	require.NoError(t, err)
	rows, err := csv.NewReader(strings.NewReader(stdout.String())).ReadAll()
	require.NoError(t, err)
	require.Len(t, rows, 4)
	width := len(strings.Split(strings.TrimSuffix(findingsCSVHeader, "\n"), ","))
	for _, row := range rows {
		assert.Len(t, row, width)
	}
	investment := func(row []string) []string { return row[width-4:] }
	assert.Equal(t, []string{"investment_transaction_id", "security_id", "security", "shares"}, investment(rows[0]))
	assert.Equal(t, []string{"", "", "", ""}, investment(rows[1]), "fixed finding")
	assert.Equal(t, []string{"", "", "", ""}, investment(rows[2]), "uncategorized item")
	assert.Equal(t, []string{"itxn-7", "sec-4", `XEQT "core"`, "1.500000"}, investment(rows[3]))
	assert.Equal(t, []string{"2026-08-03", "Margin, USD", "USD"}, rows[3][3:6])
}
