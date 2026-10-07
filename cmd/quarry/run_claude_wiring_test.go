package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Reads HOME and PATH from the process: no t.Parallel.
// The claude file has no shebang, so the kernel refuses to exec it.
func Test_the_shipped_claude_install_reports_a_claude_it_cannot_run(t *testing.T) {
	home := t.TempDir()
	bin := filepath.Join(home, "bin")
	require.NoError(t, os.Mkdir(bin, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(bin, "claude"), []byte("not a program\n"), 0o755)) //nolint:gosec // LookPath only finds an executable file
	t.Setenv("HOME", home)
	t.Setenv("PATH", bin)
	var stdout, stderr bytes.Buffer

	exitCode := runProcess(t.Context(), []string{"claude", "install"}, &stdout, &stderr)

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout.String())
	assert.Equal(t, `quarry: claude install: cannot run claude at "~/bin/claude" (exec format error); `+
		"check that it is Claude Code and that you can run it, then run quarry claude install again\n", stderr.String())
}
