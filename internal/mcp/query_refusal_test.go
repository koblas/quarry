package mcp_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
)

func Test_query_words_each_failure_for_the_model_and_logs_only_its_class(t *testing.T) {
	const (
		writeLine    = "query only reads quarry's store; it cannot change data. Fixes are made in Quicken, then the user runs quarry sync"
		externalLine = "query reads only quarry's store; other files, databases and extensions are turned off"
	)
	expired, cancelExpired := context.WithDeadline(t.Context(), time.Unix(0, 0))
	defer cancelExpired()
	const failedLog = "failed; details went to the client only"
	cases := []struct {
		name    string
		err     error
		want    string
		wantLog string
	}{
		{
			name: "a column it cannot print", err: &store.UnprintableValueError{Column: "doc", Type: "JSON"},
			want:    `cannot print column "doc" of type JSON; cast it in the query, e.g. CAST(doc AS VARCHAR)`,
			wantLog: "query failed: a result column has a type quarry cannot print; details went to the client only",
		},
		{
			name: "a query the database rejects", err: &store.QueryError{Reason: "Binder Error: no such column"},
			want:    "query failed: Binder Error: no such column",
			wantLog: "query failed: Binder Error; details went to the client only",
		},
		{name: "a query holding no statement", err: store.ErrEmptyQuery, want: blankSQLLine, wantLog: blankSQLLine},
		{name: "a write", err: store.ErrReadOnlyQuery, want: writeLine, wantLog: writeLine},
		{name: "a write wrapped by the store", err: fmt.Errorf("run: %w", store.ErrReadOnlyQuery), want: writeLine, wantLog: writeLine},
		{name: "another file", err: store.ErrExternalAccess, want: externalLine, wantLog: externalLine},
		{
			name: "an interrupted query keeps its text", err: store.Interrupted(context.Canceled),
			want: "query interrupted: context canceled", wantLog: failedLog,
		},
		{
			name: "a query a deadline interrupted", err: store.InterruptedBy(expired, errDriverInterrupt),
			want:    "query stopped after 30 seconds; aggregate or filter it in SQL, then try again",
			wantLog: "query stopped after 30 seconds; aggregate or filter it in SQL, then try again",
		},
		{name: "a fault with no reason of its own", err: errDiskOnFire, want: "disk on fire", wantLog: failedLog},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t, &fakeStore{err: c.err}, nil)

			result := h.query(t, map[string]any{"sql": "SELECT 1"})

			assert.True(t, result.IsError)
			assert.Equal(t, c.want, textOf(t, result))
			assert.Nil(t, result.StructuredContent)
			assert.Equal(t, logPrefixQuery+c.wantLog+"\n", h.stderr.String())
		})
	}
}

func Test_query_refuses_a_store_it_cannot_read_with_the_same_line_on_stderr(t *testing.T) {
	const (
		atStore      = "~/Library/Application Support/quarry/quarry.duckdb"
		snapshotPath = testHome + "/Library/Application Support/quarry/snapshots/20260927T143005Z.sqlite"
	)
	cases := []struct {
		name string
		err  error
		want string
	}{
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
			assert.Equal(t, logPrefixQuery+c.want+"\n", h.stderr.String())
		})
	}
}

func Test_query_logs_only_the_class_of_a_rejected_query(t *testing.T) {
	cases := []struct {
		name   string
		reason string
		want   string
	}{
		{name: "a conversion error carrying a stored value", reason: "Conversion Error: Could not convert string 'Chequing' to INT32", want: "Conversion Error"},
		{name: "a catalog error carrying a path", reason: "Catalog Error: Table with name /etc/hosts does not exist!", want: "Catalog Error"},
		{name: "a parser error", reason: "Parser Error: syntax error at or near \"SELEKT\"", want: "Parser Error"},
		{name: "a one-word class", reason: "Binder Error: no such column", want: "Binder Error"},
		{name: "a lower-case prefix is not a class", reason: "chequing: Conversion Error: x", want: "SQL error"},
		{name: "a prefix not ending in Error is not a class", reason: "Chequing Savings: x", want: "SQL error"},
		{name: "a prefix with a digit is not a class", reason: "Payee 42 Error: x", want: "SQL error"},
		{name: "text with no separator is not a class", reason: "Binder Error", want: "SQL error"},
		{name: "a reason that opens with the separator", reason: ": Binder Error", want: "SQL error"},
		{name: "a class on a later line is not a class", reason: "Chequing\nBinder Error: x", want: "SQL error"},
		{name: "a reason of several lines opens with its class", reason: "Binder Error: x\nLINE 1: SELECT y", want: "Binder Error"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t, &fakeStore{err: &store.QueryError{Reason: c.reason}}, nil)

			result := h.query(t, map[string]any{"sql": "SELECT 1"})

			assert.True(t, result.IsError)
			assert.Equal(t, logPrefixQuery+"query failed: "+c.want+"; details went to the client only\n", h.stderr.String())
		})
	}
}
