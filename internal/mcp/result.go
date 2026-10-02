package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// toolFunc is a tool's logic: the document to send, or the error to refuse with.
type toolFunc[In any] func(ctx context.Context, in In) (document any, err error)

// handler adapts run to the SDK: a document becomes one compact JSON text block
// plus the same bytes as structuredContent; an error becomes an isError result.
func handler[In any](run toolFunc[In]) sdk.ToolHandlerFor[In, any] {
	return func(ctx context.Context, _ *sdk.CallToolRequest, in In) (*sdk.CallToolResult, any, error) {
		document, err := run(ctx, in)
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
