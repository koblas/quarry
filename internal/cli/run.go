package cli

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/koblas/quarry/internal/config"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/snapshot"
)

// ServerFactory builds the Server sync needs, applying opts after its own
// options. It is called only from inside sync's RunE, so building the
// command tree, --help and a usage error never depend on it.
type ServerFactory func(ctx context.Context, opts ...snapshot.Option) (srv *snapshot.Server, err error)

// ReportFactory builds the Server a read command needs; command is the name
// of the command asking, for refusals that say which command to run again.
// Like ServerFactory it is called only from a command's RunE.
type ReportFactory func(ctx context.Context, command string) (srv *report.Server, err error)

// SnapshotsFactory builds the Server the snapshots command needs; command is
// the name of the command asking, for the home-directory refusal. Like the
// other factories it is called only from a command's RunE.
type SnapshotsFactory func(ctx context.Context, command string) (srv *snapshot.Server, err error)

// MCPServeFunc serves MCP to a client over stdin and stdout until the client
// closes stdin or ctx ends, returning a transport or protocol failure.
// stderr is for diagnostics only; stdout carries nothing but the protocol.
type MCPServeFunc func(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer) error

// TerminalProbe reports whether r is an interactive terminal rather than a
// pipe, file or in-memory reader.
type TerminalProbe func(r io.Reader) bool

// ConfigLoader reads quarry's config file for command, the name of the
// command asking, for refusals that say which command to run again. Like
// the factories it is called only from a command's RunE.
type ConfigLoader func(command string) (config.Config, error)

// Env is everything Execute takes from the process: its streams, the clock
// and the factories that build each command family's Server. Now must be
// set for any command that reads the date, such as spend or cashflow; a nil Now panics.
type Env struct {
	Stdin          io.Reader
	Stdout, Stderr io.Writer
	NewServer      ServerFactory
	NewReport      ReportFactory
	NewSnapshots   SnapshotsFactory
	LoadConfig     ConfigLoader
	ServeMCP       MCPServeFunc
	IsTerminal     TerminalProbe
	Now            func() time.Time
}

// Execute parses args against quarry's command tree and runs the matched
// command, writing to env's streams. It returns a UsageError unchanged, a
// command's own runtime error unwrapped, or wraps any other error as a
// UsageError pointing at the matched command's --help.
func Execute(ctx context.Context, args []string, env Env) error {
	var jsonOut bool
	root := newRootCommand(env, &jsonOut) //nolint:contextcheck // RunE reads ctx back through cmd.Context(), set by ExecuteContextC below
	// args must be non-nil: cobra falls back to the process's own os.Args for a nil slice.
	root.SetArgs(args)
	root.SetIn(env.Stdin)
	root.SetOut(env.Stdout)
	root.SetErr(env.Stderr)

	cmd, err := root.ExecuteContextC(ctx)
	if err == nil {
		return nil
	}

	if ue, ok := errors.AsType[UsageError](err); ok {
		return ue
	}
	if re, ok := errors.AsType[ReportedError](err); ok {
		return re
	}
	if re, ok := errors.AsType[*runtimeError](err); ok {
		return re.err
	}
	return UsageError{msg: err.Error() + "; Run '" + cmd.CommandPath() + " --help' for usage."}
}
