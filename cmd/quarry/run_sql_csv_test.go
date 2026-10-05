// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"bytes"
	"context"
	"encoding/csv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_run_sql_csv_prints_null_as_an_empty_field_and_an_empty_string_as_a_quoted_pair(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	syncAccountsFixture(t, home)
	const query = `SELECT source_id, holdings_value,
		CASE WHEN source_id = 1 THEN '' WHEN source_id = 2 THEN 'US, "x"' END AS note
		FROM v_account_balances ORDER BY source_id`
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sql", "--csv", query}, &stdout, &stderr)

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, ""+
		"source_id,holdings_value,note\n"+
		"1,,\"\"\n"+
		"2,,\"US, \"\"x\"\"\"\n"+
		"3,,\n"+
		"4,0.00,\n"+
		"5,,\n",
		stdout.String())
}

func Test_run_sql_csv_writes_a_row_of_one_null_column_as_a_quoted_pair_a_csv_reader_keeps(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	syncAccountsFixture(t, home)
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sql", "--csv", "SELECT holdings_value FROM v_account_balances WHERE source_id = 1"}, &stdout, &stderr)

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Equal(t, "holdings_value\n\"\"\n", stdout.String())
	records, err := csv.NewReader(strings.NewReader(stdout.String())).ReadAll()
	require.NoError(t, err)
	assert.Equal(t, [][]string{{"holdings_value"}, {""}}, records)
}

func Test_run_sql_csv_with_json_exits_2_naming_the_two_flags(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sql", "--csv", "--json", "SELECT 1"}, &stdout, &stderr)

	assert.Equal(t, 2, exitCode)
	assert.Empty(t, stdout.String())
	assert.Equal(t, "quarry: --csv and --json cannot be used together; choose one output format\n", stderr.String())
}
