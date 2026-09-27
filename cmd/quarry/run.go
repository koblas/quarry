package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/koblas/quarry/internal/cli"
	"github.com/koblas/quarry/internal/quicken/v9"
	"github.com/koblas/quarry/internal/snapshot"
)

// run is the process entrypoint's testable body: it resolves the home
// directory, builds the snapshot server, and delegates to cli.Execute,
// returning the process exit code (0 success, 1 failure, 2 usage).
func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	home, err := os.UserHomeDir()
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "quarry: %s\n", err)
		return 1
	}

	ref, err := v9.Reference(ctx)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "quarry: %s\n", err)
		return 1
	}

	srv := snapshot.NewServer(
		snapshot.WithSnapshotDir(filepath.Join(home, "Library", "Application Support", "quarry", "snapshots")),
		snapshot.WithReference(v9.ReferenceLabel, ref),
		snapshot.WithHome(home),
	)

	if err := cli.Execute(ctx, args, stdout, stderr, srv, home); err != nil {
		_, _ = fmt.Fprintf(stderr, "quarry: %s\n", err)

		var ue cli.UsageError
		if errors.As(err, &ue) {
			return 2
		}
		return 1
	}
	return 0
}
