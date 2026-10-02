// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// syncStatusFindingsFixture syncs a file raising one duplicate pair and three
// uncategorized payees under home, returning the duplicate's id.
func syncStatusFindingsFixture(t *testing.T, home string) string {
	t.Helper()
	b := v9fixture.NewBuilder()
	chequingPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	hydroPK := b.Payee(v9fixture.PayeeRow{Name: "Hydro One"})
	hydroNetworksPK := b.Payee(v9fixture.PayeeRow{Name: "HYDRO ONE NETWORKS"})
	billsPK := b.Category(v9fixture.TagRow{Name: "Bills", Type: new(int64(1))})
	first := categorizedPayeeTxn(b, chequingPK, hydroPK, billsPK, time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC), "-142.17")
	second := categorizedPayeeTxn(b, chequingPK, hydroNetworksPK, billsPK, time.Date(2026, 8, 5, 0, 0, 0, 0, time.UTC), "-142.17")
	for i, payee := range []string{"Amazon", "Costco", "Shell"} {
		amount := fmt.Sprintf("-%d0.00", i+1)
		day := time.Date(2026, 3, 1+i, 0, 0, 0, 0, time.UTC)
		txn := b.Transaction(v9fixture.TransactionRow{Account: chequingPK, Amount: amount, PostedDate: &day, Payee: b.Payee(v9fixture.PayeeRow{Name: payee})})
		b.Entry(v9fixture.EntryRow{Parent: txn, Amount: amount})
	}
	syncBundle(t, b.WriteBundle(t, filepath.Join(home, "Documents")))
	return fmt.Sprintf("duplicate:txn-%d+txn-%d", first, second)
}

func Test_run_status_shows_the_findings_line_with_open_and_ignored_counts(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	duplicate := syncStatusFindingsFixture(t, home)
	writeConfig(t, home, fmt.Sprintf("[findings]\nignore = [%q]\n", duplicate))
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"status"}, &stdout, &stderr)

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Contains(t, stdout.String(), "\nFindings  3 open, 1 ignored; run quarry findings to list them\n")
}

func Test_run_status_warns_on_a_bad_config_and_still_reports(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	duplicate := syncStatusFindingsFixture(t, home)
	writeConfig(t, home, fmt.Sprintf("[snapshots]\nkeep = 0\n[findings]\nignore = [%q]\n", duplicate))
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"status"}, &stdout, &stderr)

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Equal(t, "quarry: warning: cannot tell which findings you ignored: "+configShown+
		": snapshots.keep must be a whole number of 1 or more, got 0; findings you ignored are counted as open\n", stderr.String())
	assert.Contains(t, stdout.String(), "\nFindings  4 open; run quarry findings to list them\n")
}

const (
	statusIgnoreWarningLead = "cannot tell which findings you ignored: "
	statusIgnoreWarningTail = "; findings you ignored are counted as open"
)

// statusConfigRefusal is a config that cannot be loaded and the problem the status warning names.
type statusConfigRefusal struct {
	name    string
	setup   func(t *testing.T, home string)
	problem string
}

func statusConfigRefusals() []statusConfigRefusal {
	writing := func(content string) func(t *testing.T, home string) {
		return func(t *testing.T, home string) {
			t.Helper()
			writeConfig(t, home, content)
		}
	}
	return []statusConfigRefusal{
		{name: "TOML syntax error", setup: writing("[snapshots\nkeep = 24\n"), problem: "cannot read " + configShown + ": line 1: expected ']' to close table name"},
		{
			name: "snapshots.keep below 1", setup: writing("[snapshots]\nkeep = 0\n"),
			problem: configShown + ": snapshots.keep must be a whole number of 1 or more, got 0",
		},
		{
			name: "reporting.currency another currency", setup: writing("[reporting]\ncurrency = \"EUR\"\n"),
			problem: configShown + `: reporting.currency must be CAD, USD or native, got "EUR"`,
		},
		{
			name: "reporting as a plain value", setup: writing("reporting = \"CAD\"\n"),
			problem: configShown + `: reporting must be a table, such as reporting.currency = "CAD", got "CAD"`,
		},
		{
			name: "findings.ignore not a list", setup: writing("[findings]\nignore = \"x\"\n"),
			problem: configShown + `: findings.ignore must be a list of finding ids in quotes, such as ["duplicate:txn-4410+txn-4412"], got "x"`,
		},
		{name: "config.toml a directory", setup: func(t *testing.T, home string) {
			t.Helper()
			require.NoError(t, os.MkdirAll(filepath.Join(storeDirUnder(home), "config.toml"), 0o700))
		}, problem: "cannot read " + configShown + ": is a directory"},
	}
}

func Test_run_status_warns_once_and_counts_every_finding_open_for_each_kind_of_bad_config(t *testing.T) {
	for _, c := range statusConfigRefusals() {
		t.Run(c.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			syncStatusFindingsFixture(t, home)
			c.setup(t, home)
			var stdout, stderr bytes.Buffer

			exitCode := run(context.Background(), []string{"status"}, &stdout, &stderr)

			require.Equal(t, 0, exitCode, stderr.String())
			assert.Equal(t, "quarry: warning: "+statusIgnoreWarningLead+c.problem+statusIgnoreWarningTail+"\n", stderr.String())
			assert.Contains(t, stdout.String(), "\nFindings  4 open; run quarry findings to list them\n")
		})
	}
}

// statusFindingsJSON is the part of status --json these tests read.
type statusFindingsJSON struct {
	Findings struct {
		Open       int  `json:"open"`
		Ignored    *int `json:"ignored"`
		Fixed      int  `json:"fixed"`
		New        int  `json:"new"`
		NewlyFixed int  `json:"newly_fixed"`
	} `json:"findings"`
	Warnings []string `json:"warnings"`
}

func Test_run_status_json_carries_the_config_warning_unprefixed_and_a_null_ignored_count_for_each_kind_of_bad_config(t *testing.T) {
	for _, c := range statusConfigRefusals() {
		t.Run(c.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			syncStatusFindingsFixture(t, home)
			c.setup(t, home)
			var stdout, stderr bytes.Buffer

			exitCode := run(context.Background(), []string{"status", "--json"}, &stdout, &stderr)

			require.Equal(t, 0, exitCode, stderr.String())
			var got statusFindingsJSON
			require.NoError(t, json.Unmarshal(stdout.Bytes(), &got))
			assert.Nil(t, got.Findings.Ignored)
			assert.Equal(t, 4, got.Findings.Open)
			assert.Equal(t, []string{statusIgnoreWarningLead + strings.ReplaceAll(c.problem, configShown, configPath(home)) + statusIgnoreWarningTail}, got.Warnings)
			assert.Equal(t, "quarry: warning: "+statusIgnoreWarningLead+c.problem+statusIgnoreWarningTail+"\n", stderr.String())
		})
	}
}

func Test_run_status_json_reports_the_findings_counts_with_the_ignore_list(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	duplicate := syncStatusFindingsFixture(t, home)
	writeConfig(t, home, fmt.Sprintf("[findings]\nignore = [%q]\n", duplicate))
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"status", "--json"}, &stdout, &stderr)

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	var got statusFindingsJSON
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &got))
	assert.Equal(t, 3, got.Findings.Open)
	assert.Equal(t, new(1), got.Findings.Ignored)
	assert.Equal(t, 0, got.Findings.Fixed)
	assert.Equal(t, 3, got.Findings.New)
	assert.Equal(t, 0, got.Findings.NewlyFixed)
	assert.Equal(t, []string{}, got.Warnings)
}

func Test_run_status_stays_silent_on_unknown_keys_and_unmatched_ignore_ids(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	syncStatusFindingsFixture(t, home)
	writeConfig(t, home, "[snapshot]\nkeep = 3\n[findings]\nignore = [\"uncategorized:payee-999\"]\n")
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"status", "--json"}, &stdout, &stderr)

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	var got statusFindingsJSON
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &got))
	assert.Equal(t, []string{}, got.Warnings)
	assert.Equal(t, new(0), got.Findings.Ignored)
}

func Test_run_status_reports_every_finding_open_without_a_warning_when_there_is_no_config_file(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	syncStatusFindingsFixture(t, home)
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"status"}, &stdout, &stderr)

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Contains(t, stdout.String(), "\nFindings  4 open; run quarry findings to list them\n")
}

func Test_run_status_refuses_a_missing_store_without_reading_the_config(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeConfig(t, home, "[snapshots]\nkeep = 0\n")
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"status"}, &stdout, &stderr)

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout.String())
	assert.Equal(t, "quarry: no store at "+abbreviated(t, storePathUnder(home), home)+" yet; run quarry sync to build it\n", stderr.String())
}

func Test_run_status_refuses_a_store_whose_findings_cannot_be_read_without_the_config_warning(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	syncStatusFindingsFixture(t, home)
	editStore(t, home, "DROP TABLE finding_items")
	editStore(t, home, "DROP TABLE findings")
	writeConfig(t, home, "[snapshots]\nkeep = 0\n")
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"status"}, &stdout, &stderr)

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout.String())
	assert.Equal(t, "quarry: cannot read the store at "+abbreviated(t, storePathUnder(home), home)+
		": Table with name findings does not exist!; run quarry sync to rebuild it\n", stderr.String())
}
