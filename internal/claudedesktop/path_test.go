package claudedesktop_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/koblas/quarry/internal/claudedesktop"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errNoQuarryOnPath = errors.New("quarry not found in PATH")

// writeQuarry creates an executable file named quarry in dir and returns its path.
func writeQuarry(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "quarry")
	require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755)) //nolint:gosec // an executable stand-in
	return path
}

// installWithPath runs Install with the running binary at exe, PATH resolving quarry through
// lookPath, and temp as the temporary directory root.
func installWithPath(t *testing.T, home, exe, temp string, lookPath claudedesktop.LookPath) (claudedesktop.Result, error) {
	t.Helper()
	srv := claudedesktop.NewServer(
		claudedesktop.WithHome(home),
		claudedesktop.WithExecutable((&fakeExecutable{path: exe}).executable),
		claudedesktop.WithLookPath(lookPath),
		claudedesktop.WithTempDir(temp),
	)
	return srv.Install(t.Context())
}

func lookPathReturning(path string) claudedesktop.LookPath {
	return func(string) (string, error) { return path, nil }
}

func Test_install_writes_the_quarry_path_that_claude_desktop_will_start(t *testing.T) {
	t.Parallel()

	t.Run("the PATH link to the running binary is written, with no PATH warning", func(t *testing.T) {
		t.Parallel()
		home, folder := desktopFolder(t)
		cellar := writeQuarry(t, t.TempDir())
		pathLink := filepath.Join(t.TempDir(), "quarry")
		require.NoError(t, os.Symlink(cellar, pathLink))

		res, err := installWithPath(t, home, cellar, t.TempDir(), lookPathReturning(pathLink))

		require.NoError(t, err)
		assert.Equal(t, claudedesktop.Result{Folder: folder, Config: filepath.Join(folder, configName), Command: pathLink}, res)
		assert.JSONEq(t, quarryEntryConfig(`{"command":"`+pathLink+`","args":["mcp"]}`), readConfig(t, folder))
	})

	t.Run("the running binary is written and the other PATH file is reported when they differ", func(t *testing.T) {
		t.Parallel()
		home, folder := desktopFolder(t)
		running := writeQuarry(t, t.TempDir())
		onPath := writeQuarry(t, t.TempDir())

		res, err := installWithPath(t, home, running, t.TempDir(), lookPathReturning(onPath))

		require.NoError(t, err)
		assert.Equal(t, claudedesktop.Result{Folder: folder, Config: filepath.Join(folder, configName), Command: running, PathQuarry: onPath}, res)
		assert.JSONEq(t, quarryEntryConfig(`{"command":"`+running+`","args":["mcp"]}`), readConfig(t, folder))
	})

	t.Run("the running binary is written with no PATH warning when PATH holds no quarry", func(t *testing.T) {
		t.Parallel()
		home, folder := desktopFolder(t)
		running := writeQuarry(t, t.TempDir())
		lookPath := func(string) (string, error) { return "", errNoQuarryOnPath }

		res, err := installWithPath(t, home, running, t.TempDir(), lookPath)

		require.NoError(t, err)
		assert.Equal(t, claudedesktop.Result{Folder: folder, Config: filepath.Join(folder, configName), Command: running}, res)
		assert.JSONEq(t, quarryEntryConfig(`{"command":"`+running+`","args":["mcp"]}`), readConfig(t, folder))
	})

	t.Run("a binary under the temporary directory is refused and nothing is written", func(t *testing.T) {
		t.Parallel()
		home, folder := desktopFolder(t)
		temp := t.TempDir()
		built := writeQuarry(t, temp)

		_, err := installWithPath(t, home, built, temp, lookPathReturning(""))

		tempErr, ok := errors.AsType[*claudedesktop.TempBuildError](err)
		require.True(t, ok, "got %v", err)
		assert.Equal(t, built, tempErr.Path)
		assert.Empty(t, snapshot(t, folder))
	})
}
