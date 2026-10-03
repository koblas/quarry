// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/cli"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const mcpTestDeadline = 30 * time.Second

// What `go test` binaries report as their main module version.
const mcpTestServerVersion = "(devel)"

const mcpInstructions = `quarry serves David's Quicken Classic for Mac data from a local, read-only
store. Call sync_status first and tell the user how old the snapshot is
(snapshot.taken_at). Call describe_schema before writing SQL: its
conventions say which views already leave out transfers and how amounts,
signs and currencies work. Every number you report must come from a tool
result; never estimate. quarry cannot change data: fixes are made in
Quicken, then the user runs quarry sync.`

const mcpQueryDescription = `Run one read-only SQL query (DuckDB dialect) against quarry's store and
return its columns and rows. Call describe_schema first for the tables,
views and conventions. For spending and income use v_spending and
v_cash_flow: they already leave out transfers between the user's own
accounts. Returns at most ` + "`limit`" + ` rows (default 500, the most allowed);
aggregate in SQL rather than paging through rows. The store cannot be
changed, and other files, databases and extensions are off.
Send one statement; if you send several, only the last one's rows come back.`

const mcpSyncStatusDescription = `Report how fresh quarry's data is: the snapshot the store was built from
and when it was taken, the dates its transactions cover, the checks sync
ran (balances reconciled to Quicken, splits, transfers), open findings,
and Bank of Canada rate coverage. quarry cannot refresh the data; if it
is old, ask the user to run quarry sync.`

const mcpDescribeSchemaDescription = `Describe quarry's store: every table and view with its columns and types,
the conventions for amounts, signs, transfers and currencies, the
accounts, the category tree, and the first and last transaction dates.
Call this before writing SQL for query.`

const mcpDataQualityDescription = `List the data-quality findings quarry's last sync found: problems to fix
in Quicken (duplicates, one-sided or unlinked transfers, uncategorized
splits, payees in mixed categories, payee name variants, similar or
unused categories). Each finding has an id, the suggested fix, and the
transactions, payees or categories it is about. quarry never fixes them:
the user fixes them in Quicken and runs quarry sync, and fixed findings
drop off. To ignore a finding the user adds its id to findings.ignore in
quarry's config file.`

const mcpSpendingDescription = `Total the user's spending for a period, grouped by category, payee, tag
or month, with a total per currency. quarry's spending rules apply:
transfers between the user's own accounts, Quicken's system categories,
transactions marked "exclude from reports" and accounts Quicken leaves out
of reports are not counted, and refunds are netted, so a category can come
out negative. Each split is converted at the Bank of Canada rate for its
date. Use this rather than query for spending totals. Returns at most 500
rows; totals always count every row.`

const (
	mcpSpendingInputSchema = `{
		"type": "object",
		"properties": {
			"since": {"type": "string", "description": "First day to count: YYYY, YYYY-MM or YYYY-MM-DD; a year or month starts on its first day. ` +
		`Defaults to January 1 of this year."},
			"until": {"type": "string", "description": "Last day to count: YYYY, YYYY-MM or YYYY-MM-DD; a year or month ends on its last day. ` +
		`Defaults to today; future-dated transactions count only when until is later than today."},
			"accounts": {"type": "array", "items": {"type": "string"}, "description": "Count only these accounts, each given by id or by name in any letter case. ` +
		`Omit it to count every account."},
			"currency": {"type": "string", "enum": ["CAD", "USD", "native"], "description": "Currency for amounts: CAD, USD, or native to list each account's own currency separately. ` +
		`Defaults to reporting.currency in quarry's config file, else CAD."},
			"by": {"type": "string", "enum": ["category", "payee", "tag", "month"], "default": "category", "description": "Group by category (the default), payee, tag or month. ` +
		`A split with several tags counts under each tag."}
		},
		"additionalProperties": false
	}`
	mcpQueryInputSchema = `{
		"type": "object",
		"properties": {
			"sql":   {"type": "string", "minLength": 1},
			"limit": {"type": "integer", "minimum": 1, "maximum": 500, "default": 500}
		},
		"required": ["sql"],
		"additionalProperties": false
	}`
	mcpNoInputSchema          = `{"type": "object", "additionalProperties": false}`
	mcpDataQualityInputSchema = `{
		"type": "object",
		"properties": {
			"status": {"type": "string", "enum": ["open", "ignored", "fixed", "all"], "default": "open"},
			"type":   {"type": "string", "enum": [
				"duplicate", "one-sided-transfer", "unlinked-transfer", "uncategorized",
				"mixed-categories", "payee-variants", "similar-categories", "unused-category"]},
			"limit":  {"type": "integer", "minimum": 1, "maximum": 500, "default": 50}
		},
		"additionalProperties": false
	}`
	mcpObjectOutputSchema = `{"type": "object"}`
)

func Test_run_mcp_lists_quarrys_four_tools_over_json_rpc(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), mcpTestDeadline)
	defer cancel()
	peer := startMCP(ctx, t, func(*cli.Env) {})
	session := peer.session

	listed, err := session.ListTools(ctx, nil)
	require.NoError(t, err)
	require.NoError(t, session.Close())
	peer.waitForExit(ctx, t)
	stdout, stderr := peer.stdout, peer.stderr

	initialized := session.InitializeResult()
	assert.Equal(t, &sdk.Implementation{Name: "quarry", Version: mcpTestServerVersion}, initialized.ServerInfo)
	assert.Equal(t, mcpInstructions, initialized.Instructions)
	wantTools := map[string]struct{ description, inputSchema string }{
		"query":           {mcpQueryDescription, mcpQueryInputSchema},
		"describe_schema": {mcpDescribeSchemaDescription, mcpNoInputSchema},
		"sync_status":     {mcpSyncStatusDescription, mcpNoInputSchema},
		"data_quality":    {mcpDataQualityDescription, mcpDataQualityInputSchema},
		"spending":        {mcpSpendingDescription, mcpSpendingInputSchema},
	}
	require.Len(t, listed.Tools, len(wantTools))
	for _, tool := range listed.Tools {
		want, known := wantTools[tool.Name]
		require.True(t, known, "unexpected tool %q", tool.Name)
		assert.Equal(t, want.description, tool.Description, tool.Name)
		assertJSONEqualAny(t, want.inputSchema, tool.InputSchema, tool.Name+" input schema")
		assertJSONEqualAny(t, mcpObjectOutputSchema, tool.OutputSchema, tool.Name+" output schema")
	}
	frames := strings.Split(strings.TrimSuffix(stdout.String(), "\n"), "\n")
	assert.GreaterOrEqual(t, len(frames), 2)
	for _, frame := range frames {
		var message struct {
			JSONRPC string `json:"jsonrpc"`
		}
		require.NoError(t, json.Unmarshal([]byte(frame), &message), frame)
		assert.Equal(t, "2.0", message.JSONRPC, frame)
	}
	assert.Empty(t, stderr.String())
}

// assertJSONEqualAny compares want to got, a decoded JSON value, as JSON.
func assertJSONEqualAny(t *testing.T, want string, got any, msg string) {
	t.Helper()
	encoded, err := json.Marshal(got)
	require.NoError(t, err, msg)
	assert.JSONEq(t, want, string(encoded), msg)
}

// mcpPeer is a quarry mcp running in-process with an MCP client connected to it over pipes.
type mcpPeer struct {
	session        *sdk.ClientSession
	exit           <-chan int
	stdout, stderr *bytes.Buffer
}

// startMCP runs quarry mcp over pipes, lets tweak adjust its Env, and connects a client.
func startMCP(ctx context.Context, t *testing.T, tweak func(*cli.Env)) *mcpPeer {
	t.Helper()
	serverStdin, toServer := io.Pipe()
	serverStdout, fromServer := io.Pipe()
	peer := &mcpPeer{stdout: &bytes.Buffer{}, stderr: &bytes.Buffer{}}
	exit := make(chan int, 1)
	peer.exit = exit
	go func() {
		env := testEnv(io.MultiWriter(fromServer, peer.stdout), peer.stderr)
		env.Stdin = serverStdin
		tweak(&env)
		exit <- runWith(ctx, []string{"mcp"}, env)
		_ = fromServer.Close()
	}()

	client := sdk.NewClient(&sdk.Implementation{Name: "test-client", Version: "v0.0.0"}, nil)
	session, err := client.Connect(ctx, &sdk.IOTransport{Reader: serverStdout, Writer: toServer}, nil)
	require.NoError(t, err)
	peer.session = session
	return peer
}

// waitForExit returns quarry mcp's exit code, failing the test if it has not exited by ctx's deadline.
func (p *mcpPeer) waitForExit(ctx context.Context, t *testing.T) int {
	t.Helper()
	select {
	case code := <-p.exit:
		return code
	case <-ctx.Done():
		require.FailNow(t, "quarry mcp did not return after the client closed its session")
		return 0
	}
}
