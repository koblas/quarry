// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// onlyFileWithSuffix fails the test unless exactly one entry in dir ends in
// suffix, returning its full path.
func onlyFileWithSuffix(t *testing.T, dir, suffix string) string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	var found []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), suffix) {
			found = append(found, e.Name())
		}
	}
	require.Len(t, found, 1, "expected exactly one %s file in %s, found %v", suffix, dir, found)
	return filepath.Join(dir, found[0])
}

// abbreviated mirrors the CLI's ~-abbreviation so the expected string is
// built from the real path, not a re-derived one.
func abbreviated(t *testing.T, path, home string) string {
	t.Helper()
	require.True(t, strings.HasPrefix(path, home+string(filepath.Separator)))
	return "~" + strings.TrimPrefix(path, home)
}

// megabytes mirrors the CLI's decimal-MB rounding for a fixture small enough
// that thousands-grouping never applies.
func megabytes(bytes int64) string {
	tenths := (bytes*10 + 500000) / 1000000
	return fmt.Sprintf("%d.%d MB", tenths/10, tenths%10)
}

// snapshotID mirrors the CLI's <id> derivation: a snapshot path's basename
// with the .sqlite extension removed.
func snapshotID(snapshotPath string) string {
	return strings.TrimSuffix(filepath.Base(snapshotPath), ".sqlite")
}
