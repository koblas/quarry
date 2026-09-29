package cli

import (
	"context"
	"errors"
	"io"

	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/snapshot"
)

// ServerFactory builds the Server sync needs. It is called only from
// inside sync's RunE, so building the command tree, --help and a usage
// error never depend on it.
type ServerFactory func(ctx context.Context) (srv *snapshot.Server, err error)

// ReportFactory builds the Server a read command needs; command is the name
// of the command asking, for refusals that say which command to run again.
// Like ServerFactory it is called only from a command's RunE.
type ReportFactory func(ctx context.Context, command string) (srv *report.Server, err error)

// Env is everything Execute takes from the process: its streams and the
// factories that build each command family's Server.
type Env struct {
	Stdin          io.Reader
	Stdout, Stderr io.Writer
	NewServer      ServerFactory
	NewReport      ReportFactory
}

// Execute parses args against quarry's command tree and runs the matched
// command, writing to env's streams. It returns a UsageError unchanged, a
// command's own runtime error unwrapped, or wraps any other error as a
// UsageError with a run-sync-help hint appended.
func Execute(ctx context.Context, args []string, env Env) error {
	var jsonOut bool
	root := newRootCommand(env, &jsonOut) //nolint:contextcheck // RunE reads ctx back through cmd.Context(), set by ExecuteContext below
	// args must be non-nil: cobra falls back to the process's own os.Args for a nil slice.
	root.SetArgs(args)
	root.SetIn(env.Stdin)
	root.SetOut(env.Stdout)
	root.SetErr(env.Stderr)

	err := root.ExecuteContext(ctx)
	if err == nil {
		return nil
	}

	if ue, ok := errors.AsType[UsageError](err); ok {
		return ue
	}
	if re, ok := errors.AsType[*runtimeError](err); ok {
		return re.err
	}
	return UsageError{msg: err.Error() + "; Run 'quarry sync --help' for usage."}
}
