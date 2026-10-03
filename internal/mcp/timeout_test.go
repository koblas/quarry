package mcp_test

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/mcp"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errDriverInterrupt = errors.New("interrupt error: interrupted")

// stallingStore is a report.Store whose reads record the deadline they were given and, when stall
// is set, block until their context ends and fail as the real adapter does.
type stallingStore struct {
	report.Store

	stall     bool
	deadlines chan time.Time
}

func newStallingStore(stall bool) *stallingStore {
	const room = 4
	return &stallingStore{stall: stall, deadlines: make(chan time.Time, room)}
}

// wait records ctx's deadline, then blocks until ctx ends when the store stalls.
func (s *stallingStore) wait(ctx context.Context) bool {
	deadline, _ := ctx.Deadline()
	s.deadlines <- deadline
	if s.stall {
		<-ctx.Done()
	}
	return s.stall
}

func (s *stallingStore) Query(ctx context.Context, _ string, _ int) (store.QueryResult, error) {
	if s.wait(ctx) {
		return store.QueryResult{}, store.InterruptedBy(ctx, errDriverInterrupt)
	}
	return rowsOf(1), nil
}

func (s *stallingStore) Status(ctx context.Context) (store.Status, error) {
	s.wait(ctx)
	return store.Status{}, &store.OpenError{Fault: store.OpenFaultOther, Path: testStorePath, Err: errDriverInterrupt}
}

func (s *stallingStore) Schema(ctx context.Context) (store.Schema, error) {
	s.wait(ctx)
	return store.Schema{}, &store.OpenError{Fault: store.OpenFaultOther, Path: testStorePath, Err: errDriverInterrupt}
}

func (s *stallingStore) Findings(ctx context.Context) (store.FindingList, error) {
	s.wait(ctx)
	return store.FindingList{}, &store.OpenError{Fault: store.OpenFaultOther, Path: testStorePath, Err: errDriverInterrupt}
}

func (s *stallingStore) Spending(ctx context.Context, _ store.SpendingParams) (store.Spending, error) {
	s.wait(ctx)
	return store.Spending{}, &store.OpenError{Fault: store.OpenFaultOther, Path: testStorePath, Err: errDriverInterrupt}
}

func (s *stallingStore) CashFlow(ctx context.Context, _ store.CashFlowParams) (store.CashFlow, error) {
	s.wait(ctx)
	return store.CashFlow{}, &store.OpenError{Fault: store.OpenFaultOther, Path: testStorePath, Err: errDriverInterrupt}
}

// serveStalling serves the tools over st with opts, returning the client session and the server's stderr.
func serveStalling(t *testing.T, st *stallingStore, opts ...mcp.Option) (*sdk.ClientSession, *bytes.Buffer) {
	t.Helper()
	stderr := &bytes.Buffer{}
	factory := func(context.Context, string) (*report.Server, error) {
		return report.NewServer(report.WithStore(st), report.WithHome(testHome)), nil
	}
	opts = append([]mcp.Option{mcp.WithReport(factory), mcp.WithConfig((&configStub{}).load)}, opts...)
	return startServerLogging(t, mcp.NewServer(opts...), stderr).session, stderr
}

func callTool(t *testing.T, session *sdk.ClientSession, name string, arguments any) *sdk.CallToolResult {
	t.Helper()
	result, err := session.CallTool(t.Context(), &sdk.CallToolParams{Name: name, Arguments: arguments})
	require.NoError(t, err)
	return result
}

func Test_each_tool_answers_its_deadline_with_its_ruled_line(t *testing.T) {
	t.Parallel()
	cases := []struct {
		tool      string
		arguments any
		timeout   time.Duration
		want      string
	}{
		{"query", map[string]any{"sql": "SELECT 1"}, time.Second, "query stopped after 1 second; aggregate or filter it in SQL, then try again"},
		{"describe_schema", map[string]any{}, 2 * time.Second, "describe_schema stopped after 2 seconds; try again"},
		{"sync_status", map[string]any{}, time.Second, "sync_status stopped after 1 second; try again"},
		{"data_quality", map[string]any{}, time.Second, "data_quality stopped after 1 second; try again"},
		{"spending", map[string]any{}, time.Second, "spending stopped after 1 second; try again"},
		{"cash_flow", map[string]any{}, time.Second, "cash_flow stopped after 1 second; try again"},
	}

	for _, c := range cases {
		t.Run(c.tool, func(t *testing.T) {
			t.Parallel()
			session, stderr := serveStalling(t, newStallingStore(true), mcp.WithTimeout(c.timeout))

			result := callTool(t, session, c.tool, c.arguments)

			assert.True(t, result.IsError)
			assert.Equal(t, c.want, textOf(t, result))
			assert.Equal(t, "quarry: mcp: "+c.tool+": "+c.want+"\n", stderr.String())
		})
	}
}

func Test_a_call_finishing_inside_its_deadline_is_answered(t *testing.T) {
	session, stderr := serveStalling(t, newStallingStore(false), mcp.WithTimeout(time.Second))

	result := callTool(t, session, "query", map[string]any{"sql": "SELECT 1"})

	assert.False(t, result.IsError, textOf(t, result))
	assert.Empty(t, stderr.String())
}

func Test_a_call_without_WithTimeout_gets_a_30_second_deadline(t *testing.T) {
	st := newStallingStore(false)
	session, _ := serveStalling(t, st)
	const want = 30 * time.Second

	before := time.Now()
	callTool(t, session, "query", map[string]any{"sql": "SELECT 1"})
	after := time.Now()

	deadline := <-st.deadlines
	assert.False(t, deadline.Before(before.Add(want)), "deadline %v is before call start + 30 s", deadline)
	assert.False(t, deadline.After(after.Add(want)), "deadline %v is after call end + 30 s", deadline)
}
