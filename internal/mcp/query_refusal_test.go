package mcp_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
)

func Test_query_words_each_failure_for_the_model(t *testing.T) {
	const (
		writeLine    = "query only reads quarry's store; it cannot change data. Fixes are made in Quicken, then the user runs quarry sync"
		externalLine = "query reads only quarry's store; other files, databases and extensions are turned off"
		atStore      = "~/Library/Application Support/quarry/quarry.duckdb"
		snapshotPath = testHome + "/Library/Application Support/quarry/snapshots/20260927T143005Z.sqlite"
	)
	cases := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "a column it cannot print", err: &store.UnprintableValueError{Column: "doc", Type: "JSON"},
			want: `cannot print column "doc" of type JSON; cast it in the query, e.g. CAST(doc AS VARCHAR)`,
		},
		{
			name: "a query the database rejects", err: &store.QueryError{Reason: "Binder Error: no such column"},
			want: "query failed: Binder Error: no such column",
		},
		{name: "a query holding no statement", err: store.ErrEmptyQuery, want: blankSQLLine},
		{name: "a write", err: store.ErrReadOnlyQuery, want: writeLine},
		{name: "a write wrapped by the store", err: fmt.Errorf("run: %w", store.ErrReadOnlyQuery), want: writeLine},
		{name: "another file", err: store.ErrExternalAccess, want: externalLine},
		{name: "an interrupted query keeps its text", err: store.Interrupted(context.Canceled), want: "query interrupted: context canceled"},
		{name: "a fault with no reason of its own", err: errDiskOnFire, want: "disk on fire"},
		{
			name: "no store yet", err: &store.OpenError{Fault: store.OpenFaultMissing, Path: testStorePath},
			want: "no store at " + atStore + " yet; run quarry sync to build it",
		},
		{
			name: "a store from another version, naming its snapshot",
			err:  &store.OpenError{Fault: store.OpenFaultOtherFormat, Path: testStorePath, SnapshotPath: snapshotPath},
			want: "the store at " + atStore + " was built by another version of quarry; run quarry sync --from 20260927T143005Z to rebuild it",
		},
		{
			name: "a store from another version, with no snapshot",
			err:  &store.OpenError{Fault: store.OpenFaultOtherFormat, Path: testStorePath},
			want: "the store at " + atStore + " was built by another version of quarry; run quarry sync to rebuild it",
		},
		{
			name: "a store that is not DuckDB", err: &store.OpenError{Fault: store.OpenFaultNotDuckDB, Path: testStorePath},
			want: "cannot read the store at " + atStore + ": the file is not a DuckDB database; run quarry sync to rebuild it",
		},
		{
			name: "a store it may not read", err: &store.OpenError{Fault: store.OpenFaultPermission, Path: testStorePath},
			want: "cannot read the store at " + atStore + ": permission denied; run quarry sync to rebuild it",
		},
		{
			name: "a store unreadable for another reason", err: &store.OpenError{Fault: store.OpenFaultOther, Path: testStorePath, Reason: "IO Error at " + testStorePath},
			want: "cannot read the store at " + atStore + ": IO Error at " + atStore + "; run quarry sync to rebuild it",
		},
		{
			name: "a store another program holds", err: &store.OpenError{Fault: store.OpenFaultLocked, Path: testStorePath},
			want: "cannot read the store at " + atStore + ": another program has it open for writing; close that program and run the command again",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t, &fakeStore{err: c.err}, nil)

			result := h.query(t, map[string]any{"sql": "SELECT 1"})

			assert.True(t, result.IsError)
			assert.Equal(t, c.want, textOf(t, result))
			assert.Nil(t, result.StructuredContent)
			assert.Equal(t, logPrefixQuery+c.want+"\n", h.stderr.String())
		})
	}
}
