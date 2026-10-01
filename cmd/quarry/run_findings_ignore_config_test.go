package main

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_run_sync_refuses_a_findings_ignore_that_is_not_a_list_before_taking_a_snapshot(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeStatusFixtureBundle(t, home)
	quarryDir := storeDirUnder(home)
	var goodStdout, goodStderr bytes.Buffer
	require.Equal(t, 0, run(context.Background(), []string{"sync"}, &goodStdout, &goodStderr), goodStderr.String())
	storeBefore := fileDigest(t, filepath.Join(quarryDir, "quarry.duckdb"))
	writeConfig(t, home, "findings.ignore = \"duplicate:txn-1+txn-2\"\n")
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sync"}, &stdout, &stderr)

	assert.Equal(t, 1, countFilesWithSuffix(t, filepath.Join(quarryDir, "snapshots"), ".sqlite"))
	assert.Equal(t, storeBefore, fileDigest(t, filepath.Join(quarryDir, "quarry.duckdb")))
	assert.Empty(t, stdout.String())
	assert.Equal(t, "quarry: "+configShown+": findings.ignore must be a list of finding ids in quotes, "+
		"such as [\"duplicate:txn-4410+txn-4412\"], got \"duplicate:txn-1+txn-2\""+configFix+"\n", stderr.String())
	assert.Equal(t, 1, exitCode)
}
