package mcp

import (
	"bytes"
	"context"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// nullArguments is the JSON a client sends for tools/call arguments it leaves null.
var nullArguments = []byte("null")

// absentNullArguments treats a tools/call whose arguments are JSON null as one
// with no arguments. Left alone, a null reaches the SDK as a nil map and
// applying a schema default to it panics the server.
func absentNullArguments(next sdk.MethodHandler) sdk.MethodHandler {
	return func(ctx context.Context, method string, req sdk.Request) (sdk.Result, error) {
		if call, ok := req.(*sdk.CallToolRequest); ok && bytes.Equal(bytes.TrimSpace(call.Params.Arguments), nullArguments) {
			call.Params.Arguments = nil
		}
		return next(ctx, method, req)
	}
}
