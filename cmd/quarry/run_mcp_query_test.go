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

	"github.com/koblas/quarry/internal/cli"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	mcpBlankSQLLine       = "query needs SQL in the sql parameter"
	mcpWriteRefusal       = "query only reads quarry's store; it cannot change data. Fixes are made in Quicken, then the user runs quarry sync"
	mcpExternalRefusal    = "query reads only quarry's store; other files, databases and extensions are turned off"
	mcpUnprintableRefusal = `cannot print column "doc" of type JSON; cast it in the query, e.g. CAST(doc AS VARCHAR)`
	mcpBadColumnRefusal   = `query failed: Binder Error: Referenced column "missing_column" not found in FROM clause!`
)

func Test_run_mcp_query_returns_the_sql_json_document(t *testing.T) {
	const query = `SELECT 12.50::DECIMAL(18,2) AS amount,
		170141183460469231731687303715884105727::HUGEINT AS big,
		42::INTEGER AS count,
		DATE '2026-09-29' AS day,
		NULL::VARCHAR AS nothing`
	ctx, peer := newQueryPeer(t)
	cliStdout := runSQLJSON(ctx, t, query)

	result := callQuery(ctx, t, peer, map[string]any{"sql": query})
	require.NoError(t, peer.session.Close())
	peer.waitForExit(ctx, t)

	require.False(t, result.IsError, textOf(result))
	want := compactJSON(t, cliStdout)
	assert.Equal(t, want, textOf(result))
	assert.Equal(t, want, structuredFrame(t, peer.stdout.String()))
	assert.Empty(t, peer.stderr.String())
}

func Test_run_mcp_query_escapes_html_characters_like_the_cli(t *testing.T) {
	const query = `SELECT '<a&b>' AS html`
	ctx, peer := newQueryPeer(t)
	cliStdout := runSQLJSON(ctx, t, query)

	result := callQuery(ctx, t, peer, map[string]any{"sql": query})

	require.False(t, result.IsError, textOf(result))
	assert.Equal(t, compactJSON(t, cliStdout), textOf(result))
	assert.Contains(t, textOf(result), `"\u003ca\u0026b\u003e"`)
}

func Test_run_mcp_query_with_several_statements_returns_the_last_ones_rows(t *testing.T) {
	ctx, peer := newQueryPeer(t)

	result := callQuery(ctx, t, peer, map[string]any{"sql": "SELECT 1 AS a; SELECT 2 AS b"})

	require.False(t, result.IsError, textOf(result))
	var doc struct {
		Columns []struct {
			Name string `json:"name"`
		} `json:"columns"`
		Rows []json.RawMessage `json:"rows"`
	}
	require.NoError(t, json.Unmarshal([]byte(textOf(result)), &doc))
	require.Len(t, doc.Columns, 1)
	assert.Equal(t, "b", doc.Columns[0].Name)
	assert.JSONEq(t, "[2]", string(doc.Rows[0]))
}

func Test_run_mcp_query_over_its_limit_returns_the_first_rows_and_says_so(t *testing.T) {
	const cutTail = "; the query has more; aggregate or filter in SQL to see the rest"
	cases := []struct {
		name          string
		query         string
		limit         int // 0 leaves the argument out
		wantRows      int
		wantLimit     int
		wantTruncated bool
		wantWarnings  []string
	}{
		{
			name: "more rows than an explicit limit", query: "SELECT range AS n FROM range(10)", limit: 3,
			wantRows: 3, wantLimit: 3, wantTruncated: true,
			wantWarnings: []string{"returned the first 3 rows" + cutTail},
		},
		{
			name: "limit 1 is worded in the singular", query: "SELECT range AS n FROM range(2)", limit: 1,
			wantRows: 1, wantLimit: 1, wantTruncated: true,
			wantWarnings: []string{"returned the first 1 row" + cutTail},
		},
		{
			name: "more rows than the default limit", query: "SELECT range AS n FROM range(501)",
			wantRows: 500, wantLimit: 500, wantTruncated: true,
			wantWarnings: []string{"returned the first 500 rows" + cutTail},
		},
		{
			name: "exactly the limit is not truncated", query: "SELECT range AS n FROM range(3)", limit: 3,
			wantRows: 3, wantLimit: 3, wantWarnings: []string{},
		},
		{
			name: "no rows at all", query: "SELECT 1 AS n WHERE false",
			wantRows: 0, wantLimit: 500, wantWarnings: []string{},
		},
	}
	ctx, peer := newQueryPeer(t)

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			arguments := map[string]any{"sql": c.query}
			if c.limit != 0 {
				arguments["limit"] = c.limit
			}

			result := callQuery(ctx, t, peer, arguments)

			require.False(t, result.IsError, textOf(result))
			var doc struct {
				Rows      []json.RawMessage `json:"rows"`
				RowCount  int               `json:"row_count"`
				Limit     int               `json:"limit"`
				Truncated bool              `json:"truncated"`
				Warnings  []string          `json:"warnings"`
			}
			require.NoError(t, json.Unmarshal([]byte(textOf(result)), &doc))
			assert.NotNil(t, doc.Rows)
			assert.Len(t, doc.Rows, c.wantRows)
			assert.Equal(t, c.wantRows, doc.RowCount)
			assert.Equal(t, c.wantLimit, doc.Limit)
			assert.Equal(t, c.wantTruncated, doc.Truncated)
			assert.Equal(t, c.wantWarnings, doc.Warnings)
		})
	}
}

func Test_run_mcp_query_refuses_what_it_cannot_run(t *testing.T) {
	source := filepath.Join(t.TempDir(), "in.csv")
	require.NoError(t, os.WriteFile(source, []byte("n\n1\n"), 0o600))
	cases := []struct {
		name     string
		query    string
		wantLine string
	}{
		{name: "a write statement", query: "CREATE TABLE notes (body VARCHAR)", wantLine: mcpWriteRefusal},
		{name: "a read of another file", query: "SELECT n FROM read_csv('" + source + "')", wantLine: mcpExternalRefusal},
		{name: "whitespace only", query: " \n\t ", wantLine: mcpBlankSQLLine},
		{name: "a bare semicolon", query: ";", wantLine: mcpBlankSQLLine},
		{name: "a comment only", query: "-- note", wantLine: mcpBlankSQLLine},
		{name: "invalid SQL", query: "SELECT missing_column FROM accounts", wantLine: mcpBadColumnRefusal},
		{name: "a column it cannot print", query: `SELECT '{"a": 1}'::JSON AS doc`, wantLine: mcpUnprintableRefusal},
	}
	ctx, peer := newQueryPeer(t)

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			loggedBefore := peer.stderr.Len()

			result := callQuery(ctx, t, peer, map[string]any{"sql": c.query})

			require.True(t, result.IsError)
			require.Len(t, result.Content, 1)
			assert.Equal(t, c.wantLine, textOf(result))
			assert.Nil(t, result.StructuredContent)
			assert.Equal(t, "quarry: mcp: query: "+c.wantLine+"\n", peer.stderr.String()[loggedBefore:])
		})
	}
}

// newQueryPeer syncs the accounts fixture under a fresh HOME and connects a client to quarry mcp.
func newQueryPeer(t *testing.T) (context.Context, *mcpPeer) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	syncAccountsFixture(t, home)
	ctx, cancel := context.WithTimeout(t.Context(), mcpTestDeadline)
	t.Cleanup(cancel)
	return ctx, startMCP(ctx, t, func(*cli.Env) {})
}

// runSQLJSON is the stdout of quarry sql --json over the HOME newQueryPeer set.
func runSQLJSON(ctx context.Context, t *testing.T, query string) string {
	t.Helper()
	var stdout, stderr bytes.Buffer
	require.Equal(t, 0, run(ctx, []string{"sql", "--json", query}, &stdout, &stderr), stderr.String())
	return stdout.String()
}

func callQuery(ctx context.Context, t *testing.T, peer *mcpPeer, arguments map[string]any) *sdk.CallToolResult {
	t.Helper()
	result, err := peer.session.CallTool(ctx, &sdk.CallToolParams{Name: "query", Arguments: arguments})
	require.NoError(t, err)
	return result
}

// textOf is the text of result's content, which must be a single TextContent.
func textOf(result *sdk.CallToolResult) string {
	if len(result.Content) != 1 {
		return fmt.Sprintf("<%d content blocks>", len(result.Content))
	}
	text, ok := result.Content[0].(*sdk.TextContent)
	if !ok {
		return "<not text>"
	}
	return text.Text
}

// compactJSON is the document in indented with its insignificant whitespace removed.
func compactJSON(t *testing.T, indented string) string {
	t.Helper()
	var compact bytes.Buffer
	require.NoError(t, json.Compact(&compact, []byte(indented)))
	return compact.String()
}

// structuredFrame is the structuredContent bytes of the one response frame in stdout that carries any.
func structuredFrame(t *testing.T, stdout string) string {
	t.Helper()
	for frame := range strings.SplitSeq(strings.TrimSuffix(stdout, "\n"), "\n") {
		var message struct {
			Result struct {
				StructuredContent json.RawMessage `json:"structuredContent"`
			} `json:"result"`
		}
		require.NoError(t, json.Unmarshal([]byte(frame), &message), frame)
		if len(message.Result.StructuredContent) > 0 {
			return string(message.Result.StructuredContent)
		}
	}
	require.FailNow(t, "no frame carries structuredContent", stdout)
	return ""
}
