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
		{name: "sql with two arguments", args: []string{"sql", "SELECT 1", "extra"}, wantStderr: u6},
		{name: "sql with a negative limit", args: []string{"sql", "--limit", "-1", "SELECT 1"}, wantStderr: u7},
		{name: "status with an argument", args: []string{"status", "extra"}, wantStderr: "quarry: status takes no arguments\n"},
		{name: "accounts with an argument", args: []string{"accounts", "extra"}, wantStderr: "quarry: accounts takes no arguments\n"},
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
