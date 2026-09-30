// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_run_read_commands_reject_bad_usage(t *testing.T) {
	const (
		u5 = "quarry: sql needs a query; pass it as one quoted argument, or - to read it from stdin\n"
		u6 = "quarry: sql takes one query; quote it as one argument\n"
		u7 = "quarry: --limit must be 0 or more; 0 prints every row\n"
		u9 = "quarry: unknown flag: -- note; Run 'quarry sql --help' for usage.\n"
	)
	home := t.TempDir()
	t.Setenv("HOME", home)
	syncAccountsFixture(t, home)
	cases := []struct {
		name       string
		args       []string
		stdin      string
		wantStderr string
	}{
		{name: "sql without a query", args: []string{"sql"}, wantStderr: u5},
		{name: "sql with a blank query", args: []string{"sql", "   "}, wantStderr: u5},
		{name: "sql - with empty stdin", args: []string{"sql", "-"}, wantStderr: u5},
		{name: "sql with only a semicolon", args: []string{"sql", ";"}, wantStderr: u5},
		{name: "sql with only a comment", args: []string{"sql", "--", "-- note"}, wantStderr: u5},
		{name: "sql with a query that starts with a dash", args: []string{"sql", "-- note"}, wantStderr: u9},
		{name: "sql with two arguments", args: []string{"sql", "SELECT 1", "extra"}, wantStderr: u6},
		{name: "sql with a negative limit", args: []string{"sql", "--limit", "-1", "SELECT 1"}, wantStderr: u7},
		{name: "status with an argument", args: []string{"status", "extra"}, wantStderr: "quarry: status takes no arguments\n"},
		{name: "accounts with an argument", args: []string{"accounts", "extra"}, wantStderr: "quarry: accounts takes no arguments\n"},
		{name: "spend with an argument", args: []string{"spend", "extra"}, wantStderr: "quarry: spend takes no arguments\n"},
		{name: "cashflow with an argument", args: []string{"cashflow", "extra"}, wantStderr: "quarry: cashflow takes no arguments\n"},
		{
			name: "findings with an argument", args: []string{"findings", "duplicate:txn-1+txn-2"},
			wantStderr: "quarry: findings takes no arguments; to ignore a finding add its id to findings.ignore in " +
				"~/Library/Application Support/quarry/config.toml; Run 'quarry findings --help' for usage.\n",
		},
		{name: "findings with a bad status", args: []string{"findings", "--status", "closed"}, wantStderr: "quarry: --status must be open, ignored, fixed or all\n"},
		{
			name: "findings with a bad type", args: []string{"findings", "--type", "duplicates"},
			wantStderr: "quarry: --type must be duplicate, one-sided-transfer, unlinked-transfer, uncategorized, mixed-categories, " +
				"payee-variants, similar-categories or unused-category\n",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			env := defaultEnv(&stdout, &stderr)
			env.Stdin = strings.NewReader(c.stdin)

			exitCode := runWith(context.Background(), c.args, env)

			assert.Equal(t, 2, exitCode)
			assert.Empty(t, stdout.String())
			assert.Equal(t, c.wantStderr, stderr.String())
		})
	}
}

func Test_run_sql_refuses_a_multi_line_query_that_starts_with_a_dash_as_an_unknown_flag(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var stdout, stderr bytes.Buffer
	env := defaultEnv(&stdout, &stderr)

	exitCode := runWith(context.Background(), []string{"sql", "-- monthly totals\nSELECT 1"}, env)

	assert.Equal(t, 2, exitCode)
	assert.Empty(t, stdout.String())
	assert.True(t, strings.HasSuffix(stderr.String(), "; Run 'quarry sql --help' for usage.\n"), stderr.String())
}

func Test_run_findings_rejects_a_bad_status_before_looking_for_a_store(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"findings", "--status", "closed"}, &stdout, &stderr)

	assert.Equal(t, 2, exitCode)
	assert.Empty(t, stdout.String())
	assert.Equal(t, "quarry: --status must be open, ignored, fixed or all\n", stderr.String())
}
