// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_run_sync_help_names_both_documents_folders(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sync", "--help"}, &stdout, &stderr)

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
	home := t.TempDir()
	t.Setenv("HOME", home)
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
		"cent, and every transaction must equal the sum of its splits; if a check\n"+
		"fails, the previous store is left unchanged.")
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
			home := t.TempDir()
			t.Setenv("HOME", home)
			v9fixture.OpenBundle(t, filepath.Join(home, "Documents"))
			var stdout, stderr bytes.Buffer

			exitCode := run(context.Background(), c.args, &stdout, &stderr)

			assert.Equal(t, 2, exitCode)
			assert.Empty(t, stdout.String())
			assert.Equal(t, c.wantStderr, stderr.String())
			_, statErr := os.Stat(filepath.Join(home, "Library", "Application Support", "quarry", "snapshots"))
			assert.ErrorIs(t, statErr, os.ErrNotExist)
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
			home := t.TempDir()
			t.Setenv("HOME", home)
			v9fixture.OpenBundle(t, filepath.Join(home, "Documents"))
			var stdout, stderr bytes.Buffer

			exitCode := run(context.Background(), c.args, &stdout, &stderr)

			assert.Equal(t, 2, exitCode)
			assert.Empty(t, stdout.String())
			assert.Equal(t, "quarry: flag needs an argument: --quicken; Run 'quarry sync --help' for usage.\n", stderr.String())
			_, statErr := os.Stat(filepath.Join(home, "Library", "Application Support", "quarry", "snapshots"))
			assert.ErrorIs(t, statErr, os.ErrNotExist)
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
			t.Setenv("HOME", t.TempDir())
			var stdout, stderr bytes.Buffer

			exitCode := run(context.Background(), c.args, &stdout, &stderr)

			require.Equal(t, 2, exitCode)
			assert.Empty(t, stdout.String())
			assert.Equal(t, c.wantStderr, stderr.String())
			assert.NotContains(t, stderr.String(), "Did you mean")
		})
	}
}

func Test_run_read_commands_need_a_value_for_the_currency_flag(t *testing.T) {
	for _, command := range []string{"spend", "cashflow", "recurring", "anomalies", "accounts"} {
		t.Run(command, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			var stdout, stderr bytes.Buffer

			exitCode := run(context.Background(), []string{command, "--currency"}, &stdout, &stderr)

			assert.Equal(t, 2, exitCode)
			assert.Empty(t, stdout.String())
			assert.Equal(t, "quarry: flag needs an argument: --currency; Run 'quarry "+command+" --help' for usage.\n", stderr.String())
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
			name: "search", args: []string{"search", "--bogus"},
			wantStderr: "quarry: unknown flag: --bogus; Run 'quarry search --help' for usage.\n",
		},
		{
			name: "snapshots", args: []string{"snapshots", "--bogus"},
			wantStderr: "quarry: unknown flag: --bogus; Run 'quarry snapshots --help' for usage.\n",
		},
		{name: "mcp", args: []string{"mcp", "--bogus"}, wantStderr: "quarry: unknown flag: --bogus; Run 'quarry mcp --help' for usage.\n"},
		{name: "root", args: []string{"spending"}, wantStderr: "quarry: unknown command \"spending\" for \"quarry\"; Run 'quarry --help' for usage.\n"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			var stdout, stderr bytes.Buffer

			exitCode := run(context.Background(), c.args, &stdout, &stderr)

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
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sync"}, &stdout, &stderr)

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout.String())
	assert.Equal(t, "quarry: cannot find your home directory ($HOME is not set); set HOME, then run quarry sync again\n",
		stderr.String())
}

func Test_run_help_and_usage_errors_do_not_need_home(t *testing.T) {
	t.Setenv("HOME", "")

	t.Run("root help", func(t *testing.T) {
		var stdout, stderr bytes.Buffer

		exitCode := run(context.Background(), []string{"--help"}, &stdout, &stderr)

		assert.Equal(t, 0, exitCode)
		assert.NotEmpty(t, stdout.String())
		assert.Empty(t, stderr.String())
	})

	t.Run("sync help", func(t *testing.T) {
		var stdout, stderr bytes.Buffer

		exitCode := run(context.Background(), []string{"sync", "--help"}, &stdout, &stderr)

		assert.Equal(t, 0, exitCode)
		assert.NotEmpty(t, stdout.String())
		assert.Empty(t, stderr.String())
	})

	t.Run("snapshots help", func(t *testing.T) {
		var stdout, stderr bytes.Buffer

		exitCode := run(context.Background(), []string{"snapshots", "--help"}, &stdout, &stderr)

		assert.Equal(t, 0, exitCode)
		assert.NotEmpty(t, stdout.String())
		assert.Empty(t, stderr.String())
	})

	t.Run("mcp help", func(t *testing.T) {
		var stdout, stderr bytes.Buffer

		exitCode := run(context.Background(), []string{"mcp", "--help"}, &stdout, &stderr)

		assert.Equal(t, 0, exitCode)
		assert.NotEmpty(t, stdout.String())
		assert.Empty(t, stderr.String())
	})

	t.Run("unknown command is still a usage error, not the home-directory refusal", func(t *testing.T) {
		var stdout, stderr bytes.Buffer

		exitCode := run(context.Background(), []string{"frob"}, &stdout, &stderr)

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
			var stdout, stderr bytes.Buffer
			env := testEnv(&stdout, &stderr)
			env.Stdin = strings.NewReader(c.stdin)

			exitCode := runWith(context.Background(), c.args, env)

			assert.Equal(t, 2, exitCode)
			assert.Empty(t, stdout.String())
			assert.Equal(t, c.wantStderr, stderr.String())
		})
	}
}

const sqlNeedsAQuery = "quarry: sql needs a query; pass it as one quoted argument, or - to read it from stdin\n"

func Test_run_reports_exit_1_when_the_context_is_already_cancelled(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var stdout, stderr bytes.Buffer

	exitCode := run(ctx, []string{"sync", "--quicken", filepath.Join(home, "Any.quicken")}, &stdout, &stderr)

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
