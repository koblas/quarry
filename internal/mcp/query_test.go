package mcp_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_query_refuses_blank_sql_without_touching_the_store(t *testing.T) {
	cases := map[string]string{
		"spaces":            "   ",
		"tabs and newlines": "\t\n \r\n",
	}

	for name, sql := range cases {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t, &fakeStore{result: rowsOf(1)}, nil)

			result := h.query(t, map[string]any{"sql": sql})

			assert.True(t, result.IsError)
			assert.Equal(t, blankSQLLine, textOf(t, result))
			assert.Empty(t, h.store.asked)
			assert.Zero(t, h.built)
			assert.Equal(t, logPrefixQuery+blankSQLLine+"\n", h.stderr.String())
		})
	}
}

func Test_query_asks_the_store_for_the_effective_limit(t *testing.T) {
	cases := []struct {
		name      string
		arguments map[string]any
		want      int
	}{
		{name: "limit omitted is 500, asked as 501", arguments: map[string]any{"sql": "SELECT 1"}, want: 501},
		{name: "limit 1 is asked as 2", arguments: map[string]any{"sql": "SELECT 1", "limit": 1}, want: 2},
		{name: "limit 500 is asked as 501", arguments: map[string]any{"sql": "SELECT 1", "limit": 500}, want: 501},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t, &fakeStore{result: rowsOf(1)}, nil)

			result := h.query(t, c.arguments)

			require.False(t, result.IsError, textOf(t, result))
			assert.Equal(t, []int{c.want}, h.store.asked)
		})
	}
}

func Test_query_refuses_a_limit_the_schema_rejects_without_touching_the_store(t *testing.T) {
	cases := map[string]any{
		"zero":             0,
		"above the cap":    501,
		"a string":         "5",
		"not an integer":   2.5,
		"negative integer": -1,
	}

	for name, limit := range cases {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t, &fakeStore{result: rowsOf(1)}, nil)

			result := h.query(t, map[string]any{"sql": "SELECT 1", "limit": limit})

			assert.True(t, result.IsError)
			assert.Empty(t, h.store.asked)
			assert.Zero(t, h.built)
			assert.Equal(t, logPrefixQuery+argsRefusedLog+"\n", h.stderr.String())
		})
	}
}

func Test_query_over_its_limit_returns_the_first_row_and_says_so_in_the_singular(t *testing.T) {
	h := newHarness(t, &fakeStore{result: rowsOf(2)}, nil)

	result := h.query(t, map[string]any{"sql": "SELECT n FROM t", "limit": 1})

	require.False(t, result.IsError, textOf(t, result))
	var doc struct {
		RowCount  int      `json:"row_count"`
		Limit     int      `json:"limit"`
		Truncated bool     `json:"truncated"`
		Warnings  []string `json:"warnings"`
	}
	require.NoError(t, json.Unmarshal([]byte(textOf(t, result)), &doc))
	assert.Equal(t, 1, doc.RowCount)
	assert.Equal(t, 1, doc.Limit)
	assert.True(t, doc.Truncated)
	assert.Equal(t, []string{"returned the first 1 row; the query has more; aggregate or filter in SQL to see the rest"}, doc.Warnings)
}

func Test_query_with_arguments_missing_sql_is_refused_not_panicked(t *testing.T) {
	cases := map[string]any{
		"omitted": nil,
		"null":    json.RawMessage("null"),
		"empty":   map[string]any{},
	}

	for name, arguments := range cases {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t, &fakeStore{result: rowsOf(1)}, nil)

			result := h.query(t, arguments)

			assert.True(t, result.IsError)
			assert.Empty(t, h.store.asked)
			assert.Equal(t, logPrefixQuery+argsRefusedLog+"\n", h.stderr.String())
		})
	}
}

func Test_query_builds_a_fresh_report_server_for_every_call(t *testing.T) {
	h := newHarness(t, &fakeStore{result: rowsOf(1)}, nil)

	h.query(t, map[string]any{"sql": "SELECT 1"})
	h.query(t, map[string]any{"sql": "SELECT 2"})

	assert.Equal(t, []string{"mcp", "mcp"}, h.commands)
}

func Test_query_refuses_with_the_report_factory_text_when_it_fails(t *testing.T) {
	h := newHarness(t, &fakeStore{result: rowsOf(1)}, errNoHome)

	result := h.query(t, map[string]any{"sql": "SELECT 1"})

	assert.True(t, result.IsError)
	assert.Equal(t, errNoHome.Error(), textOf(t, result))
	assert.Equal(t, logPrefixQuery+failedLogLine+"\n", h.stderr.String())
	assert.Empty(t, h.store.asked)
}

func Test_query_returns_a_store_fault_it_cannot_classify_unchanged(t *testing.T) {
	h := newHarness(t, &fakeStore{err: errDiskOnFire}, nil)

	result := h.query(t, map[string]any{"sql": "SELECT 1"})

	assert.True(t, result.IsError)
	assert.Equal(t, "disk on fire", textOf(t, result))
	assert.Equal(t, logPrefixQuery+failedLogLine+"\n", h.stderr.String())
}
