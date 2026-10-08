// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_run_sync_help_names_both_documents_folders(t *testing.T) {
	newHome(t)

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"sync", "--help"})

	require.Equal(t, 0, exitCode)
	assert.Empty(t, stderr.String())
	assert.Contains(t, stdout.String(), "Without --quicken, quarry uses quicken.path from\n"+
		"~/Library/Application Support/quarry/config.toml if it is set. Otherwise it\n"+
		"looks for .quicken files in ~/Documents and in\n"+
		"~/Library/Application Support/Quicken/Documents, and uses the one it finds\n"+
		"if there is exactly one.")
	//nolint:dupword // the --quicken placeholder "path" is followed by usage text that starts with it
	assert.Contains(t, stdout.String(), "--quicken path    path to the .quicken file to snapshot "+
		"(default: quicken.path in the config file, else the only one in ~/Documents or Quicken's Documents folder)")
}

func Test_run_prints_the_sync_help(t *testing.T) {
	newHome(t)
	var rootStdout, rootStderr, syncStdout, syncStderr bytes.Buffer

	rootExit := run(context.Background(), []string{"--help"}, &rootStdout, &rootStderr)
	syncExit := run(context.Background(), []string{"sync", "--help"}, &syncStdout, &syncStderr)

	require.Equal(t, 0, rootExit)
	require.Equal(t, 0, syncExit)
	assert.Empty(t, rootStderr.String())
	assert.Empty(t, syncStderr.String())
	assert.Contains(t, rootStdout.String(), "Snapshot the open Quicken file and rebuild quarry's store from it")
	assert.Contains(t, syncStdout.String(), "quarry then rebuilds its store, ~/Library/Application Support/quarry/quarry.duckdb,\n"+
		"from the snapshot. In every reconciled account, the reconciled transactions\n"+
		"must add up to the balance of its last reconciled statement in Quicken to the\n"+
		"cent, and every transaction must equal the sum of its splits, and in every\n"+
		"brokerage and retirement account each security's share count must equal\n"+
		"Quicken's; if a check fails, the previous store is left unchanged.")
	assert.Contains(t, syncStdout.String(), "sync then looks for things to clean up in Quicken, such as uncategorized\n"+
		"splits, one-sided transfers and possible duplicates; run quarry findings to\n"+
		"list them. Findings never fail a sync.")
	assert.Contains(t, syncStdout.String(), "list them. Findings never fail a sync.\n\n"+
		"sync then fetches the Bank of Canada's daily USD/CAD exchange rates for any\n"+
		"dates the store does not have, back to your earliest transaction. This is\n"+
		"quarry's only use of the network, and the request carries nothing but the\n"+
		"dates. If the fetch fails, sync still succeeds, warns, and reports convert\n"+
		"with the rates the store already has.\n\n"+
		"After it rebuilds the store, sync deletes the oldest snapshots beyond the\n"+
		"newest 12 (snapshots.keep in ~/Library/Application Support/quarry/config.toml),\n"+
		"never the one the store was built from; a failed sync deletes nothing. Run\n"+
		"quarry snapshots to list them.")
	assert.Contains(t, syncStdout.String(), "With --from, quarry rebuilds the store from a snapshot it took earlier and\n"+
		"does not read Quicken at all.")
	assert.Contains(t, syncStdout.String(), "  quarry sync --quicken ~/Documents/Home.quicken\n  quarry sync --from 20260927T143005Z\n")
	//nolint:dupword // the --from placeholder "snapshot" is followed by usage text that starts with it
	assert.Contains(t, syncStdout.String(), "--from snapshot   snapshot to rebuild the store from instead of reading Quicken: "+
		"an ID such as 20260927T143005Z, or the path to its .sqlite file")
}

func Test_run_help_says_only_one_writer_runs_at_a_time(t *testing.T) {
	const oneWriter = "Only one quarry sync or quarry snapshots prune runs at a time; while one\n" +
		"is running, another stops at once and changes nothing."
	cases := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "sync follows the auto-prune paragraph",
			args: []string{"sync", "--help"},
			want: "never the one the store was built from; a failed sync deletes nothing. Run\n" +
				"quarry snapshots to list them.\n\n" + oneWriter,
		},
		{
			name: "prune follows its first paragraph and replaces the dry-run line",
			args: []string{"snapshots", "prune", "--help"},
			want: "snapshot the store was built from is never deleted, even when it is older.\n\n" +
				oneWriter + "\n\n" +
				"With --dry-run, prune lists what it would delete and deletes nothing; it\n" +
				"runs even while a sync is running.",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			newHome(t)

			exitCode, stdout, stderr := runCapture(context.Background(), c.args)

			require.Equal(t, 0, exitCode)
			assert.Empty(t, stderr.String())
			assert.Contains(t, stdout.String(), c.want)
		})
	}
}

func Test_run_refuses_from_with_quicken_or_without_a_value(t *testing.T) {
	const u3 = "quarry: --from and --quicken cannot be used together; --from rebuilds from a snapshot without reading Quicken\n"
	const u4 = "quarry: flag needs an argument: --from; Run 'quarry sync --help' for usage.\n"
	cases := []struct {
		name       string
		args       []string
		wantStderr string
	}{
		{name: "with --quicken", args: []string{"sync", "--from", "20260927T143005Z", "--quicken", "~/Documents/Home.quicken"}, wantStderr: u3},
		{name: "with an empty --quicken", args: []string{"sync", "--from", "20260927T143005Z", "--quicken="}, wantStderr: u3},
		{name: "empty, with --quicken", args: []string{"sync", "--from=", "--quicken", "~/Documents/Home.quicken"}, wantStderr: u3},
		{name: "empty", args: []string{"sync", "--from="}, wantStderr: u4},
		{name: "all whitespace", args: []string{"sync", "--from=   "}, wantStderr: u4},
		{name: "missing its value", args: []string{"sync", "--from"}, wantStderr: u4},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := newHome(t)
			v9fixture.OpenBundle(t, filepath.Join(home, "Documents"))

			exitCode, stdout, stderr := runCapture(context.Background(), c.args)

			assert.Equal(t, 2, exitCode)
			assert.Empty(t, stdout.String())
			assert.Equal(t, c.wantStderr, stderr.String())
			assertNoSnapshotsDir(t, home)
		})
	}
}

func Test_run_refuses_an_empty_or_whitespace_quicken_flag_as_a_usage_error_even_with_a_bundle_in_documents(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{name: "empty", args: []string{"sync", "--quicken="}},
		{name: "all whitespace", args: []string{"sync", "--quicken=   "}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := newHome(t)
			v9fixture.OpenBundle(t, filepath.Join(home, "Documents"))

			exitCode, stdout, stderr := runCapture(context.Background(), c.args)

			assert.Equal(t, 2, exitCode)
			assert.Empty(t, stdout.String())
			assert.Equal(t, "quarry: flag needs an argument: --quicken; Run 'quarry sync --help' for usage.\n", stderr.String())
			assertNoSnapshotsDir(t, home)
		})
	}
}

func Test_run_rejects_usage_errors(t *testing.T) {
	cases := []struct {
		name       string
		args       []string
		wantStderr string
	}{
		{
			name:       "positional argument",
			args:       []string{"sync", "~/x.quicken"},
			wantStderr: "quarry: sync takes no arguments; pass the file with --quicken <path>\n",
		},
		{
			name:       "unknown flag",
			args:       []string{"sync", "--bogus"},
			wantStderr: "quarry: unknown flag: --bogus; Run 'quarry sync --help' for usage.\n",
		},
		{
			name:       "flag missing its value",
			args:       []string{"sync", "--quicken"},
			wantStderr: "quarry: flag needs an argument: --quicken; Run 'quarry sync --help' for usage.\n",
		},
		{
			name:       "snapshots with a positional argument",
			args:       []string{"snapshots", "list"},
			wantStderr: "quarry: snapshots takes no arguments; to delete old snapshots run quarry snapshots prune; Run 'quarry snapshots --help' for usage.\n",
		},
		{
			name:       "prune with a negative keep",
			args:       []string{"snapshots", "prune", "--keep", "-1"},
			wantStderr: "quarry: --keep must be 1 or more; the snapshot the store was built from is always kept; Run 'quarry snapshots prune --help' for usage.\n",
		},
		{
			name:       "prune with a keep that is not a number",
			args:       []string{"snapshots", "prune", "--keep", "abc"},
			wantStderr: "quarry: invalid argument \"abc\" for \"--keep\" flag: strconv.ParseInt: parsing \"abc\": invalid syntax; Run 'quarry snapshots prune --help' for usage.\n",
		},
		{
			name:       "prune with a positional argument",
			args:       []string{"snapshots", "prune", "extra"},
			wantStderr: "quarry: prune takes no arguments; Run 'quarry snapshots prune --help' for usage.\n",
		},
		{
			name:       "mcp with a positional argument",
			args:       []string{"mcp", "extra"},
			wantStderr: "quarry: mcp takes no arguments\n",
		},
		{
			name:       "claude install with a positional argument",
			args:       []string{"claude", "install", "extra"},
			wantStderr: claudeInstallArgs,
		},
		{
			name:       "claude uninstall with a positional argument",
			args:       []string{"claude", "uninstall", "extra"},
			wantStderr: claudeUninstallArgs,
		},
		{
			name:       "claude with an unknown subcommand",
			args:       []string{"claude", "bogus"},
			wantStderr: "quarry: unknown command \"bogus\" for \"quarry claude\"; Run 'quarry claude --help' for usage.\n",
		},
		{
			name:       "unknown command",
			args:       []string{"frob"},
			wantStderr: "quarry: unknown command \"frob\" for \"quarry\"; Run 'quarry --help' for usage.\n",
		},
		{
			name:       "near miss of a known command",
			args:       []string{"synk"},
			wantStderr: "quarry: unknown command \"synk\" for \"quarry\"; Run 'quarry --help' for usage.\n",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			newHome(t)

			exitCode, stdout, stderr := runCapture(context.Background(), c.args)

			require.Equal(t, 2, exitCode)
			assert.Empty(t, stdout.String())
			assert.Equal(t, c.wantStderr, stderr.String())
			assert.NotContains(t, stderr.String(), "Did you mean")
		})
	}
}

func Test_run_read_flag_usage_errors_point_at_the_commands_help(t *testing.T) {
	type usageCase struct {
		name       string
		args       []string
		wantStderr string
	}
	cases := make([]usageCase, 0, 11)
	cases = append(cases,
		usageCase{
			name: "holdings --as-of without a value", args: []string{"holdings", "--as-of"},
			wantStderr: "quarry: flag needs an argument: --as-of; Run 'quarry holdings --help' for usage.\n",
		},
		usageCase{
			name: "holdings --all, which does not exist", args: []string{"holdings", "--all"},
			wantStderr: "quarry: unknown flag: --all; Run 'quarry holdings --help' for usage.\n",
		},
	)
	for _, command := range []string{"spend", "cashflow", "recurring", "anomalies", "accounts", "holdings", "networth", "acb", "summary"} {
		cases = append(cases, usageCase{
			name: command + " --currency without a value", args: []string{command, "--currency"},
			wantStderr: "quarry: flag needs an argument: --currency; Run 'quarry " + command + " --help' for usage.\n",
		})
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			newHome(t)

			exitCode, stdout, stderr := runCapture(context.Background(), c.args)

			assert.Equal(t, 2, exitCode)
			assert.Empty(t, stdout.String())
			assert.Equal(t, c.wantStderr, stderr.String())
		})
	}
}

func Test_run_usage_hint_names_the_matched_command(t *testing.T) {
	cases := []struct {
		name       string
		args       []string
		wantStderr string
	}{
		{name: "sql", args: []string{"sql", "--bogus"}, wantStderr: "quarry: unknown flag: --bogus; Run 'quarry sql --help' for usage.\n"},
		{name: "status", args: []string{"status", "--bogus"}, wantStderr: "quarry: unknown flag: --bogus; Run 'quarry status --help' for usage.\n"},
		{name: "accounts", args: []string{"accounts", "--bogus"}, wantStderr: "quarry: unknown flag: --bogus; Run 'quarry accounts --help' for usage.\n"},
		{name: "holdings", args: []string{"holdings", "--bogus"}, wantStderr: "quarry: unknown flag: --bogus; Run 'quarry holdings --help' for usage.\n"},
		{name: "networth", args: []string{"networth", "--bogus"}, wantStderr: "quarry: unknown flag: --bogus; Run 'quarry networth --help' for usage.\n"},
		{name: "acb", args: []string{"acb", "--bogus"}, wantStderr: "quarry: unknown flag: --bogus; Run 'quarry acb --help' for usage.\n"},
		{name: "spend", args: []string{"spend", "--bogus"}, wantStderr: "quarry: unknown flag: --bogus; Run 'quarry spend --help' for usage.\n"},
		{
			name: "cashflow", args: []string{"cashflow", "--bogus"},
			wantStderr: "quarry: unknown flag: --bogus; Run 'quarry cashflow --help' for usage.\n",
		},
		{
			name: "recurring", args: []string{"recurring", "--bogus"},
			wantStderr: "quarry: unknown flag: --bogus; Run 'quarry recurring --help' for usage.\n",
		},
		{
			name: "anomalies", args: []string{"anomalies", "--bogus"},
			wantStderr: "quarry: unknown flag: --bogus; Run 'quarry anomalies --help' for usage.\n",
		},
		{
			name: "summary", args: []string{"summary", "--bogus"},
			wantStderr: "quarry: unknown flag: --bogus; Run 'quarry summary --help' for usage.\n",
		},
		{
			name: "search", args: []string{"search", "--bogus"},
			wantStderr: "quarry: unknown flag: --bogus; Run 'quarry search --help' for usage.\n",
		},
		{
			name: "snapshots", args: []string{"snapshots", "--bogus"},
			wantStderr: "quarry: unknown flag: --bogus; Run 'quarry snapshots --help' for usage.\n",
		},
		{name: "mcp", args: []string{"mcp", "--bogus"}, wantStderr: "quarry: unknown flag: --bogus; Run 'quarry mcp --help' for usage.\n"},
		{
			name: "claude install", args: []string{"claude", "install", "--bogus"},
			wantStderr: "quarry: unknown flag: --bogus; Run 'quarry claude install --help' for usage.\n",
		},
		{
			name: "claude uninstall", args: []string{"claude", "uninstall", "--bogus"},
			wantStderr: "quarry: unknown flag: --bogus; Run 'quarry claude uninstall --help' for usage.\n",
		},
		{name: "root", args: []string{"spending"}, wantStderr: "quarry: unknown command \"spending\" for \"quarry\"; Run 'quarry --help' for usage.\n"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			newHome(t)

			exitCode, stdout, stderr := runCapture(context.Background(), c.args)

			assert.Equal(t, 2, exitCode)
			assert.Empty(t, stdout.String())
			assert.Equal(t, c.wantStderr, stderr.String())
		})
	}
}

// Home resolution happens only inside sync's RunE, on a valid sync
// invocation with HOME unset.
func Test_run_reports_exit_1_when_home_directory_cannot_be_resolved(t *testing.T) {
	t.Setenv("HOME", "")

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"sync"})

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout.String())
	assert.Equal(t, "quarry: cannot find your home directory ($HOME is not set); set HOME, then run quarry sync again\n",
		stderr.String())
}

func Test_run_help_and_usage_errors_do_not_need_home(t *testing.T) {
	t.Setenv("HOME", "")

	for _, c := range []struct {
		name string
		args []string
	}{
		{name: "root help", args: []string{"--help"}},
		{name: "sync help", args: []string{"sync", "--help"}},
		{name: "snapshots help", args: []string{"snapshots", "--help"}},
		{name: "mcp help", args: []string{"mcp", "--help"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			exitCode, stdout, stderr := runCapture(context.Background(), c.args)

			assert.Equal(t, 0, exitCode)
			assert.NotEmpty(t, stdout.String())
			assert.Empty(t, stderr.String())
		})
	}

	t.Run("claude install help", func(t *testing.T) {
		var stdout, stderr bytes.Buffer

		exitCode := run(context.Background(), []string{"claude", "install", "--help"}, &stdout, &stderr)

		assert.Equal(t, 0, exitCode)
		assert.NotEmpty(t, stdout.String())
		assert.Empty(t, stderr.String())
	})

	t.Run("claude uninstall help", func(t *testing.T) {
		var stdout, stderr bytes.Buffer

		exitCode := run(context.Background(), []string{"claude", "uninstall", "--help"}, &stdout, &stderr)

		assert.Equal(t, 0, exitCode)
		assert.NotEmpty(t, stdout.String())
		assert.Empty(t, stderr.String())
	})

	t.Run("unknown command is still a usage error, not the home-directory refusal", func(t *testing.T) {
		exitCode, stdout, stderr := runCapture(context.Background(), []string{"frob"})

		assert.Equal(t, 2, exitCode)
		assert.Empty(t, stdout.String())
		assert.Equal(t, "quarry: unknown command \"frob\" for \"quarry\"; Run 'quarry --help' for usage.\n", stderr.String())
	})

	usageErrors := []struct {
		name       string
		args       []string
		stdin      string
		wantStderr string
	}{
		{name: "sql without a query", args: []string{"sql"}, wantStderr: sqlNeedsAQuery},
		{name: "sql - with an empty stdin", args: []string{"sql", "-"}, wantStderr: sqlNeedsAQuery},
		{name: "sql with a negative limit", args: []string{"sql", "--limit", "-1", "SELECT 1"}, wantStderr: "quarry: --limit must be 0 or more; 0 prints every row\n"},
		{name: "status with an argument", args: []string{"status", "extra"}, wantStderr: "quarry: status takes no arguments\n"},
		{name: "sql with an unknown flag", args: []string{"sql", "--bogus"}, wantStderr: "quarry: unknown flag: --bogus; Run 'quarry sql --help' for usage.\n"},
	}
	for _, c := range usageErrors {
		t.Run(c.name, func(t *testing.T) {
			exitCode, stdout, stderr := runCaptureWithStdin(c.args, c.stdin)

			assert.Equal(t, 2, exitCode)
			assert.Empty(t, stdout.String())
			assert.Equal(t, c.wantStderr, stderr.String())
		})
	}
}

const sqlNeedsAQuery = "quarry: sql needs a query; pass it as one quoted argument, or - to read it from stdin\n"

func Test_run_reports_exit_1_when_the_context_is_already_cancelled(t *testing.T) {
	home := newHome(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	exitCode, stdout, stderr := runCapture(ctx, []string{"sync", "--quicken", filepath.Join(home, "Any.quicken")})

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout.String())
	assert.Equal(t, "quarry: sync interrupted; nothing was kept; run quarry sync again\n", stderr.String())
}

func Test_runProcess_returns_the_usage_exit_code_for_an_unknown_command(t *testing.T) {
	var stdout, stderr bytes.Buffer

	exitCode := runProcess(t.Context(), []string{"frob"}, &stdout, &stderr)

	assert.Equal(t, 2, exitCode)
	assert.Equal(t, "quarry: unknown command \"frob\" for \"quarry\"; Run 'quarry --help' for usage.\n", stderr.String())
}

// runCaptureWithStdin is runCapture with stdin as the command's standard input.
func runCaptureWithStdin(args []string, stdin string) (int, *bytes.Buffer, *bytes.Buffer) {
	var stdout, stderr bytes.Buffer
	env := testEnv(&stdout, &stderr)
	env.Stdin = strings.NewReader(stdin)
	return runWith(context.Background(), args, env), &stdout, &stderr
}

const (
	badCurrencyFlag   = "quarry: --currency must be CAD, USD or native\n"
	claudeInstallArgs = "quarry: install takes no arguments; Run 'quarry claude install --help' for usage.\n"

	claudeUninstallArgs = "quarry: uninstall takes no arguments; Run 'quarry claude uninstall --help' for usage.\n"
)

func Test_run_read_commands_reject_bad_usage(t *testing.T) {
	const (
		u6 = "quarry: sql takes one query; quote it as one argument\n"
		u7 = "quarry: --limit must be 0 or more; 0 prints every row\n"
		u9 = "quarry: unknown flag: -- note; Run 'quarry sql --help' for usage.\n"
	)
	home := newHome(t)
	syncAccountsFixture(t, home)
	cases := []struct {
		name       string
		args       []string
		stdin      string
		wantStderr string
	}{
		{name: "sql without a query", args: []string{"sql"}, wantStderr: sqlNeedsAQuery},
		{name: "sql with a blank query", args: []string{"sql", "   "}, wantStderr: sqlNeedsAQuery},
		{name: "sql - with empty stdin", args: []string{"sql", "-"}, wantStderr: sqlNeedsAQuery},
		{name: "sql with only a semicolon", args: []string{"sql", ";"}, wantStderr: sqlNeedsAQuery},
		{name: "sql with only a comment", args: []string{"sql", "--", "-- note"}, wantStderr: sqlNeedsAQuery},
		{name: "sql with a query that starts with a dash", args: []string{"sql", "-- note"}, wantStderr: u9},
		{name: "sql with two arguments", args: []string{"sql", "SELECT 1", "extra"}, wantStderr: u6},
		{name: "sql with a negative limit", args: []string{"sql", "--limit", "-1", "SELECT 1"}, wantStderr: u7},
		{name: "spend with an empty currency", args: []string{"spend", "--currency="}, wantStderr: badCurrencyFlag},
		{name: "spend with a bad currency and a bad grouping", args: []string{"spend", "--currency", "EUR", "--by", "bogus"}, wantStderr: badCurrencyFlag},
		{name: "acb with another currency", args: []string{"acb", "--currency", "EUR"}, wantStderr: "quarry: acb is in CAD only, as the CRA requires; run it without --currency\n"},
		{name: "status with an argument", args: []string{"status", "extra"}, wantStderr: "quarry: status takes no arguments\n"},
		{name: "accounts with an argument", args: []string{"accounts", "extra"}, wantStderr: "quarry: accounts takes no arguments\n"},
		{name: "holdings with an argument", args: []string{"holdings", "extra"}, wantStderr: "quarry: holdings takes no arguments\n"},
		{name: "networth with an argument", args: []string{"networth", "extra"}, wantStderr: "quarry: networth takes no arguments\n"},
		{name: "acb with an argument", args: []string{"acb", "extra"}, wantStderr: "quarry: acb takes no arguments\n"},
		{name: "spend with an argument", args: []string{"spend", "extra"}, wantStderr: "quarry: spend takes no arguments\n"},
		{name: "cashflow with an argument", args: []string{"cashflow", "extra"}, wantStderr: "quarry: cashflow takes no arguments\n"},
		{name: "recurring with an argument", args: []string{"recurring", "extra"}, wantStderr: "quarry: recurring takes no arguments\n"},
		{name: "anomalies with an argument", args: []string{"anomalies", "extra"}, wantStderr: "quarry: anomalies takes no arguments\n"},
		{name: "summary with an argument", args: []string{"summary", "extra"}, wantStderr: "quarry: summary takes no arguments\n"},
		{name: "mcp with an argument", args: []string{"mcp", "extra"}, wantStderr: "quarry: mcp takes no arguments\n"},
		{name: "mcp with --json", args: []string{"mcp", "--json"}, wantStderr: "quarry: mcp always speaks JSON on stdout; drop --json\n"},
		{name: "claude install with an argument", args: []string{"claude", "install", "extra"}, wantStderr: claudeInstallArgs},
		{name: "claude install with --json", args: []string{"claude", "install", "--json"}, wantStderr: "quarry: claude install prints no JSON; drop --json\n"},
		{name: "claude install with --json and an argument", args: []string{"claude", "install", "--json", "extra"}, wantStderr: claudeInstallArgs},
		{name: "claude uninstall with an argument", args: []string{"claude", "uninstall", "extra"}, wantStderr: claudeUninstallArgs},
		{name: "claude uninstall with --json", args: []string{"claude", "uninstall", "--json"}, wantStderr: "quarry: claude uninstall prints no JSON; drop --json\n"},
		{name: "claude uninstall with --json and an argument", args: []string{"claude", "uninstall", "--json", "extra"}, wantStderr: claudeUninstallArgs},
		{
			name: "findings with an argument", args: []string{"findings", "duplicate:txn-1+txn-2"},
			wantStderr: "quarry: findings takes no arguments; to ignore a finding add its id to findings.ignore in " +
				"~/Library/Application Support/quarry/config.toml; Run 'quarry findings --help' for usage.\n",
		},
		{name: "findings with a bad status", args: []string{"findings", "--status", "closed"}, wantStderr: "quarry: --status must be open, ignored, fixed or all\n"},
		{
			name: "findings with a bad type", args: []string{"findings", "--type", "duplicates"},
			wantStderr: "quarry: --type must be duplicate, one-sided-transfer, unlinked-transfer, uncategorized, mixed-categories, " +
				"payee-variants, similar-categories, unused-category, unclassified-account or shares-without-cost\n",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			exitCode, stdout, stderr := runCaptureWithStdin(c.args, c.stdin)

			assert.Equal(t, 2, exitCode)
			assert.Empty(t, stdout.String())
			assert.Equal(t, c.wantStderr, stderr.String())
		})
	}
}

func Test_run_read_commands_refuse_a_bad_currency_flag(t *testing.T) {
	for _, command := range []string{"spend", "cashflow", "recurring", "anomalies", "accounts", "holdings", "networth", "summary"} {
		t.Run(command, func(t *testing.T) {
			newHome(t)

			exitCode, stdout, stderr := runCapture(context.Background(), []string{command, "--currency", "EUR"})

			assert.Equal(t, 2, exitCode)
			assert.Empty(t, stdout.String())
			assert.Equal(t, badCurrencyFlag, stderr.String())
		})
	}
}

func Test_run_sql_refuses_a_multi_line_query_that_starts_with_a_dash_as_an_unknown_flag(t *testing.T) {
	newHome(t)

	exitCode, stdout, stderr := runCaptureWithStdin([]string{"sql", "-- monthly totals\nSELECT 1"}, "")

	assert.Equal(t, 2, exitCode)
	assert.Empty(t, stdout.String())
	assert.True(t, strings.HasSuffix(stderr.String(), "; Run 'quarry sql --help' for usage.\n"), stderr.String())
}

func Test_run_findings_rejects_a_bad_status_before_looking_for_a_store(t *testing.T) {
	newHome(t)

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"findings", "--status", "closed"})

	assert.Equal(t, 2, exitCode)
	assert.Empty(t, stdout.String())
	assert.Equal(t, "quarry: --status must be open, ignored, fixed or all\n", stderr.String())
}

func Test_run_spend_refuses_a_bad_currency_flag_before_reading_a_bad_config(t *testing.T) {
	home := newHome(t)
	writeConfig(t, home, "reporting.currency = \"EUR\"\n")

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"spend", "--currency", "EUR"})

	assert.Equal(t, 2, exitCode)
	assert.Empty(t, stdout.String())
	assert.Equal(t, badCurrencyFlag, stderr.String())
}
