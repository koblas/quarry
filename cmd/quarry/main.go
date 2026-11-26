package main

import (
	"context"
	"os"
)

func main() {
	// uncovered: main is the process entrypoint; signalContext and runProcess are exercised directly by their own tests.
	ctx, stop := signalContext(context.Background())
	code := runProcess(ctx, os.Args[1:], os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}
