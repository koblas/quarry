package mcp_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/mcp"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
)

const (
	testHome       = "/home/dave"
	testStorePath  = testHome + "/Library/Application Support/quarry/quarry.duckdb"
	blankSQLLine   = "query needs SQL in the sql parameter"
	logPrefixQuery = "quarry: mcp: query: "
	failedLogLine  = "failed; details went to the client only"
	argsRefusedLog = "refused the call's arguments; details went to the client only"
)

var (
	errDiskOnFire   = errors.New("disk on fire")
	errFactoryBroke = errors.New("the report factory broke")
	errNoHome       = errors.New("cannot find your home directory ($HOME is not set); set HOME, then run quarry mcp again")
)

// fakeStore answers Query with result, or err when set, keeping at most maxRows rows as the real store does.
type fakeStore struct {
	report.Store

	result store.QueryResult
	err    error
	asked  []int

	schema      store.Schema
	schemaReads int

	status      store.Status
	statusReads int

	findings      store.FindingList
	findingsReads int

	spent   store.Spending
	flow    store.CashFlow
	charges store.Charges
	through []time.Time
}

// Spending answers with spent, or err when set.
func (f *fakeStore) Spending(context.Context, store.SpendingParams) (store.Spending, error) {
	return f.spent, f.err
}

// CashFlow answers with flow, or err when set.
func (f *fakeStore) CashFlow(context.Context, store.CashFlowParams) (store.CashFlow, error) {
	return f.flow, f.err
}

// Charges answers with charges, or err when set, recording the day it was asked to read through.
func (f *fakeStore) Charges(_ context.Context, params store.ChargeParams) (store.Charges, error) {
	f.through = append(f.through, params.Through)
	return f.charges, f.err
}

// Findings answers with findings, or err when set, counting the reads.
func (f *fakeStore) Findings(context.Context) (store.FindingList, error) {
	f.findingsReads++
	return f.findings, f.err
}

// Status answers with status, or err when set, counting the reads.
func (f *fakeStore) Status(context.Context) (store.Status, error) {
	f.statusReads++
	return f.status, f.err
}

// Schema answers with schema, or err when set, counting the reads.
func (f *fakeStore) Schema(context.Context) (store.Schema, error) {
	f.schemaReads++
	return f.schema, f.err
}

func (f *fakeStore) Query(_ context.Context, _ string, maxRows int) (store.QueryResult, error) {
	f.asked = append(f.asked, maxRows)
	result := f.result
	if maxRows > 0 && len(result.Rows) > maxRows {
		result.Rows = result.Rows[:maxRows]
	}
	return result, f.err
}

// harness is a served query tool over a fake store, with the server's stderr captured.
type harness struct {
	running

	store    *fakeStore
	stderr   *bytes.Buffer
	built    int
	commands []string
}

// newHarness serves the tools over st with opts; buildErr, when set, is what the report factory fails with.
func newHarness(t *testing.T, st *fakeStore, buildErr error, opts ...mcp.Option) *harness {
	t.Helper()
	h := &harness{store: st, stderr: &bytes.Buffer{}}
	factory := func(_ context.Context, command string) (*report.Server, error) {
		h.built++
		h.commands = append(h.commands, command)
		if buildErr != nil {
			return nil, buildErr
		}
		return report.NewServer(report.WithStore(st), report.WithHome(testHome)), nil
	}
	h.running = startServerLogging(t, newServer(append([]mcp.Option{mcp.WithReport(factory)}, opts...)...), h.stderr)
	return h
}

// query calls the query tool with arguments.
func (h *harness) query(t *testing.T, arguments any) *sdk.CallToolResult {
	t.Helper()
	result, err := h.session.CallTool(t.Context(), &sdk.CallToolParams{Name: "query", Arguments: arguments})
	require.NoError(t, err)
	return result
}

// describeSchema calls the describe_schema tool with no arguments.
func (h *harness) describeSchema(t *testing.T) *sdk.CallToolResult {
	t.Helper()
	result, err := h.session.CallTool(t.Context(), &sdk.CallToolParams{Name: "describe_schema", Arguments: map[string]any{}})
	require.NoError(t, err)
	return result
}

// syncStatus calls the sync_status tool with no arguments.
func (h *harness) syncStatus(t *testing.T) *sdk.CallToolResult {
	t.Helper()
	result, err := h.session.CallTool(t.Context(), &sdk.CallToolParams{Name: "sync_status", Arguments: map[string]any{}})
	require.NoError(t, err)
	return result
}

// dataQuality calls the data_quality tool with arguments.
func (h *harness) dataQuality(t *testing.T, arguments any) *sdk.CallToolResult {
	t.Helper()
	result, err := h.session.CallTool(t.Context(), &sdk.CallToolParams{Name: "data_quality", Arguments: arguments})
	require.NoError(t, err)
	return result
}

// spending calls the spending tool with arguments.
func (h *harness) spending(t *testing.T, arguments any) *sdk.CallToolResult {
	t.Helper()
	result, err := h.session.CallTool(t.Context(), &sdk.CallToolParams{Name: "spending", Arguments: arguments})
	require.NoError(t, err)
	return result
}

// cashFlow calls the cash_flow tool with arguments.
func (h *harness) cashFlow(t *testing.T, arguments any) *sdk.CallToolResult {
	t.Helper()
	result, err := h.session.CallTool(t.Context(), &sdk.CallToolParams{Name: "cash_flow", Arguments: arguments})
	require.NoError(t, err)
	return result
}

// recurringCharges calls the recurring_charges tool with arguments.
func (h *harness) recurringCharges(t *testing.T, arguments any) *sdk.CallToolResult {
	t.Helper()
	result, err := h.session.CallTool(t.Context(), &sdk.CallToolParams{Name: "recurring_charges", Arguments: arguments})
	require.NoError(t, err)
	return result
}

// anomalies calls the anomalies tool with arguments.
func (h *harness) anomalies(t *testing.T, arguments any) *sdk.CallToolResult {
	t.Helper()
	result, err := h.session.CallTool(t.Context(), &sdk.CallToolParams{Name: "anomalies", Arguments: arguments})
	require.NoError(t, err)
	return result
}

// rowsOf is a one-column result of n rows holding 0..n-1.
func rowsOf(n int) store.QueryResult {
	rows := make([][]store.QueryValue, n)
	for i := range rows {
		rows[i] = []store.QueryValue{{Text: strconv.Itoa(i), Native: int64(i)}}
	}
	return store.QueryResult{Columns: []store.QueryColumn{{Name: "n", Type: "BIGINT"}}, Rows: rows}
}

// textOf is the text of result's one content block.
func textOf(t *testing.T, result *sdk.CallToolResult) string {
	t.Helper()
	require.Len(t, result.Content, 1)
	text, ok := result.Content[0].(*sdk.TextContent)
	require.True(t, ok, "content is not text")
	return text.Text
}

// jsonOf is v, a decoded JSON value, encoded again.
func jsonOf(t *testing.T, v any) string {
	t.Helper()
	encoded, err := json.Marshal(v)
	require.NoError(t, err)
	return string(encoded)
}

// payeeNamed is the i-th of a run of payee names that are letters only, so each keeps its own payee key.
func payeeNamed(i int) string {
	const letters = "abcdefghijklmnopqrstuvwxyz"
	return string([]byte{letters[i/676%26], letters[i/26%26], letters[i%26]})
}

// inUSD relists payee's charges in USD with no converted amount, so a CAD report cannot convert them.
func inUSD(rows []store.Charge, payee string) {
	for i := range rows {
		if *rows[i].Payee == payee {
			rows[i].Currency = "USD"
			rows[i].Account.Currency = "USD"
		}
	}
}
