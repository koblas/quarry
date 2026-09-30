// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// quickenPathConfig is a config file whose only setting is quicken.path.
func quickenPathConfig(path string) string {
	return "[quicken]\npath = \"" + path + "\"\n"
}

// assertRefusedBeforeSnapshotting checks the one-line refusal contract: exit 1,
// empty stdout, exactly wantStderr, and no snapshots folder under home.
func assertRefusedBeforeSnapshotting(t *testing.T, home string, exitCode int, stdout, stderr, wantStderr string) {
	t.Helper()
	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout)
	assert.Equal(t, wantStderr, stderr)
	_, statErr := os.Stat(filepath.Join(storeDirUnder(home), "snapshots"))
	assert.ErrorIs(t, statErr, os.ErrNotExist)
}

func Test_run_sync_snapshots_the_file_named_by_quicken_path(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	bundle := v9fixture.OpenBundle(t, filepath.Join(home, "Books"))
	require.NoError(t, os.MkdirAll(filepath.Join(home, "Documents", "A.quicken"), 0o700))
	require.NoError(t, os.MkdirAll(filepath.Join(home, "Documents", "B.quicken"), 0o700))
	writeConfig(t, home, quickenPathConfig("~/Books/Home.quicken"))
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sync"}, &stdout, &stderr)

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Contains(t, stdout.String(), "Source    "+abbreviated(t, bundle.Dir, home)+"\n")
}

func Test_run_sync_refuses_a_quicken_path_that_does_not_exist(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeConfig(t, home, quickenPathConfig("~/Books/Missing.quicken"))
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sync"}, &stdout, &stderr)

	assertRefusedBeforeSnapshotting(t, home, exitCode, stdout.String(), stderr.String(),
		"quarry: ~/Books/Missing.quicken does not exist; check quicken.path in "+configShown+
			", or pass the file with --quicken <path>\n")
}

func Test_run_sync_refuses_a_quicken_path_that_is_not_a_bundle(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	require.NoError(t, os.MkdirAll(filepath.Join(home, "Books"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(home, "Books", "notes.txt"), []byte("x"), 0o600))
	writeConfig(t, home, quickenPathConfig("~/Books/notes.txt"))
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sync"}, &stdout, &stderr)

	assertRefusedBeforeSnapshotting(t, home, exitCode, stdout.String(), stderr.String(),
		"quarry: ~/Books/notes.txt is not a Quicken for Mac file "+
			"(expected a .quicken bundle containing a data file); "+
			"set quicken.path in "+configShown+" to the .quicken bundle\n")
}

func Test_run_sync_prefers_the_quicken_flag_over_quicken_path(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	bundle := v9fixture.OpenBundle(t, filepath.Join(home, "Books"))
	require.NoError(t, os.MkdirAll(filepath.Join(home, "Documents"), 0o700))
	link := filepath.Join(home, "Documents", "A.quicken")
	require.NoError(t, os.Symlink(bundle.Dir, link))
	writeConfig(t, home, quickenPathConfig("~/Books/Missing.quicken"))
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sync", "--quicken", link}, &stdout, &stderr)

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Contains(t, stdout.String(), "Source    "+abbreviated(t, link, home)+"\n")
}

func Test_run_sync_from_ignores_quicken_path(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	bundle := v9fixture.OpenBundle(t, filepath.Join(home, "Books"))
	var syncStdout, syncStderr bytes.Buffer
	require.Equal(t, 0, run(context.Background(), []string{"sync", "--quicken", bundle.Dir}, &syncStdout, &syncStderr), syncStderr.String())
	snapshotsDir := filepath.Join(storeDirUnder(home), "snapshots")
	id := snapshotID(onlyFileWithSuffix(t, snapshotsDir, ".sqlite"))
	storePath := filepath.Join(storeDirUnder(home), "quarry.duckdb")
	require.NoError(t, os.Remove(storePath))
	writeConfig(t, home, quickenPathConfig("~/Books/Missing.quicken"))
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sync", "--from", id}, &stdout, &stderr)

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.FileExists(t, storePath)
}
