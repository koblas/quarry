// White-box: the request context and the result shapes errorLog guards cannot be driven through a client.
package mcp

import (
	"bytes"
	"context"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// logged runs a tools/call for tool "query" through errorLog(w) with next answering res and recording line when it is not "".
func logged(ctx context.Context, res sdk.Result, line string) string {
	var w bytes.Buffer
	call := &sdk.CallToolRequest{Params: &sdk.CallToolParamsRaw{Name: "query"}}
	next := func(ctx context.Context, _ string, _ sdk.Request) (sdk.Result, error) {
		if line != "" {
			recordLog(ctx, line)
		}
		return res, nil
	}

	_, _ = errorLog(&w)(next)(ctx, "tools/call", call)

	return w.String()
}

func refusalResult(text string) *sdk.CallToolResult {
	return &sdk.CallToolResult{IsError: true, Content: []sdk.Content{&sdk.TextContent{Text: text}}}
}

func Test_errorLog_writes_the_line_the_handler_recorded_not_the_text_the_client_gets(t *testing.T) {
	const modelText = "query failed: Conversion Error: Could not convert string 'Chequing' to INT32"

	got := logged(t.Context(), refusalResult(modelText), "query failed: Conversion Error; details went to the client only")

	assert.Equal(t, "quarry: mcp: query: query failed: Conversion Error; details went to the client only\n", got)
}

func Test_errorLog_logs_an_argument_refusal_for_a_refusal_with_no_recorded_line(t *testing.T) {
	got := logged(t.Context(), refusalResult("validating arguments: sql is 'SELECT 1; DROP'"), "")

	assert.Equal(t, "quarry: mcp: query: refused the call's arguments; details went to the client only\n", got)
}

func Test_errorLog_writes_nothing_for_a_call_whose_context_is_done(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	assert.Empty(t, logged(ctx, refusalResult("context canceled"), "recorded"))
}

func Test_errorLog_writes_nothing_for_a_call_the_server_answered_with_a_protocol_error(t *testing.T) {
	var nilResult *sdk.CallToolResult

	assert.Empty(t, logged(t.Context(), nilResult, ""))
}

func Test_errorLog_writes_nothing_for_a_call_that_succeeded(t *testing.T) {
	assert.Empty(t, logged(t.Context(), &sdk.CallToolResult{}, ""))
}

// fakeQueryStore records the maxRows Query is asked for.
type fakeQueryStore struct {
	report.Store

	asked []int
}

func (f *fakeQueryStore) Query(_ context.Context, _ string, maxRows int) (store.QueryResult, error) {
	f.asked = append(f.asked, maxRows)
	return store.QueryResult{}, nil
}

func Test_query_caps_a_limit_the_schema_did_not_check(t *testing.T) {
	cases := map[string]int{"zero": 0, "negative": -3, "above the cap": maxRows + 1}

	for name, limit := range cases {
		t.Run(name, func(t *testing.T) {
			st := &fakeQueryStore{}
			srv := NewServer(WithReport(func(context.Context, string) (*report.Server, error) {
				return report.NewServer(report.WithStore(st)), nil
			}))

			_, err := srv.query(t.Context(), queryInput{SQL: "SELECT 1", Limit: limit})

			require.NoError(t, err)
			assert.Equal(t, []int{maxRows + 1}, st.asked)
		})
	}
}

// overlapWriter counts writes that begin while another is still in progress.
type overlapWriter struct {
	active   atomic.Int32
	overlaps atomic.Int32
}

func (w *overlapWriter) Write(p []byte) (int, error) {
	if w.active.Add(1) > 1 {
		w.overlaps.Add(1)
	}
	for range 10 {
		runtime.Gosched()
	}
	w.active.Add(-1)
	return len(p), nil
}

func Test_errorLog_never_writes_two_lines_at_once(t *testing.T) {
	const calls = 64
	var w overlapWriter
	log := errorLog(&w)(func(ctx context.Context, _ string, _ sdk.Request) (sdk.Result, error) {
		recordLog(ctx, "boom")
		return refusalResult("boom"), nil
	})
	call := &sdk.CallToolRequest{Params: &sdk.CallToolParamsRaw{Name: "query"}}
	var wg sync.WaitGroup

	for range calls {
		wg.Go(func() { _, _ = log(t.Context(), "tools/call", call) })
	}
	wg.Wait()

	assert.Zero(t, w.overlaps.Load())
}
