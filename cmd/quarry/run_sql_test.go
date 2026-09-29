package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_run_sql_prints_the_query_result_as_a_table(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	syncAccountsFixture(t, home)
	const query = `SELECT source_id AS id, name, balance,
		CASE WHEN source_id = 1 THEN 'line one' || chr(10) || 'tab' || chr(9) || 'cr' || chr(13) END AS note
		FROM v_account_balances ORDER BY source_id`
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sql", query}, &stdout, &stderr)

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, ""+
		"id  name            balance  note\n"+
		" 1  Chequing       12345.67  line one\\ntab\\tcr\\r\n"+
		" 2  US Chequing     8310.00  NULL\n"+
		" 3  Old Savings        0.00  NULL\n"+
		" 4  RRSP               NULL  NULL\n"+ //nolint:dupword // two adjacent NULL cells are the expected row
		" 5  Visa Infinite  -1204.17  NULL\n",
		stdout.String())
}

func Test_run_sql_prints_only_the_header_for_zero_rows(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	syncAccountsFixture(t, home)
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sql", "SELECT name, balance FROM v_account_balances WHERE false"}, &stdout, &stderr)

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Equal(t, "name  balance\n", stdout.String())
}

func Test_run_sql_prints_at_most_limit_rows(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	syncAccountsFixture(t, home)
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sql", "--limit", "2", "SELECT source_id FROM accounts ORDER BY source_id"}, &stdout, &stderr)

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Equal(t, "source_id\n        1\n        2\n", stdout.String())
}

func Test_run_sql_reports_a_query_error(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	syncAccountsFixture(t, home)
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sql", "SELECT missing_column FROM accounts"}, &stdout, &stderr)

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout.String())
	assert.True(t, strings.HasPrefix(stderr.String(), "quarry: run query: Binder Error: "), stderr.String())
}

func Test_run_sql_needs_exactly_one_argument(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{name: "no query", args: []string{"sql"}},
		{name: "two arguments", args: []string{"sql", "SELECT 1", "extra"}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			var stdout, stderr bytes.Buffer

			exitCode := run(context.Background(), c.args, &stdout, &stderr)

			assert.Equal(t, 2, exitCode)
			assert.Empty(t, stdout.String())
		})
	}
}
