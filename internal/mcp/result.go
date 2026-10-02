package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/koblas/quarry/internal/platform/humanize"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// toolFunc is a tool's logic: the document to send, or the error to refuse with.
type toolFunc[In any] func(ctx context.Context, in In) (document any, err error)

// stoppedFunc words a tool's deadline for the model; its argument is the timeout, e.g. "30 seconds".
type stoppedFunc func(limit string) string

// stoppedError is the isError text of a call that ran past its deadline.
type stoppedError string

func (e stoppedError) Error() string { return string(e) }

// handler adapts run to the SDK: a document becomes one compact JSON text block plus the same
// bytes as structuredContent; an error becomes an isError result. run gets ctx bounded by timeout,
// and an error that carries context.DeadlineExceeded is answered with stopped's line instead.
func handler[In any](timeout time.Duration, stopped stoppedFunc, run toolFunc[In]) sdk.ToolHandlerFor[In, any] {
	return func(ctx context.Context, _ *sdk.CallToolRequest, in In) (*sdk.CallToolResult, any, error) {
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		document, err := run(ctx, in)
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, nil, stoppedError(stopped(humanize.Count(int(timeout/time.Second), "second", "seconds")))
		}
		if err != nil {
			return nil, nil, err
		}
		body, err := json.Marshal(document)
		if err != nil {
			// unreachable: documents hold only strings, bools, ints, finite floats and nil (document.sqlCell is the only producer of cell values)
			return nil, nil, fmt.Errorf("encode result: %w", err)
		}
		return &sdk.CallToolResult{
			Content:           []sdk.Content{&sdk.TextContent{Text: string(body)}},
			StructuredContent: json.RawMessage(body),
		}, nil, nil
	}
}

// errorLog is the tools/call middleware that writes "quarry: mcp: <tool>: <text>"
// to w for each isError call, none once the call's context is done.
func errorLog(w io.Writer) sdk.Middleware {
	var mu sync.Mutex
	return func(next sdk.MethodHandler) sdk.MethodHandler {
		return func(ctx context.Context, method string, req sdk.Request) (sdk.Result, error) {
			res, err := next(ctx, method, req)
			call, isCall := req.(*sdk.CallToolRequest)
			result, isResult := res.(*sdk.CallToolResult)
			if !isCall || !isResult || result == nil || !result.IsError || ctx.Err() != nil {
				return res, err
			}
			// One Write under the lock keeps concurrent calls' lines whole.
			mu.Lock()
			defer mu.Unlock()
			_, _ = fmt.Fprintf(w, "quarry: mcp: %s: %s\n", call.Params.Name, errorText(result))
			return res, err
		}
	}
}

// errorText is the text of result's first text block, "" when it has none.
func errorText(result *sdk.CallToolResult) string {
	for _, content := range result.Content {
		if text, ok := content.(*sdk.TextContent); ok {
			return text.Text
		}
	}
	return ""
}

// stoppedLine is the timeout line of tool, which has nothing to suggest but trying again.
func stoppedLine(tool string) stoppedFunc {
	return func(limit string) string { return tool + " stopped after " + limit + "; try again" }
}

// queryStoppedLine is the query tool's timeout line, which names the way to a faster query.
func queryStoppedLine(limit string) string {
	return toolQuery + " stopped after " + limit + "; aggregate or filter it in SQL, then try again"
}
