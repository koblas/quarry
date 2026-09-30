// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// countFilesWithSuffix is how many entries in dir end in suffix.
func countFilesWithSuffix(t *testing.T, dir, suffix string) int {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	count := 0
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), suffix) {
			count++
		}
	}
	return count
}

// fileDigest is the hex SHA-256 of the file at path.
func fileDigest(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return fmt.Sprintf("%x", sha256.Sum256(data))
}

func Test_run_sync_refuses_a_malformed_config_before_taking_a_snapshot(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeStatusFixtureBundle(t, home)
	quarryDir := storeDirUnder(home)
	snapshotsDir := filepath.Join(quarryDir, "snapshots")
	var goodStdout, goodStderr bytes.Buffer
	require.Equal(t, 0, run(context.Background(), []string{"sync"}, &goodStdout, &goodStderr), goodStderr.String())
	storeBefore := fileDigest(t, filepath.Join(quarryDir, "quarry.duckdb"))
	require.NoError(t, os.WriteFile(filepath.Join(quarryDir, "config.toml"), []byte("[snapshots\nkeep = 24\n"), 0o600))
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sync"}, &stdout, &stderr)

	assert.Equal(t, 1, countFilesWithSuffix(t, snapshotsDir, ".sqlite"))
	assert.Equal(t, storeBefore, fileDigest(t, filepath.Join(quarryDir, "quarry.duckdb")))
	require.Equal(t, 1, exitCode, stderr.String())
	assert.Empty(t, stdout.String())
	assert.Regexp(t, "^"+regexp.QuoteMeta("quarry: cannot read ~/Library/Application Support/quarry/config.toml: line 1: ")+
		"[^\n]+"+regexp.QuoteMeta("; fix the file and run the command again")+"\n$", stderr.String())
}
