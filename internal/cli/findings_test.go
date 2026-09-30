package cli_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"regexp"
	"testing"

	"github.com/koblas/quarry/internal/cli"
	"github.com/koblas/quarry/internal/config"
	"github.com/koblas/quarry/internal/report"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errFindingsFactory = errors.New("open the store: disk gone")

func executeFindings(t *testing.T, fake fakeReportStore, stdout, stderr io.Writer, args ...string) error {
	t.Helper()
	env := cli.Env{
		Stdout: stdout, Stderr: stderr,
		LoadConfig: func(string) (config.Config, error) { return config.Config{}, nil },
		NewReport: func(context.Context, string) (*report.Server, error) {
			return report.NewServer(report.WithStore(fake)), nil
		},
	}
	return cli.Execute(t.Context(), append([]string{"findings"}, args...), env)
}

func Test_findings_help_says_what_findings_lists_and_how_to_ignore_one(t *testing.T) {
	const long = `List the problems sync found in the Quicken data, as a worklist to fix in
Quicken; quarry never changes the data itself. Each finding names what it
is about and what to change. After you fix them in Quicken, run quarry
sync: findings it no longer finds are marked fixed.

quarry looks for:
  duplicate           two transactions in one account with the same amount,
                      dated within 3 days of each other, unless both are
                      reconciled
  one-sided-transfer  a transfer with no matching transaction in the other
                      account
  unlinked-transfer   two transactions in different accounts of the same
                      currency that look like one transfer (opposite
                      amounts, within 3 days) but are not linked as one
  uncategorized       splits with no category, one finding per payee;
                      quarry cashflow counts them as income or spending
  mixed-categories    a payee whose transactions go back and forth between
                      categories
  payee-variants      payees whose names differ only in case, punctuation,
                      spacing, or store and reference numbers
  similar-categories  categories whose names differ only in case,
                      punctuation, spacing or a plural
  unused-category     a category no transaction uses; check that no
                      scheduled transaction or budget uses it before you
                      delete it

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

func Test_findings_help_shows_its_short_line_and_examples(t *testing.T) {
	const examples = `Examples:
  quarry findings
  quarry findings --type duplicate
  quarry findings --status all --csv > findings.csv
`
	var stdout, stderr bytes.Buffer

	err := executeFindings(t, fakeReportStore{}, &stdout, &stderr, "--help")
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), examples)

	var list bytes.Buffer
	require.NoError(t, cli.Execute(t.Context(), []string{"--help"}, cli.Env{Stdout: &list, Stderr: io.Discard}))
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
				"uncategorized, mixed-categories, payee-variants, similar-categories or unused-category",
		},
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
				"payee-variants, similar-categories or unused-category",
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
	}

	for _, args := range cases {
		t.Run(args[0]+" "+args[1], func(t *testing.T) {
			var stdout, stderr bytes.Buffer

			err := executeFindings(t, fakeReportStore{}, &stdout, &stderr, args...)

			require.NoError(t, err)
			assert.Equal(t, "No open findings\n", stdout.String())
		})
	}
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
