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
	"github.com/koblas/quarry/internal/store/duckstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_run_status_describes_the_store_sync_built(t *testing.T) {
	home := newHome(t)

	bundle := writeStatusFixtureBundle(t, home)

	syncBundle(t, bundle)
	editStore(t, home, "DELETE FROM fx_rates") // the Rates line's age follows the real clock; the none arm does not
	snapshotsDir := snapshotsDir(home)
	snapshotPath := onlyFileWithSuffix(t, snapshotsDir, ".sqlite")
	raw, err := os.ReadFile(onlyFileWithSuffix(t, snapshotsDir, ".json"))
	require.NoError(t, err)
	var manifest struct {
		Snapshot struct {
			TakenAt string `json:"taken_at"`
		} `json:"snapshot"`
	}
	require.NoError(t, json.Unmarshal(raw, &manifest))
	takenAt, err := time.Parse(time.RFC3339, manifest.Snapshot.TakenAt)
	require.NoError(t, err)

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"status"})

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	want := fmt.Sprintf("%-10s%s\n%-10s%s, taken %s (just now)\n%-10s%s\n%-10s%s\n%-10s%s\n%-10s%s\n%-10s%s\n%-10s%s\n%-10s%s\n%-10s%s\n%-10s%s\n",
		"Store", abbreviated(t, storePathUnder(home), home),
		"Snapshot", snapshotID(snapshotPath), takenAt.In(time.Local).Format("2006-01-02 15:04 MST"), //nolint:gosmopolitan // status prints the user's local zone
		"Source", abbreviated(t, bundle.Dir, home),
		"Dates", "2026-01-05 to 2026-03-20",
		"Rows", "4 transactions, 4 splits, 2 transfers, 0 payees, 0 categories, 0 tags; 0 investment transactions, 0 securities, 0 prices",
		"Balances", "1 account matches Quicken's last reconciled balance; 1 never reconciled and 1 investment account's cash not checked",
		"Splits", "all 4 transactions equal the sum of their splits",
		"Shares", "no holdings to check",
		"Transfers", "1 paired, 1 one-sided",
		"Findings", "3 open; run quarry findings to list them",
		"Rates", "none, so amounts are not converted; run quarry sync to fetch them from the Bank of Canada",
	)
	assert.Equal(t, want, stdout.String())
}

func Test_run_status_says_investment_accounts_cash_is_not_checked(t *testing.T) {
	home := newHome(t)
	b := v9fixture.NewBuilder()
	brokerage := b.Account(v9fixture.AccountRow{Name: "Brokerage", Type: "BROKERAGENORMAL", Currency: "CAD", Active: true})
	ira := b.Account(v9fixture.AccountRow{Name: "IRA", Type: "RETIREMENTIRA", Currency: "CAD", Active: true})
	day := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	dividend := new(int64(10))
	for _, account := range []int64{brokerage, ira} {
		pk := b.InvestmentTransaction(v9fixture.TransactionRow{Account: account, PostedDate: &day, Type: dividend, Amount: "12.00"})
		b.Entry(v9fixture.EntryRow{Parent: pk, Amount: "12.00"})
	}
	syncBundle(t, b.WriteBundle(t, filepath.Join(home, "Documents")))

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"status"})

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Contains(t, stdout.String(), fmt.Sprintf("%-10s%s\n", "Balances", "no accounts to check; 2 investment accounts' cash not checked"))
}

func Test_run_status_reports_the_latest_build_when_import_runs_holds_several(t *testing.T) {
	home := newHome(t)
	bundle := writeStatusFixtureBundle(t, home)
	syncBundle(t, bundle)
	snapshotsDir := filepath.Join(storeDirUnder(home), "snapshots")
	earlierPath := onlyFileWithSuffix(t, snapshotsDir, ".sqlite")
	laterPath := filepath.Join(snapshotsDir, "later-build.sqlite")
	var before bytes.Buffer
	require.Equal(t, 0, run(context.Background(), []string{"status"}, &before, &bytes.Buffer{}))
	editStore(t, home, "INSERT INTO import_runs SELECT * REPLACE (2 AS id) FROM import_runs") //nolint:unqueryvet // a copy of the row is the point
	editStore(t, home, "UPDATE import_runs SET snapshot_path = '"+laterPath+"' WHERE id = 2")

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"status"})

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, strings.Replace(before.String(), snapshotID(earlierPath), "later-build", 1), stdout.String())
}

func Test_run_status_reports_a_failed_stdout_write(t *testing.T) {
	home := newHome(t)
	b := v9fixture.NewBuilder()
	b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	bundle := b.WriteBundle(t, filepath.Join(home, "Documents"))
	syncBundle(t, bundle)
	var stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"status"}, failingWriter{err: errNoSpace}, &stderr)

	assert.Equal(t, 1, exitCode)
	assert.Equal(t, "quarry: cannot write the result to stdout: no space left on device\n", stderr.String())
}

func Test_run_help_prints_quarrys_description(t *testing.T) {
	t.Setenv("HOME", "")

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"--help"})

	require.Equal(t, 0, exitCode)
	assert.Empty(t, stderr.String())
	assert.Contains(t, stdout.String(), `quarry copies the Quicken Classic for Mac file you have open into a local,
read-only snapshot, rebuilds its own store from that snapshot, and checks the
store against Quicken's balances. Every other command reads that store;
quarry never writes to the Quicken file.`)
	assert.Contains(t, stdout.String(), ""+
		"Available Commands:\n"+
		"  acb         Show adjusted cost base and realized capital gains per tax year, in CAD\n"+
		"  accounts    List accounts with their current balances\n"+
		"  anomalies   List charges unusually large for their payee or category\n"+
		"  cashflow    Show income, spending and savings rate by month or year\n"+
		"  claude      Install quarry in Claude Code and Claude Desktop, or remove it\n"+
		"  findings    List what to clean up in Quicken\n"+
		"  help        Help about any command\n"+
		"  holdings    List the securities held in each account and their value\n"+
		"  mcp         Serve quarry's store to Claude over MCP (stdio)\n"+
		"  networth    Show net worth today or at each month end, by account type and currency\n"+
		"  recurring   List charges that repeat every week, month, quarter or year\n"+
		"  search      Find transactions by payee, memo, amount, date, account or category\n"+
		"  snapshots   List the snapshots quarry has taken and which one the store was built from\n"+
		"  spend       Show spending by category, payee, tag or month\n"+
		"  sql         Run a read-only SQL query against quarry's store\n"+
		"  status      Show which snapshot the store was built from and what it holds\n"+
		"  summary     Summarize a month: unusual charges, new recurring charges, net worth and findings\n"+
		"  sync        Snapshot the open Quicken file and rebuild quarry's store from it\n\n")
}

func Test_run_status_help_describes_the_command_without_needing_home(t *testing.T) {
	t.Setenv("HOME", "")

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"status", "--help"})

	require.Equal(t, 0, exitCode)
	assert.Empty(t, stderr.String())
	assert.Contains(t, stdout.String(), `Show the store quarry's commands read: the snapshot it was built from, when
that snapshot was taken, the Quicken file it came from, the dates its
transactions cover, the checks sync ran when it built the store, and how
many findings are open.

status reads quarry's store, and the config file for the findings you ignored;
it never looks at Quicken. Run quarry sync to bring the store up to date.

Rates shows the span of Bank of Canada USD/CAD rates the store holds and,
when the last sync could not fetch new ones, why.`)
}

// writeStatusFixtureBundle writes a bundle with reconciled, never-reconciled and
// investment accounts, one paired cross-currency transfer and one one-sided leg.
func writeStatusFixtureBundle(t *testing.T, home string) v9fixture.Bundle {
	t.Helper()
	b := v9fixture.NewBuilder()
	chequingPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	savingsPK := b.Account(v9fixture.AccountRow{Name: "US Savings", Type: "SAVINGS", Currency: "USD", Active: true})
	b.Account(v9fixture.AccountRow{Name: "Brokerage", Type: "BROKERAGENORMAL", Currency: "CAD", Active: true})
	earliest := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)
	transferDay := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	latest := time.Date(2026, 3, 20, 0, 0, 0, 0, time.UTC)
	reconciled := int64(2)
	depositTxn := b.Transaction(v9fixture.TransactionRow{Account: chequingPK, Amount: "100.00", PostedDate: &earliest, Status: &reconciled})
	b.Entry(v9fixture.EntryRow{Parent: depositTxn, Amount: "100.00"})
	b.Reconcile(v9fixture.ReconcileRow{Account: chequingPK, EndDate: &earliest, EndingBalance: "100.00"})
	sentTxn := b.Transaction(v9fixture.TransactionRow{Account: chequingPK, Amount: "-100.00", PostedDate: &transferDay})
	b.Entry(v9fixture.EntryRow{Parent: sentTxn, Amount: "-100.00", QuickenID: 1001, Transfer: "2002"})
	receivedTxn := b.Transaction(v9fixture.TransactionRow{Account: savingsPK, Amount: "75.00", PostedDate: &transferDay})
	b.Entry(v9fixture.EntryRow{Parent: receivedTxn, Amount: "75.00", QuickenID: 2002, Transfer: "1001"})
	strayTxn := b.Transaction(v9fixture.TransactionRow{Account: chequingPK, Amount: "-5.00", PostedDate: &latest})
	b.Entry(v9fixture.EntryRow{Parent: strayTxn, Amount: "-5.00", QuickenID: 3001, Transfer: "Old Visa"})

	return b.WriteBundle(t, filepath.Join(home, "Documents"))
}

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
	home := newHome(t)
	duplicate := syncStatusFindingsFixture(t, home)
	writeConfig(t, home, fmt.Sprintf("[findings]\nignore = [%q]\n", duplicate))

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"status"})

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Contains(t, stdout.String(), "\nFindings  3 open, 1 ignored; run quarry findings to list them\n")
}

func Test_run_status_warns_on_a_bad_config_and_still_reports(t *testing.T) {
	home := newHome(t)
	duplicate := syncStatusFindingsFixture(t, home)
	writeConfig(t, home, fmt.Sprintf("[snapshots]\nkeep = 0\n[findings]\nignore = [%q]\n", duplicate))

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"status"})

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Equal(t, "quarry: warning: cannot tell which findings you ignored or how you classified your accounts: "+configShown+
		": snapshots.keep must be a whole number of 1 or more, got 0; findings you ignored are counted as open, and every investment account is counted as unclassified\n", stderr.String())
	assert.Contains(t, stdout.String(), "\nFindings  4 open; run quarry findings to list them\n")
}

const (
	statusIgnoreWarningLead = "cannot tell which findings you ignored or how you classified your accounts: "
	statusIgnoreWarningTail = "; findings you ignored are counted as open, and every investment account is counted as unclassified"
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
			home := newHome(t)
			syncStatusFindingsFixture(t, home)
			c.setup(t, home)

			exitCode, stdout, stderr := runCapture(context.Background(), []string{"status"})

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
			home := newHome(t)
			syncStatusFindingsFixture(t, home)
			c.setup(t, home)

			exitCode, stdout, stderr := runCapture(context.Background(), []string{"status", "--json"})

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
	home := newHome(t)
	duplicate := syncStatusFindingsFixture(t, home)
	writeConfig(t, home, fmt.Sprintf("[findings]\nignore = [%q]\n", duplicate))

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"status", "--json"})

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
	home := newHome(t)
	syncStatusFindingsFixture(t, home)
	writeConfig(t, home, "[snapshot]\nkeep = 3\n[findings]\nignore = [\"uncategorized:payee-999\"]\n")

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"status", "--json"})

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	var got statusFindingsJSON
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &got))
	assert.Equal(t, []string{}, got.Warnings)
	assert.Equal(t, new(0), got.Findings.Ignored)
}

func Test_run_status_reports_every_finding_open_without_a_warning_when_there_is_no_config_file(t *testing.T) {
	home := newHome(t)
	syncStatusFindingsFixture(t, home)

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"status"})

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Contains(t, stdout.String(), "\nFindings  4 open; run quarry findings to list them\n")
}

func Test_run_status_refuses_a_missing_store_without_reading_the_config(t *testing.T) {
	home := newHome(t)
	writeConfig(t, home, "[snapshots]\nkeep = 0\n")

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"status"})

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout.String())
	assert.Equal(t, "quarry: no store at "+abbreviated(t, storePathUnder(home), home)+" yet; run quarry sync to build it\n", stderr.String())
}

func Test_run_status_refuses_a_store_whose_findings_cannot_be_read_without_the_config_warning(t *testing.T) {
	home := newHome(t)
	syncStatusFindingsFixture(t, home)
	editStore(t, home, "DROP TABLE finding_items")
	editStore(t, home, "DROP TABLE findings")
	writeConfig(t, home, "[snapshots]\nkeep = 0\n")

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"status"})

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout.String())
	assert.Equal(t, "quarry: cannot read the store at "+abbreviated(t, storePathUnder(home), home)+
		": Table with name findings does not exist!; run quarry sync to rebuild it\n", stderr.String())
}

func Test_run_status_json_describes_the_store_sync_built(t *testing.T) {
	home := newHome(t)
	bundle := writeStatusFixtureBundle(t, home)
	syncBundle(t, bundle)
	snapshotsDir := snapshotsDir(home)
	snapshotPath := onlyFileWithSuffix(t, snapshotsDir, ".sqlite")
	raw, err := os.ReadFile(onlyFileWithSuffix(t, snapshotsDir, ".json"))
	require.NoError(t, err)
	var manifest struct {
		Snapshot struct {
			TakenAt string `json:"taken_at"`
			SHA256  string `json:"sha256"`
		} `json:"snapshot"`
	}
	require.NoError(t, json.Unmarshal(raw, &manifest))

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"status", "--json"})

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	var unfixed struct {
		Store struct {
			BuiltAt string `json:"built_at"`
		} `json:"store"`
	}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &unfixed))
	builtAt, err := time.Parse(time.RFC3339, unfixed.Store.BuiltAt)
	require.NoError(t, err)
	assert.Equal(t, time.UTC, builtAt.Location())
	assert.Equal(t, unfixed.Store.BuiltAt, builtAt.UTC().Format(time.RFC3339))
	want := fmt.Sprintf(`{
  "store": {
    "path": %q,
    "format_version": %d,
    "quarry_version": %q,
    "built_at": %q,
    "rows": {
      "accounts": 3,
      "categories": 0,
      "payees": 0,
      "tags": 0,
      "transactions": 4,
      "splits": 4,
      "split_tags": 0,
      "transfers": 2,
      "investment_transactions": 0,
      "securities": 0,
      "prices": 0
    }
  },
  "snapshot": {
    "id": %q,
    "path": %q,
    "taken_at": %q,
    "source": %q,
    "sha256": %q
  },
  "dates": {
    "first": "2026-01-05",
    "last": "2026-03-20"
  },
  "balances": {
    "checked": 1,
    "never_reconciled": 1,
    "investment_accounts": 1
  },
  "splits": {
    "checked": 4
  },
  "shares": {
    "checked": 0
  },
  "transfers": {
    "paired": 1,
    "cross_currency": 1,
    "one_sided": 1
  },
  "findings": {
    "open": 3,
    "ignored": 0,
    "fixed": 0,
    "new": 2,
    "newly_fixed": 0
  },
  "rates": {
    "first": "2026-01-02",
    "last": "2026-01-02",
    "fetch_error": null
  },
  "warnings": []
}
`,
		storePathUnder(home), duckstore.FormatVersion, "(devel)", unfixed.Store.BuiltAt,
		snapshotID(snapshotPath), snapshotPath, manifest.Snapshot.TakenAt, bundle.Dir, manifest.Snapshot.SHA256)
	assert.Equal(t, want, stdout.String())
}

func Test_run_status_json_reports_the_latest_build_when_import_runs_holds_several(t *testing.T) {
	home := newHome(t)
	bundle := writeStatusFixtureBundle(t, home)
	syncBundle(t, bundle)
	snapshotsDir := filepath.Join(storeDirUnder(home), "snapshots")
	earlierPath := onlyFileWithSuffix(t, snapshotsDir, ".sqlite")
	laterPath := filepath.Join(snapshotsDir, "later-build.sqlite")
	var before bytes.Buffer
	require.Equal(t, 0, run(context.Background(), []string{"status", "--json"}, &before, &bytes.Buffer{}))
	editStore(t, home, "INSERT INTO import_runs SELECT * REPLACE (2 AS id) FROM import_runs") //nolint:unqueryvet // a copy of the row is the point
	editStore(t, home, "UPDATE import_runs SET snapshot_path = '"+laterPath+"' WHERE id = 2")

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"status", "--json"})

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	wantJSON := strings.ReplaceAll(before.String(), earlierPath, laterPath)
	wantJSON = strings.ReplaceAll(wantJSON, snapshotID(earlierPath), "later-build")
	assert.Equal(t, wantJSON, stdout.String()) //nolint:testifylint // the bytes are the contract
}

func Test_status_json_carries_each_path_the_skill_reads(t *testing.T) {
	home := newHome(t)
	syncBundle(t, writeStatusFixtureBundle(t, home))
	var stdout, stderr bytes.Buffer
	require.Equal(t, 0, run(context.Background(), []string{"status", "--json"}, &stdout, &stderr), stderr.String())
	var status any
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &status))
	freshness := splitSkill(t, repoFile(t, skillPath)).bodies[skillHeadings[0]]

	// wantType is the JSON type the skill relies on; nullable allows null.
	paths := []struct {
		path     string
		wantType any
		nullable bool
	}{
		{"snapshot.taken_at", "", false},
		{"dates.last", "", false},
		{"rates.fetch_error", nil, true},
		{"rates.last", "", false},
		{"findings.open", float64(0), false},
	}
	for _, p := range paths {
		t.Run(p.path, func(t *testing.T) {
			got, ok := statusPath(status, p.path)
			require.True(t, ok, "status --json has no %s", p.path)
			if !p.nullable {
				assert.IsType(t, p.wantType, got)
			}
			assert.Contains(t, freshness, p.path)
		})
	}
}

// statusPath walks a dotted path through decoded JSON objects.
func statusPath(doc any, path string) (any, bool) {
	for key := range strings.SplitSeq(path, ".") {
		object, ok := doc.(map[string]any)
		if !ok {
			return nil, false
		}
		if doc, ok = object[key]; !ok {
			return nil, false
		}
	}
	return doc, true
}

func Test_run_status_reports_the_share_check(t *testing.T) {
	home := newHome(t)
	syncBundle(t, holdingsBundle(t, home))

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"status"})

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Contains(t, stdout.String(),
		"Rows      4 transactions, 4 splits, 0 transfers, 0 payees, 0 categories, 0 tags; 3 investment transactions, 2 securities, 3 prices\n")
	assert.Contains(t, stdout.String(), "Shares    2 holdings match Quicken's share counts\n")
	assert.NotContains(t, stdout.String(), "not imported")

	var jsonOut, jsonErr bytes.Buffer
	exitCode = run(context.Background(), []string{"status", "--json"}, &jsonOut, &jsonErr)

	require.Equal(t, 0, exitCode, jsonErr.String())
	var doc map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(jsonOut.Bytes(), &doc))
	assert.JSONEq(t, `{"checked":2}`, string(doc["shares"]))
	assert.NotContains(t, doc, "not_imported")
	var storeDoc struct {
		Rows map[string]int `json:"rows"`
	}
	require.NoError(t, json.Unmarshal(doc["store"], &storeDoc))
	assert.Equal(t, 3, storeDoc.Rows["investment_transactions"])
	assert.Equal(t, 2, storeDoc.Rows["securities"])
	assert.Equal(t, 3, storeDoc.Rows["prices"])
}

// Case-insensitive volumes resolve the lower-case name too: only the recorded path and the printed id tell them apart.
func Test_run_status_names_a_store_built_from_an_upper_case_sqlite_snapshot_by_its_id(t *testing.T) {
	home := newHome(t)
	syncBundle(t, writeNamedAccountBundle(t, filepath.Join(home, "Documents"), "Chequing"))
	snapshotsDir := filepath.Join(storeDirUnder(home), "snapshots")
	id := snapshotID(onlyFileWithSuffix(t, snapshotsDir, ".sqlite"))
	upper := filepath.Join(snapshotsDir, id+".SQLITE")
	require.NoError(t, os.Rename(filepath.Join(snapshotsDir, id+".sqlite"), upper))
	require.Contains(t, dirNames(t, snapshotsDir), id+".SQLITE")
	require.NoError(t, os.Remove(storePathUnder(home)))
	var rebuildOut, rebuildErr bytes.Buffer
	require.Equal(t, 0, run(context.Background(), []string{"sync", "--from", id}, &rebuildOut, &rebuildErr), rebuildErr.String())

	t.Run("text", func(t *testing.T) {
		exitCode, stdout, stderr := runCapture(context.Background(), []string{"status"})

		require.Equal(t, 0, exitCode, stderr.String())
		assert.Contains(t, stdout.String(), "\nSnapshot  "+id+", taken ")
	})

	t.Run("json", func(t *testing.T) {
		exitCode, stdout, stderr := runCapture(context.Background(), []string{"status", "--json"})

		require.Equal(t, 0, exitCode, stderr.String())
		var parsed struct {
			Snapshot struct {
				ID   string `json:"id"`
				Path string `json:"path"`
			} `json:"snapshot"`
		}
		require.NoError(t, json.Unmarshal(stdout.Bytes(), &parsed))
		assert.Equal(t, id, parsed.Snapshot.ID)
		assert.Equal(t, upper, parsed.Snapshot.Path)
	})
}

func Test_run_status_counts_an_unclassified_account_until_the_config_classifies_it(t *testing.T) {
	home := newHome(t)
	id := syncUnclassifiedFixture(t, home)
	var before, after, stderr bytes.Buffer

	require.Equal(t, 0, run(context.Background(), []string{"status"}, &before, &stderr), stderr.String())
	writeConfig(t, home, fmt.Sprintf("[accounts]\nregistered = [%q]\n", id))
	require.Equal(t, 0, run(context.Background(), []string{"status"}, &after, &stderr), stderr.String())

	assert.Contains(t, before.String(), "\nFindings  1 open; run quarry findings to list them\n")
	assert.Contains(t, after.String(), "\nFindings  none open\n")
	assert.Empty(t, stderr.String())
}

// unclassifiedAccountBundle is a chequing control and one brokerage account, with the brokerage account's id.
func unclassifiedAccountBundle() (*v9fixture.Builder, string) {
	b := v9fixture.NewBuilder()
	b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	brokeragePK := b.Account(v9fixture.AccountRow{Name: "Questrade TFSA", Type: "BROKERAGENORMAL", Currency: "CAD", Active: true})
	return b, fmt.Sprintf("acct-%d", brokeragePK)
}

// syncUnclassifiedFixture syncs unclassifiedAccountBundle under home, returning the brokerage account's id.
func syncUnclassifiedFixture(t *testing.T, home string) string {
	t.Helper()
	b, id := unclassifiedAccountBundle()
	syncBundle(t, b.WriteBundle(t, filepath.Join(home, "Documents")))
	return id
}

// statusFindings is quarry status --json over the HOME the test set, decoded.
func statusFindings(t *testing.T) statusFindingsJSON {
	t.Helper()
	var stdout, stderr bytes.Buffer
	require.Equal(t, 0, run(context.Background(), []string{"status", "--json"}, &stdout, &stderr), stderr.String())
	var got statusFindingsJSON
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &got))
	return got
}

func Test_run_status_json_counts_an_unclassified_account_open_and_never_new_until_the_config_lists_it(t *testing.T) {
	home := newHome(t)
	id := syncUnclassifiedFixture(t, home)

	before := statusFindings(t)
	writeConfig(t, home, fmt.Sprintf("[accounts]\nnon-registered = [%q]\n", id))
	after := statusFindings(t)

	assert.Equal(t, []int{1, 0}, []int{before.Findings.Open, before.Findings.New})
	assert.Equal(t, []int{0, 0}, []int{after.Findings.Open, after.Findings.New})
}

func Test_run_status_json_counts_an_ignored_unclassified_account_as_ignored(t *testing.T) {
	home := newHome(t)
	id := syncUnclassifiedFixture(t, home)
	writeConfig(t, home, fmt.Sprintf("[findings]\nignore = [\"unclassified-account:%s\"]\n", id))

	got := statusFindings(t)

	require.NotNil(t, got.Findings.Ignored)
	assert.Equal(t, []int{0, 1}, []int{got.Findings.Open, *got.Findings.Ignored})
}

func Test_run_status_counts_every_investment_account_open_when_the_config_is_unreadable(t *testing.T) {
	home := newHome(t)
	id := syncUnclassifiedFixture(t, home)
	writeConfig(t, home, fmt.Sprintf("[snapshots]\nkeep = 0\n[accounts]\nnon-registered = [%q]\n", id))

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"status"})

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Contains(t, stdout.String(), "\nFindings  1 open; run quarry findings to list them\n")
	assert.Equal(t, "quarry: warning: "+statusIgnoreWarningLead+configShown+
		": snapshots.keep must be a whole number of 1 or more, got 0"+statusIgnoreWarningTail+"\n", stderr.String())
}

func Test_run_status_json_counts_every_investment_account_open_when_the_config_is_unreadable(t *testing.T) {
	home := newHome(t)
	id := syncUnclassifiedFixture(t, home)
	writeConfig(t, home, fmt.Sprintf("[snapshots]\nkeep = 0\n[accounts]\nnon-registered = [%q]\n", id))

	got := statusFindings(t)

	assert.Equal(t, 1, got.Findings.Open)
	assert.Nil(t, got.Findings.Ignored)
	assert.Equal(t, []string{statusIgnoreWarningLead + configPath(home) + ": snapshots.keep must be a whole number of 1 or more, got 0" + statusIgnoreWarningTail}, got.Warnings)
}

func Test_run_mcp_sync_status_counts_an_unclassified_account_open_until_the_config_lists_it(t *testing.T) {
	var home, id string
	ctx, peer := newStatusPeer(t, func(t *testing.T, h string) {
		t.Helper()
		home = h
		id = syncUnclassifiedFixture(t, h)
	})

	before := readSyncStatus(ctx, t, peer)
	writeConfig(t, home, fmt.Sprintf("[accounts]\nnon-registered = [%q]\n", id))
	after := readSyncStatus(ctx, t, peer)

	assert.Equal(t, []int{1, 0}, []int{before.Findings.Open, after.Findings.Open})
}
