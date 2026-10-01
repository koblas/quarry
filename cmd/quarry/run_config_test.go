// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// countFilesWithSuffix is how many entries in dir end in suffix.
func countFilesWithSuffix(t *testing.T, dir, suffix string) int {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	count := 0
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), suffix) {
			count++
		}
	}
	return count
}

const (
	configShown = "~/Library/Application Support/quarry/config.toml"
	configFix   = "; fix the file and run the command again"

	combinedCarryWarning = "cannot carry import history and findings forward from the previous store (the file is not a DuckDB database); " +
		"both start again with this sync"
)

// corruptPreviousStore syncs once, then replaces the store with bytes DuckDB cannot open.
func corruptPreviousStore(t *testing.T, home string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	require.Equal(t, 0, run(context.Background(), []string{"sync"}, &stdout, &stderr), stderr.String())
	require.NoError(t, os.WriteFile(storePathUnder(home), []byte("this is not a database"), 0o600))
}

// configPath is the config file's absolute path under home, as --json names it.
func configPath(home string) string { return filepath.Join(storeDirUnder(home), "config.toml") }

// writeConfig writes content as the config file under home.
func writeConfig(t *testing.T, home, content string) {
	t.Helper()
	dir := storeDirUnder(home)
	require.NoError(t, os.MkdirAll(dir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.toml"), []byte(content), 0o600))
}

// fileDigest is the hex SHA-256 of the file at path.
func fileDigest(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return fmt.Sprintf("%x", sha256.Sum256(data))
}

func Test_run_sync_refuses_a_malformed_config_before_taking_a_snapshot(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeStatusFixtureBundle(t, home)
	quarryDir := storeDirUnder(home)
	snapshotsDir := filepath.Join(quarryDir, "snapshots")
	var goodStdout, goodStderr bytes.Buffer
	require.Equal(t, 0, run(context.Background(), []string{"sync"}, &goodStdout, &goodStderr), goodStderr.String())
	storeBefore := fileDigest(t, filepath.Join(quarryDir, "quarry.duckdb"))
	require.NoError(t, os.WriteFile(filepath.Join(quarryDir, "config.toml"), []byte("[snapshots\nkeep = 24\n"), 0o600))
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sync"}, &stdout, &stderr)

	assert.Equal(t, 1, countFilesWithSuffix(t, snapshotsDir, ".sqlite"))
	assert.Equal(t, storeBefore, fileDigest(t, filepath.Join(quarryDir, "quarry.duckdb")))
	require.Equal(t, 1, exitCode, stderr.String())
	assert.Empty(t, stdout.String())
	assert.Regexp(t, "^"+regexp.QuoteMeta("quarry: cannot read ~/Library/Application Support/quarry/config.toml: line 1: ")+
		"[^\n]+"+regexp.QuoteMeta("; fix the file and run the command again")+"\n$", stderr.String())
}

func Test_run_sync_refuses_a_relative_quicken_path(t *testing.T) {
	cases := []struct {
		name string
		args func(bundleDir string) []string
	}{
		{name: "bare sync", args: func(string) []string { return []string{"sync"} }},
		{name: "with --quicken, which does not excuse the key", args: func(bundleDir string) []string { return []string{"sync", "--quicken", bundleDir} }},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			bundle := writeStatusFixtureBundle(t, home)
			writeConfig(t, home, "quicken.path = \"Home.quicken\"\n")
			var stdout, stderr bytes.Buffer

			exitCode := run(context.Background(), c.args(bundle.Dir), &stdout, &stderr)

			assert.Empty(t, stdout.String())
			assert.Equal(t, "quarry: "+configShown+": quicken.path must be a full path or start with ~/, got \"Home.quicken\""+configFix+"\n", stderr.String())
			assert.Equal(t, 1, exitCode)
		})
	}
}

func Test_run_sync_refuses_a_quicken_path_that_is_not_a_string(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{name: "bare sync", args: []string{"sync"}},
		{name: "with --from, which never reads Quicken", args: []string{"sync", "--from", "20260927T143005Z"}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			writeConfig(t, home, "quicken.path = 12\n")
			var stdout, stderr bytes.Buffer

			exitCode := run(context.Background(), c.args, &stdout, &stderr)

			assert.Empty(t, stdout.String())
			assert.Equal(t, "quarry: "+configShown+": quicken.path must be a path in quotes, got 12"+configFix+"\n", stderr.String())
			assert.Equal(t, 1, exitCode)
		})
	}
}

func Test_run_sync_refuses_a_bad_config_value_with_the_ruled_copy(t *testing.T) {
	cases := []struct {
		name    string
		content string
		line    string
	}{
		{
			name: "snapshots.keep of zero", content: "snapshots.keep = 0\n",
			line: configShown + ": snapshots.keep must be a whole number of 1 or more, got 0",
		},
		{
			name: "snapshots.keep as a string", content: "snapshots.keep = \"twelve\"\n",
			line: configShown + ": snapshots.keep must be a whole number of 1 or more, got \"twelve\"",
		},
		{
			name: "snapshots.keep as a table", content: "[snapshots.keep]\nx = 1\n",
			line: configShown + ": snapshots.keep must be a whole number of 1 or more, got a table",
		},
		{
			name: "snapshots.keep over several lines", content: "snapshots.keep = [ 1,\n  2 ]\n",
			line: configShown + ": snapshots.keep must be a whole number of 1 or more, got [ 1, 2 ]",
		},
		{
			name: "quicken.path as a table", content: "[quicken.path]\nx = 1\n",
			line: configShown + ": quicken.path must be a path in quotes, got a table",
		},
		{
			name: "snapshots as a plain value", content: "snapshots = 3\n",
			line: configShown + ": snapshots must be a table, such as snapshots.keep = 12, got 3",
		},
		{
			name: "quicken as a plain value", content: "quicken = \"x\"\n",
			line: configShown + ": quicken must be a table, such as quicken.path = \"~/Documents/Home.quicken\", got \"x\"",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			writeConfig(t, home, c.content)
			var stdout, stderr bytes.Buffer

			exitCode := run(context.Background(), []string{"sync"}, &stdout, &stderr)

			assert.Empty(t, stdout.String())
			assert.Equal(t, "quarry: "+c.line+configFix+"\n", stderr.String())
			assert.Equal(t, 1, exitCode)
		})
	}
}

func Test_run_sync_refuses_a_config_it_cannot_read(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	require.NoError(t, os.MkdirAll(filepath.Join(storeDirUnder(home), "config.toml"), 0o700))
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sync"}, &stdout, &stderr)

	assert.Empty(t, stdout.String())
	assert.Equal(t, "quarry: cannot read "+configShown+": is a directory"+configFix+"\n", stderr.String())
	assert.Equal(t, 1, exitCode)
}

func Test_run_read_commands_ignore_a_malformed_config(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	replaceStore(t, home, cashFlowRows(
		[]store.Account{chequingAccount("acct-chq", 1)},
		spendSplit{id: "s01", account: "acct-chq", category: "cat-salary", currency: "CAD", day: day(2026, 3, 1), cents: 50000},
		spendSplit{id: "s02", account: "acct-chq", category: "cat-groceries", currency: "CAD", day: day(2026, 3, 10), cents: -12000},
		spendSplit{id: "s03", account: "acct-chq", category: "cat-groceries", payee: "payee-costco", currency: "CAD", day: day(2026, 7, 5), cents: -2000},
		spendSplit{id: "s04", account: "acct-chq", category: "cat-groceries", payee: "payee-costco", currency: "CAD", day: day(2026, 8, 5), cents: -2000},
		spendSplit{id: "s05", account: "acct-chq", category: "cat-groceries", payee: "payee-costco", currency: "CAD", day: day(2026, 9, 5), cents: -2000},
		spendSplit{id: "s06", account: "acct-chq", category: "cat-groceries", payee: "payee-bakery", currency: "CAD", day: day(2026, 3, 1), cents: -10000},
		spendSplit{id: "s07", account: "acct-chq", category: "cat-groceries", payee: "payee-bakery", currency: "CAD", day: day(2026, 4, 1), cents: -10000},
		spendSplit{id: "s08", account: "acct-chq", category: "cat-groceries", payee: "payee-bakery", currency: "CAD", day: day(2026, 5, 1), cents: -10000},
		spendSplit{id: "s09", account: "acct-chq", category: "cat-groceries", payee: "payee-bakery", currency: "CAD", day: day(2026, 6, 1), cents: -25000},
	))
	commands := map[string][]string{
		"accounts":  {"accounts"},
		"anomalies": {"anomalies", "--since", "2026-01", "--until", "2026-09"},
		"spend":     {"spend", "--since", "2026-01", "--until", "2026-09"},
		"cashflow":  {"cashflow", "--since", "2026-01", "--until", "2026-09"},
		"recurring": {"recurring", "--since", "2026-01", "--until", "2026-09"},
		"sql":       {"sql", "SELECT name FROM accounts"},
	}
	before := map[string]string{}
	for name, args := range commands {
		var stdout, stderr bytes.Buffer
		require.Equal(t, 0, runWith(context.Background(), args, spendEnv(&stdout, &stderr)), stderr.String())
		before[name] = stdout.String()
	}
	require.Contains(t, before["anomalies"], "Bakery")
	writeConfig(t, home, "[snapshots\nkeep = 24\n")

	for name, args := range commands {
		t.Run(name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer

			exitCode := runWith(context.Background(), args, spendEnv(&stdout, &stderr))

			assert.Equal(t, before[name], stdout.String())
			assert.Empty(t, stderr.String())
			assert.Equal(t, 0, exitCode)
		})
	}
}

func Test_run_findings_refuses_a_bad_config_before_looking_for_a_store(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeConfig(t, home, "snapshots.keep = 0\n")
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"findings"}, &stdout, &stderr)

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout.String())
	assert.Equal(t, "quarry: "+configShown+": snapshots.keep must be a whole number of 1 or more, got 0"+configFix+"\n", stderr.String())
}

func Test_run_findings_lists_after_warning_about_an_unknown_config_key(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	replaceStore(t, home, spendRows([]store.Account{chequingAccount("acct-1", 1)},
		spendSplit{id: "1", account: "acct-1", category: "cat-fuel", payee: "payee-costco", currency: "CAD", day: day(2026, 9, 1), cents: -4500}))
	writeConfig(t, home, "snapshot.keep = 3\n")
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"findings"}, &stdout, &stderr)

	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "No open findings\n", stdout.String())
	assert.Equal(t, "quarry: warning: "+configShown+": unknown key snapshot.keep; quarry ignores it\n", stderr.String())
}

func Test_run_sync_warns_about_unknown_config_keys_before_its_own_warnings(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeStatusFixtureBundle(t, home)
	corruptPreviousStore(t, home)
	writeConfig(t, home, "snapshot.keep = 3\n")
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sync"}, &stdout, &stderr)

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Equal(t, "quarry: warning: "+configShown+": unknown key snapshot.keep; quarry ignores it\n"+
		"quarry: warning: "+combinedCarryWarning+"\n",
		stderr.String())
}

func Test_run_sync_warns_about_a_config_key_that_differs_from_a_known_one_only_in_letter_case(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeStatusFixtureBundle(t, home)
	corruptPreviousStore(t, home)
	writeConfig(t, home, "[Snapshots]\nKeep = 50\n")
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sync"}, &stdout, &stderr)

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Equal(t, "quarry: warning: "+configShown+": unknown key Snapshots; quarry ignores it\n"+
		"quarry: warning: "+combinedCarryWarning+"\n",
		stderr.String())
}

func Test_run_sync_json_lists_config_warnings_before_its_own_without_the_prefix(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeStatusFixtureBundle(t, home)
	corruptPreviousStore(t, home)
	writeConfig(t, home, "snapshot.keep = 3\n")
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sync", "--json"}, &stdout, &stderr)

	require.Equal(t, 0, exitCode, stderr.String())
	var doc struct {
		Warnings []string `json:"warnings"`
	}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &doc))
	assert.Equal(t, []string{
		configPath(home) + ": unknown key snapshot.keep; quarry ignores it",
		combinedCarryWarning,
	}, doc.Warnings)
}

func Test_run_sync_json_lists_the_history_warning_after_the_config_warning_without_the_prefix(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeStatusFixtureBundle(t, home)
	corruptPreviousStore(t, home)
	writeConfig(t, home, "snapshot.keep = 3\n")
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sync", "--json"}, &stdout, &stderr)

	require.Equal(t, 0, exitCode, stderr.String())
	var doc struct {
		Warnings []string `json:"warnings"`
	}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &doc))
	assert.Equal(t, []string{
		configPath(home) + ": unknown key snapshot.keep; quarry ignores it",
		combinedCarryWarning,
	}, doc.Warnings)
}

func Test_run_sync_quotes_an_unknown_config_key_that_is_not_a_bare_key(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeStatusFixtureBundle(t, home)
	corruptPreviousStore(t, home)
	writeConfig(t, home, "\"snapshots.keep\" = 5\n")
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sync"}, &stdout, &stderr)

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Equal(t, "quarry: warning: "+configShown+": unknown key \"snapshots.keep\"; quarry ignores it\n"+
		"quarry: warning: "+combinedCarryWarning+"\n",
		stderr.String())
}

func Test_run_sync_json_lists_a_quoted_unknown_config_key_without_the_prefix(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeStatusFixtureBundle(t, home)
	corruptPreviousStore(t, home)
	writeConfig(t, home, "\"a\\nb\" = 1\n")
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sync", "--json"}, &stdout, &stderr)

	require.Equal(t, 0, exitCode, stderr.String())
	var doc struct {
		Warnings []string `json:"warnings"`
	}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &doc))
	assert.Equal(t, []string{
		configPath(home) + ": unknown key \"a\\nb\"; quarry ignores it",
		combinedCarryWarning,
	}, doc.Warnings)
}

func Test_run_sync_from_json_names_the_config_file_by_its_absolute_path_and_stderr_abbreviates_it(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	syncBundle(t, writeNamedAccountBundle(t, filepath.Join(home, "Documents"), "Chequing"))
	manifest := onlyFileWithSuffix(t, filepath.Join(storeDirUnder(home), "snapshots"), ".json")
	writeConfig(t, home, "snapshot.keep = 3\n")
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sync", "--from", strings.TrimSuffix(filepath.Base(manifest), ".json"), "--json"}, &stdout, &stderr)

	require.Equal(t, 0, exitCode, stderr.String())
	var doc struct {
		Warnings []string `json:"warnings"`
	}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &doc))
	assert.Equal(t, []string{configPath(home) + ": unknown key snapshot.keep; quarry ignores it"}, doc.Warnings)
	assert.Equal(t, "quarry: warning: "+configShown+": unknown key snapshot.keep; quarry ignores it\n", stderr.String())
}
