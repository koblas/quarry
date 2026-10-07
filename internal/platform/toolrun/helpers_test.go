package toolrun_test

import (
	"context"
	"os"
	"os/exec"
)

// execSelf prepares a child that re-runs this test binary as the helper name,
// sharing this process's stdout and stderr.
func execSelf(name string) *exec.Cmd {
	cmd := exec.CommandContext(context.Background(), os.Args[0], name) //nolint:gosec // re-runs this test binary
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd
}

// killProcess ends the process with the given pid, if it is still running.
func killProcess(pid int) {
	if p, err := os.FindProcess(pid); err == nil {
		_ = p.Kill()
	}
}
