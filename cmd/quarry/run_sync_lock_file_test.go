// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	lockShown  = "~/Library/Application Support/quarry/quarry.lock"
	quarryDirS = "~/Library/Application Support/quarry"
)

// unusableLockRow arranges the quarry folder under home so its lock file is unusable.
type unusableLockRow struct {
	name     string
	arrange  func(t *testing.T, home, quarryDir string)
	wantLine string
}

func unusableLockRows() []unusableLockRow {
	return []unusableLockRow{
		{
			name: "a directory",
			arrange: func(t *testing.T, _, quarryDir string) {
				t.Helper()
				require.NoError(t, os.Mkdir(lockPathUnder(quarryDir), 0o700))
			},
			wantLine: "quarry: " + lockShown + " is not a regular file; remove it, then run the command again\n",
		},
		{
			name: "a symlink to a regular file",
			arrange: func(t *testing.T, home, quarryDir string) {
				t.Helper()
				target := filepath.Join(home, "elsewhere.lock")
				require.NoError(t, os.WriteFile(target, nil, 0o600))
				require.NoError(t, os.Symlink(target, lockPathUnder(quarryDir)))
			},
			wantLine: "quarry: " + lockShown + " is not a regular file; remove it, then run the command again\n",
		},
		{
			name: "a file with mode 0000",
			arrange: func(t *testing.T, _, quarryDir string) {
				t.Helper()
				skipAsRoot(t)
				require.NoError(t, os.WriteFile(lockPathUnder(quarryDir), nil, 0o000))
			},
			wantLine: "quarry: cannot open " + lockShown + ": permission denied; " +
				"make it readable by your user, or remove it, then run the command again\n",
		},
		{
			name: "a read-only folder with no lock file",
			arrange: func(t *testing.T, _, quarryDir string) {
				t.Helper()
				skipAsRoot(t)
				require.NoError(t, os.Chmod(quarryDir, 0o500))
				t.Cleanup(func() { assert.NoError(t, os.Chmod(quarryDir, 0o700)) })
			},
			wantLine: "quarry: cannot create " + lockShown + ": permission denied; " +
				"make " + quarryDirS + " writable by your user, then run the command again\n",
		},
	}
}

func Test_run_sync_refuses_an_unusable_lock_file_with_a_fix(t *testing.T) {
	for _, row := range unusableLockRows() {
		for _, cell := range outputCells {
			t.Run(row.name+" "+cell.name, func(t *testing.T) {
				home := t.TempDir()
				t.Setenv("HOME", home)
				writeStatusFixtureBundle(t, home)
				quarryDir := storeDirUnder(home)
				require.NoError(t, os.MkdirAll(quarryDir, 0o700))
				sentinel := []byte("previous store bytes, untouched while the lock file is unusable")
				storePath := filepath.Join(quarryDir, "quarry.duckdb")
				require.NoError(t, os.WriteFile(storePath, sentinel, 0o600))
				row.arrange(t, home, quarryDir)

				exitCode, stdout, stderr := runQuarry(t, append([]string{"sync"}, cell.flag...)...)

				assert.Equal(t, 1, exitCode)
				assert.Empty(t, stdout)
				assert.Equal(t, row.wantLine, stderr)
				assert.NoDirExists(t, filepath.Join(quarryDir, "snapshots"))
				after, err := os.ReadFile(storePath)
				require.NoError(t, err)
				assert.Equal(t, sentinel, after)
			})
		}
	}
}
