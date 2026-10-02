package main

import (
	"context"
	"os"
)

func main() {
	ctx, stop := signalContext(context.Background())
	// unreachable: main is the process entrypoint; signalContext and runProcess are exercised directly by their own tests.
	code := runProcess(ctx, os.Args[1:], os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}
