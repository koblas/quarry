package main

import (
	"context"
	"os"
)

func main() {
	// unreachable: main is the process entrypoint; run is exercised directly by its own tests.
	os.Exit(run(context.Background(), os.Args[1:], os.Stdout, os.Stderr))
}
