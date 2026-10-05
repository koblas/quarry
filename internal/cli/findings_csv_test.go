package cli_test

import (
	"bytes"
	"context"
	"encoding/csv"
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

const findingsCSVHeader = "finding_id,type,status,date,account,currency,payee,category,amount,other_account,transactions,splits,transaction_id,split_id,payee_id,category_id,fix\n"

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
		`duplicate:txn-1+txn-2,duplicate,open,2026-08-03,Chequing,CAD,"Smith, ""Jo""",,-142.17,,,,txn-1,,,,`+duplicateFixCSV+"\n"+
		`duplicate:txn-1+txn-2,duplicate,open,2026-08-05,Chequing,CAD,Hydro One,,-142.17,,,,txn-2,,,,`+duplicateFixCSV+"\n"+
		`one-sided-transfer:xfer-9,one-sided-transfer,open,2026-08-01,Visa,USD,Payment,,-1200.50,Savings,,,txn-9,split-9,,,`+oneSidedFixCSV+"\n",
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
		"uncategorized:no-payee,uncategorized,open,2026-08-02,Visa,CAD,,,-10.00,,,,txn-4,split-4,,,"+uncategorizedFixCSV+"\n", stdout.String())
}

func Test_findings_csv_applies_the_status_and_type_filters(t *testing.T) {
	const uncategorizedRow = "uncategorized:payee-7,uncategorized,open,2026-09-01,Visa,CAD,Amazon,,-10.00,,,,txn-7,split-7,,," + uncategorizedFixCSV + "\n"
	const fixedRow = "duplicate:txn-1+txn-2,duplicate,fixed,,,,,,,,,,,,,," + duplicateFixCSV + "\n"
	const ignoredRow = "one-sided-transfer:xfer-3,one-sided-transfer,ignored,2026-08-02,Visa,USD,Payment,,-5.00,Savings,,,txn-3,split-3,,," + oneSidedFixCSV + "\n"
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
		"uncategorized:payee-7,uncategorized,open,2026-08-02,Visa,CAD,Amazon,,-10.00,,,,txn-7,,payee-7,cat-3,"+uncategorizedFixCSV+"\n", stdout.String())
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
	env := cli.Env{
		Stdout: &stdout, Stderr: &stderr,
		LoadConfig: func(string) (config.Config, error) {
			return config.Config{Warnings: []string{"config.toml: unknown key findings.ignored"}}, nil
		},
		NewReport: func(context.Context, string) (*report.Server, error) {
			return report.NewServer(report.WithStore(fakeReportStore{})), nil
		},
	}

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
		"unlinked-transfer:txn-1+txn-2,unlinked-transfer,open,2026-08-02,Chequing,CAD,Rent,Income:Other,-10.00,,,,txn-1,,,,"+unlinkedFixCSV+"\n"+
		`unlinked-transfer:txn-1+txn-2,unlinked-transfer,open,2026-08-03,Chequing,CAD,Rent,"Bills, fixed",-10.00,,,,txn-2,,,,`+unlinkedFixCSV+"\n",
		stdout.String())
}

func Test_findings_csv_leaves_the_category_of_a_split_or_uncategorized_unlinked_transfer_item_an_empty_field_not_an_empty_string(t *testing.T) {
	fake := unlinkedFake(store.FindingItem{Splits: 2}, store.FindingItem{Splits: 1})
	var stdout bytes.Buffer

	err := executeFindings(t, fake, &stdout, &bytes.Buffer{}, "--csv")

	require.NoError(t, err)
	assert.Equal(t, findingsCSVHeader+
		"unlinked-transfer:txn-1+txn-2,unlinked-transfer,open,2026-08-02,Chequing,CAD,Rent,,-10.00,,,,txn-1,,,,"+unlinkedFixCSV+"\n"+
		"unlinked-transfer:txn-1+txn-2,unlinked-transfer,open,2026-08-03,Chequing,CAD,Rent,,-10.00,,,,txn-2,,,,"+unlinkedFixCSV+"\n",
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
		"mixed-categories:payee-12,mixed-categories,open,,,,Costco,Groceries,,,30,,,,payee-12,cat-3,"+mixedFixCSV+"\n", stdout.String())
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
		"payee-variants:tim-hortons,payee-variants,open,,,,TIM HORTONS #1234,,,,212,,,,payee-12,,"+variantsFixCSV+"\n", stdout.String())
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
		"similar-categories:grocery,similar-categories,open,,,,,Groceries,,,,812,,,,cat-3,"+similarFixCSV+"\n"+
		"similar-categories:grocery,similar-categories,open,,,,,Grocery,,,,0,,,,cat-9,"+similarFixCSV+"\n", stdout.String())
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
		"unused-category:cat-40,unused-category,open,,,,,Vacation,,,,,,,,cat-40,"+unusedFixCSV+"\n"+
		"unused-category:cat-40,unused-category,open,,,,,Vacation:Hotel,,,,,,,,cat-41,"+unusedFixCSV+"\n", stdout.String())
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
		{"unclassified-account:acct-31", "unclassified-account", "open", "", "Old RRSP", "USD", "", "", "", "", "", "", "", "", "", "", fix},
		{"unclassified-account:acct-12", "unclassified-account", "open", "", "Questrade TFSA", "CAD", "", "", "", "", "", "", "", "", "", "", fix},
	}, rows)
}
