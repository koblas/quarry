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

	"github.com/koblas/quarry/internal/cli"
	"github.com/koblas/quarry/internal/store"
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
	mcpParserRefusal      = `query failed: Parser Error: syntax error at or near "SELEKT"`
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
		arguments     map[string]any
		wantRows      int
		wantLimit     int
		wantTruncated bool
		wantWarnings  []string
	}{
		{
			name: "more rows than an explicit limit", arguments: map[string]any{"sql": "SELECT range AS n FROM range(10)", "limit": 3},
			wantRows: 3, wantLimit: 3, wantTruncated: true,
			wantWarnings: []string{"returned the first 3 rows" + cutTail},
		},
		{
			name: "limit 1 is worded in the singular", arguments: map[string]any{"sql": "SELECT range AS n FROM range(2)", "limit": 1},
			wantRows: 1, wantLimit: 1, wantTruncated: true,
			wantWarnings: []string{"returned the first 1 row" + cutTail},
		},
		{
			name: "more rows than the default limit", arguments: map[string]any{"sql": "SELECT range AS n FROM range(501)"},
			wantRows: 500, wantLimit: 500, wantTruncated: true,
			wantWarnings: []string{"returned the first 500 rows" + cutTail},
		},
		{
			name: "exactly the limit is not truncated", arguments: map[string]any{"sql": "SELECT range AS n FROM range(3)", "limit": 3},
			wantRows: 3, wantLimit: 3, wantWarnings: []string{},
		},
		{
			name: "no rows at all", arguments: map[string]any{"sql": "SELECT 1 AS n WHERE false"},
			wantRows: 0, wantLimit: 500, wantWarnings: []string{},
		},
	}
	ctx, peer := newQueryPeer(t)

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			result := callQuery(ctx, t, peer, c.arguments)

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
		wantLog  string
	}{
		{name: "a write statement", query: "CREATE TABLE notes (body VARCHAR)", wantLine: mcpWriteRefusal, wantLog: mcpWriteRefusal},
		{name: "a read of another file", query: "SELECT n FROM read_csv('" + source + "')", wantLine: mcpExternalRefusal, wantLog: mcpExternalRefusal},
		{name: "whitespace only", query: " \n\t ", wantLine: mcpBlankSQLLine, wantLog: mcpBlankSQLLine},
		{name: "a bare semicolon", query: ";", wantLine: mcpBlankSQLLine, wantLog: mcpBlankSQLLine},
		{name: "a comment only", query: "-- note", wantLine: mcpBlankSQLLine, wantLog: mcpBlankSQLLine},
		{name: "malformed SQL", query: "SELEKT 1", wantLine: mcpParserRefusal, wantLog: "query failed: Parser Error; details went to the client only"},
		{
			name: "a missing column", query: "SELECT missing_column FROM accounts", wantLine: mcpBadColumnRefusal,
			wantLog: "query failed: Binder Error; details went to the client only",
		},
		{
			name: "a column it cannot print", query: `SELECT '{"a": 1}'::JSON AS doc`, wantLine: mcpUnprintableRefusal,
			wantLog: "query failed: a result column has a type quarry cannot print; details went to the client only",
		},
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
			assert.Equal(t, "quarry: mcp: query: "+c.wantLog+"\n", peer.stderr.String()[loggedBefore:])
		})
	}
}

func Test_run_mcp_query_logs_no_stored_value_and_no_sql_when_a_cast_of_a_stored_name_fails(t *testing.T) {
	const query = "SELECT CAST(name AS INTEGER) AS chequing_as_number FROM accounts"
	ctx, peer := newQueryPeer(t)

	result := callQuery(ctx, t, peer, map[string]any{"sql": query})

	require.True(t, result.IsError)
	assert.Contains(t, textOf(result), "Chequing")
	assert.Equal(t, "quarry: mcp: query: query failed: Conversion Error; details went to the client only\n", peer.stderr.String())
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

const (
	describeCategoryCap     = 500
	describeCategoryWarning = "describe_schema lists the first 500 categories of 501; query the categories table for the rest"
)

// schemaDocument is describe_schema's result as a client reads it.
type schemaDocument struct {
	Conventions string `json:"conventions"`
	Relations   []struct {
		Name    string `json:"name"`
		Kind    string `json:"kind"`
		Columns []struct {
			Name string `json:"name"`
			Type string `json:"type"`
		} `json:"columns"`
	} `json:"relations"`
	Accounts []struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Type     string `json:"type"`
		Currency string `json:"currency"`
		Closed   bool   `json:"closed"`
	} `json:"accounts"`
	Categories []struct {
		ID       string `json:"id"`
		FullPath string `json:"full_path"`
		Kind     string `json:"kind"`
		Hidden   bool   `json:"hidden"`
	} `json:"categories"`
	Dates struct {
		First *string `json:"first"`
		Last  *string `json:"last"`
	} `json:"dates"`
	Warnings []string `json:"warnings"`
}

func Test_run_mcp_describe_schema_describes_the_store(t *testing.T) {
	ctx, peer := newDescribePeer(t, describeSchemaRows())
	wantRelations := sqlRelations(ctx, t)
	var help, helpErr bytes.Buffer
	require.Equal(t, 0, run(ctx, []string{"sql", "--help"}, &help, &helpErr), helpErr.String())

	result := callDescribeSchema(ctx, t, peer)
	require.NoError(t, peer.session.Close())
	peer.waitForExit(ctx, t)

	require.False(t, result.IsError, textOf(result))
	var doc schemaDocument
	require.NoError(t, json.Unmarshal([]byte(textOf(result)), &doc))
	assert.Equal(t, wantRelations, relationsOf(doc))
	assert.NotEmpty(t, doc.Conventions)
	assert.Contains(t, help.String(), doc.Conventions)
	assert.Equal(t, []string{"alpha", "Chequing", "chequing", "Zeta"}, accountNames(doc))
	assert.Equal(t, "acct-alpha", doc.Accounts[0].ID)
	assert.Equal(t, "chequing", doc.Accounts[0].Type)
	assert.Equal(t, "USD", doc.Accounts[0].Currency)
	assert.False(t, doc.Accounts[0].Closed)
	assert.Equal(t, "acct-cheq-upper", doc.Accounts[1].ID)
	assert.True(t, doc.Accounts[1].Closed)
	assert.Equal(t, []string{"Auto", "Auto:Fuel", "Food:Groceries", "Food:Groceries:Organic", "Misc:Old"}, categoryPaths(doc))
	assert.Equal(t, "expense", doc.Categories[1].Kind)
	assert.False(t, doc.Categories[1].Hidden)
	assert.Equal(t, "cat-old", doc.Categories[4].ID)
	assert.True(t, doc.Categories[4].Hidden)
	require.NotNil(t, doc.Dates.First)
	require.NotNil(t, doc.Dates.Last)
	assert.Equal(t, "2024-01-02", *doc.Dates.First)
	assert.Equal(t, "2026-09-30", *doc.Dates.Last)
	assert.Equal(t, []string{}, doc.Warnings)
	raw := structuredFrame(t, peer.stdout.String())
	assert.Equal(t, []string{"conventions", "relations", "accounts", "categories", "dates", "warnings"}, objectKeys(t, raw))
	assert.Equal(t, []string{"name", "kind", "columns"}, objectKeys(t, firstOf(t, raw, "relations")))
	assert.Equal(t, []string{"id", "name", "type", "currency", "closed"}, objectKeys(t, firstOf(t, raw, "accounts")))
	assert.Equal(t, []string{"id", "full_path", "kind", "hidden"}, objectKeys(t, firstOf(t, raw, "categories")))
	assert.Empty(t, peer.stderr.String())
}

func Test_run_mcp_describe_schema_lists_the_first_500_categories_and_says_so(t *testing.T) {
	ctx, peer := newDescribePeer(t, manyCategoriesRows(describeCategoryCap+1))

	result := callDescribeSchema(ctx, t, peer)

	require.False(t, result.IsError, textOf(result))
	var doc schemaDocument
	require.NoError(t, json.Unmarshal([]byte(textOf(result)), &doc))
	paths := categoryPaths(doc)
	require.Len(t, paths, describeCategoryCap)
	assert.Equal(t, "Category 000", paths[0])
	assert.Equal(t, "Category 499", paths[describeCategoryCap-1])
	assert.NotContains(t, paths, "Category 500")
	assert.Equal(t, []string{describeCategoryWarning}, doc.Warnings)
	assert.Len(t, doc.Accounts, 1)
}

// describeSchemaRows is a store whose accounts and categories are listed out of order,
// with names differing only in case and a hidden category and a grandchild.
func describeSchemaRows() store.Rows {
	accounts := []store.Account{
		{ID: "acct-zeta", SourceID: 1, Name: "Zeta", Type: "savings", Currency: "CAD", Active: true},
		{ID: "acct-cheq-lower", SourceID: 2, Name: "chequing", Type: "chequing", Currency: "CAD", Active: true},
		{ID: "acct-alpha", SourceID: 3, Name: "alpha", Type: "chequing", Currency: "USD", Active: true},
		{ID: "acct-cheq-upper", SourceID: 4, Name: "Chequing", Type: "chequing", Currency: "CAD", Closed: true, Active: true},
	}
	day := func(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }
	rows := spendRows(accounts,
		spendSplit{id: "mid", account: "acct-zeta", currency: "CAD", day: day(2025, 5, 5), cents: -100},
		spendSplit{id: "first", account: "acct-alpha", currency: "USD", day: day(2024, 1, 2), cents: -200},
		spendSplit{id: "last", account: "acct-cheq-lower", currency: "CAD", day: day(2026, 9, 30), cents: -300},
	)
	rows.Categories = []store.Category{
		{ID: "cat-old", SourceID: 5, Name: "Old", FullPath: "Misc:Old", Kind: "expense", Hidden: true},
		{ID: "cat-organic", SourceID: 4, ParentID: new("cat-groceries"), Name: "Organic", FullPath: "Food:Groceries:Organic", Kind: "expense"},
		{ID: "cat-groceries", SourceID: 2, Name: "Groceries", FullPath: "Food:Groceries", Kind: "expense"},
		{ID: "cat-fuel", SourceID: 1, ParentID: new("cat-auto"), Name: "Fuel", FullPath: "Auto:Fuel", Kind: "expense"},
		{ID: "cat-auto", SourceID: 3, Name: "Auto", FullPath: "Auto", Kind: "expense"},
	}
	rows.ReferencedCategoryIDs = nil
	return rows
}

// manyCategoriesRows is a store of count categories, "Category 000" up, listed in descending order.
func manyCategoriesRows(count int) store.Rows {
	rows := spendRows([]store.Account{chequingAccount("acct-cad", 1)},
		spendSplit{id: "only", account: "acct-cad", currency: "CAD", day: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), cents: -100},
	)
	rows.Categories = nil
	for i := count - 1; i >= 0; i-- {
		rows.Categories = append(rows.Categories, store.Category{
			ID: fmt.Sprintf("cat-%03d", i), SourceID: int64(i + 1), Name: fmt.Sprintf("Category %03d", i),
			FullPath: fmt.Sprintf("Category %03d", i), Kind: "expense",
		})
	}
	rows.ReferencedCategoryIDs = nil
	return rows
}

// newDescribePeer builds a store of rows under a fresh HOME and connects a client to quarry mcp.
func newDescribePeer(t *testing.T, rows store.Rows) (context.Context, *mcpPeer) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	replaceStore(t, home, rows)
	ctx, cancel := context.WithTimeout(t.Context(), mcpTestDeadline)
	t.Cleanup(cancel)
	return ctx, startMCP(ctx, t, func(*cli.Env) {})
}

func callDescribeSchema(ctx context.Context, t *testing.T, peer *mcpPeer) *sdk.CallToolResult {
	t.Helper()
	result, err := peer.session.CallTool(ctx, &sdk.CallToolParams{Name: "describe_schema", Arguments: map[string]any{}})
	require.NoError(t, err)
	return result
}

// relation is one table or view with its columns as "name type", in the order they are declared.
type relation struct {
	Name, Kind string
	Columns    []string
}

// sqlRelations is every relation of the store's main schema, tables first, as quarry sql sees it.
func sqlRelations(ctx context.Context, t *testing.T) []relation {
	t.Helper()
	kinds := map[string]string{"BASE TABLE": "table", "VIEW": "view"}
	listed := sqlJSONDocument(ctx, t, `SELECT table_name, table_type FROM information_schema.tables
		WHERE table_catalog = current_database() AND table_schema = 'main' ORDER BY table_type, table_name`)
	require.NotEmpty(t, listed.Rows)
	relations := make([]relation, 0, len(listed.Rows))
	for _, row := range listed.Rows {
		name, isName := row[0].(string)
		tableType, isType := row[1].(string)
		require.True(t, isName && isType, "%v", row)
		kind := kinds[tableType]
		require.NotEmpty(t, kind, tableType)
		var columns []string
		for _, col := range sqlJSONDocument(ctx, t, "FROM "+name+" LIMIT 0").Columns {
			columns = append(columns, col.Name+" "+col.Type)
		}
		relations = append(relations, relation{Name: name, Kind: kind, Columns: columns})
	}
	return relations
}

type sqlDocument struct {
	Columns []struct {
		Name string `json:"name"`
		Type string `json:"type"`
	} `json:"columns"`
	Rows [][]any `json:"rows"`
}

// sqlJSONDocument is quarry sql --json's document for query over the HOME the test set.
func sqlJSONDocument(ctx context.Context, t *testing.T, query string) sqlDocument {
	t.Helper()
	var doc sqlDocument
	require.NoError(t, json.Unmarshal([]byte(runSQLJSON(ctx, t, query)), &doc))
	return doc
}

func relationsOf(doc schemaDocument) []relation {
	relations := make([]relation, 0, len(doc.Relations))
	for _, r := range doc.Relations {
		var columns []string
		for _, c := range r.Columns {
			columns = append(columns, c.Name+" "+c.Type)
		}
		relations = append(relations, relation{Name: r.Name, Kind: r.Kind, Columns: columns})
	}
	return relations
}

func accountNames(doc schemaDocument) []string {
	names := make([]string, 0, len(doc.Accounts))
	for _, a := range doc.Accounts {
		names = append(names, a.Name)
	}
	return names
}

func categoryPaths(doc schemaDocument) []string {
	paths := make([]string, 0, len(doc.Categories))
	for _, c := range doc.Categories {
		paths = append(paths, c.FullPath)
	}
	return paths
}

// objectKeys is the keys of the JSON object raw, in the order raw writes them.
func objectKeys(t *testing.T, raw string) []string {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader([]byte(raw)))
	opening, err := decoder.Token()
	require.NoError(t, err)
	require.Equal(t, json.Delim('{'), opening, raw)
	var keys []string
	for decoder.More() {
		key, err := decoder.Token()
		require.NoError(t, err)
		name, isName := key.(string)
		require.True(t, isName, "%v", key)
		keys = append(keys, name)
		var skipped json.RawMessage
		require.NoError(t, decoder.Decode(&skipped))
	}
	return keys
}

// firstOf is the first element of the array under key in the JSON object raw.
func firstOf(t *testing.T, raw, key string) string {
	t.Helper()
	var object map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(raw), &object))
	var elements []json.RawMessage
	require.NoError(t, json.Unmarshal(object[key], &elements), key)
	require.NotEmpty(t, elements, key)
	return string(elements[0])
}
