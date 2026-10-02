package mcp

import (
	"bytes"
	"context"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// nullArguments is the JSON a client sends for tools/call arguments it leaves null.
var nullArguments = []byte("null")

// absentNullArguments treats tools/call arguments of JSON null as absent; a
// null reaching the SDK's schema-default step panics the server.
func absentNullArguments(next sdk.MethodHandler) sdk.MethodHandler {
	return func(ctx context.Context, method string, req sdk.Request) (sdk.Result, error) {
		if call, ok := req.(*sdk.CallToolRequest); ok && bytes.Equal(call.Params.Arguments, nullArguments) {
			call.Params.Arguments = nil
		}
		return next(ctx, method, req)
	}
}
