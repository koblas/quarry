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
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// toolFunc is a tool's logic: the document to send, or the error to refuse with.
type toolFunc[In any] func(ctx context.Context, in In) (document any, err error)

// stoppedFunc words a tool's deadline for the model; its argument is the timeout, e.g. "30 seconds".
type stoppedFunc func(limit string) string

// The stderr lines of outcomes that carry text from the client, the store or the config, which stay off stderr.
const (
	argumentsRefusedLog = "refused the call's arguments; details went to the client only"
	failedLog           = "failed; details went to the client only"
	windowRefusedLog    = "refused the call's since or until; details went to the client only"
	unknownAccountLog   = "refused the call's accounts: one names no account; details went to the client only"
	ambiguousAccountLog = "refused the call's accounts: one names more than one account; details went to the client only"
	unknownCategoryLog  = "refused the call's category: it names no category; details went to the client only"
)

// withheldStoreLog is the stderr line of a store that failed for a reason only the client should read; at is its ~ path.
func withheldStoreLog(at string) string {
	return "cannot read the store at " + at + "; details went to the client only"
}

// stoppedError is the isError text of a call that ran past its deadline.
type stoppedError string

func (e stoppedError) Error() string { return string(e) }

// loggedError is an error whose stderr line differs from its text, which only the client sees.
type loggedError struct {
	error

	line string
}

func (e loggedError) Unwrap() error { return e.error }

// withLog is err, logged to stderr as line.
func withLog(err error, line string) error { return loggedError{error: err, line: line} }

// verbatim is err, logged to stderr as its own text; only for fixed copy that carries nothing from the call.
func verbatim(err error) error { return withLog(err, err.Error()) }

// logLine is the stderr line of err, a tool's failure: its recorded line, a report refusal's, else the generic one.
func logLine(err error) string {
	if logged, ok := errors.AsType[loggedError](err); ok {
		return logged.line
	}
	if refusal, ok := errors.AsType[report.RefusalError](err); ok {
		return refusalLine(refusal)
	}
	return failedLog
}

// refusalLine is the stderr line of refusal: a class line where its text carries the caller's account or a
// reason from the store's own engine, else its text, which is fixed copy.
func refusalLine(refusal report.RefusalError) string {
	switch refusal.Kind {
	case report.RefusalUnknownAccount:
		return unknownAccountLog
	case report.RefusalAmbiguousAccount:
		return ambiguousAccountLog
	case report.RefusalUnknownCategory:
		return unknownCategoryLog
	case report.RefusalStore:
		if refusal.Fault == store.OpenFaultOther {
			return withheldStoreLog(refusal.At)
		}
	case report.RefusalGeneric:
	}
	return refusal.Error()
}

// logSlot is where one tool call's handler leaves its stderr line for errorLog.
type logSlot struct {
	line string
	set  bool
}

type logSlotKey struct{}

// recordLog sets the stderr line of the call ctx belongs to; a ctx errorLog did not prepare drops it.
func recordLog(ctx context.Context, line string) {
	if slot, ok := ctx.Value(logSlotKey{}).(*logSlot); ok {
		slot.line, slot.set = line, true
	}
}

// handler adapts run to the SDK: a document becomes one JSON text block plus structuredContent, an error an isError result.
// run's ctx ends at timeout, and an error carrying context.DeadlineExceeded is answered with stopped's line.
func handler[In any](timeout time.Duration, stopped stoppedFunc, run toolFunc[In]) sdk.ToolHandlerFor[In, any] {
	return func(ctx context.Context, _ *sdk.CallToolRequest, in In) (*sdk.CallToolResult, any, error) {
		runCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		document, err := run(runCtx, in)
		if errors.Is(err, context.DeadlineExceeded) {
			err = verbatim(stoppedError(stopped(humanize.Count(int(timeout/time.Second), "second", "seconds"))))
		}
		if err != nil {
			return refuse(ctx, err)
		}
		body, err := json.Marshal(document)
		if err != nil {
			// unreachable: documents hold only strings, bools, ints, finite floats and nil (document.NewSQL's cells are the only values of other kinds)
			return refuse(ctx, fmt.Errorf("encode result: %w", err))
		}
		return &sdk.CallToolResult{
			Content:           []sdk.Content{&sdk.TextContent{Text: string(body)}},
			StructuredContent: json.RawMessage(body),
		}, nil, nil
	}
}

// refuse records err's stderr line and returns it as the handler's failure.
func refuse(ctx context.Context, err error) (*sdk.CallToolResult, any, error) {
	recordLog(ctx, logLine(err))
	return nil, nil, err
}

// errorLog is the tools/call middleware that writes "quarry: mcp: <tool>: <line>" to w for each isError call,
// none once the call's context is done. The line is the one the handler recorded; a refusal with none was the SDK's.
func errorLog(w io.Writer) sdk.Middleware {
	var mu sync.Mutex
	return func(next sdk.MethodHandler) sdk.MethodHandler {
		return func(ctx context.Context, method string, req sdk.Request) (sdk.Result, error) {
			slot := &logSlot{}
			res, err := next(context.WithValue(ctx, logSlotKey{}, slot), method, req)
			call, isCall := req.(*sdk.CallToolRequest)
			result, isResult := res.(*sdk.CallToolResult)
			if !isCall || !isResult || result == nil || !result.IsError || ctx.Err() != nil {
				return res, err
			}
			line := argumentsRefusedLog
			if slot.set {
				line = slot.line
			}
			// One Write under the lock keeps concurrent calls' lines whole.
			mu.Lock()
			defer mu.Unlock()
			_, _ = fmt.Fprintf(w, "quarry: mcp: %s: %s\n", call.Params.Name, line)
			return res, err
		}
	}
}

// stoppedLine is the timeout line of a tool with nothing to suggest but trying again.
func stoppedLine(name string) stoppedFunc {
	return func(limit string) string { return name + " stopped after " + limit + "; try again" }
}

// queryStoppedLine is the query tool's timeout line, which names the way to a faster query.
func queryStoppedLine(limit string) string {
	return toolQuery + " stopped after " + limit + "; aggregate or filter it in SQL, then try again"
}
