// White-box: the request context and the result shapes errorLog guards cannot be driven through a client.
package mcp

import (
	"bytes"
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// logged runs a tools/call for tool "query" through errorLog(w) with next answering res, and returns w's text.
func logged(ctx context.Context, res sdk.Result, err error) string {
	var w bytes.Buffer
	call := &sdk.CallToolRequest{Params: &sdk.CallToolParamsRaw{Name: "query"}}
	next := func(context.Context, string, sdk.Request) (sdk.Result, error) { return res, err }

	_, _ = errorLog(&w)(next)(ctx, "tools/call", call)

	return w.String()
}

func refusalResult(text string) *sdk.CallToolResult {
	return &sdk.CallToolResult{IsError: true, Content: []sdk.Content{&sdk.TextContent{Text: text}}}
}

func Test_errorLog_writes_the_line_of_a_refusal_whose_context_is_live(t *testing.T) {
	assert.Equal(t, "quarry: mcp: query: boom\n", logged(t.Context(), refusalResult("boom"), nil))
}

func Test_errorLog_writes_nothing_for_a_call_whose_context_is_done(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	assert.Empty(t, logged(ctx, refusalResult("context canceled"), nil))
}

func Test_errorLog_writes_nothing_for_a_call_the_server_answered_with_a_protocol_error(t *testing.T) {
	var nilResult *sdk.CallToolResult

	assert.Empty(t, logged(t.Context(), nilResult, context.Canceled))
}

func Test_errorLog_writes_an_empty_text_for_a_refusal_with_no_text_block(t *testing.T) {
	assert.Equal(t, "quarry: mcp: query: \n", logged(t.Context(), &sdk.CallToolResult{IsError: true}, nil))
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

func Test_errorLog_keeps_concurrent_refusals_lines_whole(t *testing.T) {
	const calls = 64
	var w bytes.Buffer
	log := errorLog(&w)(func(context.Context, string, sdk.Request) (sdk.Result, error) {
		return refusalResult("boom"), nil
	})
	call := &sdk.CallToolRequest{Params: &sdk.CallToolParamsRaw{Name: "query"}}
	var wg sync.WaitGroup

	for range calls {
		wg.Go(func() { _, _ = log(t.Context(), "tools/call", call) })
	}
	wg.Wait()

	assert.Equal(t, strings.Repeat("quarry: mcp: query: boom\n", calls), w.String())
}
