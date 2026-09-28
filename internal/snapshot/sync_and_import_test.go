package snapshot_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/koblas/quarry/internal/platform/homepath"
	"github.com/koblas/quarry/internal/quicken/v9"
	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/koblas/quarry/internal/snapshot"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeImporter is a hand-written Importer fake: it records every path it
// was called with and returns the configured result, or err when set.
type fakeImporter struct {
	calls  []string
	result store.Result
	err    error
}

func (f *fakeImporter) Import(_ context.Context, snapshotPath string) (store.Result, error) {
	f.calls = append(f.calls, snapshotPath)
	return f.result, f.err
}

// newImportServer builds a Server with a snapshots directory and a store
// path both under home, so refusal copy has something real to abbreviate.
func newImportServer(t *testing.T, home string, imp snapshot.Importer) *snapshot.Server {
	t.Helper()
	ref, err := v9.Reference(t.Context())
	require.NoError(t, err)
	return snapshot.NewServer(
		snapshot.WithSnapshotDir(filepath.Join(home, "snapshots")),
		snapshot.WithReference(v9.ReferenceLabel, ref),
		snapshot.WithHome(home),
		snapshot.WithImporter(imp),
		snapshot.WithStorePath(filepath.Join(home, "quarry", "quarry.duckdb")),
	)
}

func snapshotIDFromPath(path string) string {
	return strings.TrimSuffix(filepath.Base(path), ".sqlite")
}

func Test_sync_and_import_imports_the_committed_snapshot(t *testing.T) {
	bundle := v9fixture.OpenBundle(t, t.TempDir())
	home := t.TempDir()
	fake := &fakeImporter{result: store.Result{
		Path: filepath.Join(home, "quarry", "quarry.duckdb"), Counts: store.Counts{Accounts: 2},
	}}
	srv := newImportServer(t, home, fake)

	outcome, err := srv.SyncAndImport(t.Context(), bundle.Dir)

	require.NoError(t, err)
	require.Len(t, fake.calls, 1)
	assert.Equal(t, outcome.Manifest.Snapshot.Path, fake.calls[0])
	require.NotNil(t, outcome.Store)
	assert.Equal(t, fake.result, *outcome.Store)
}

func Test_sync_and_import_skips_the_import_on_a_schema_mismatch(t *testing.T) {
	bundle := v9fixture.MissingSchemaBundle(t, t.TempDir())
	home := t.TempDir()
	fake := &fakeImporter{}
	srv := newImportServer(t, home, fake)

	outcome, err := srv.SyncAndImport(t.Context(), bundle.Dir)

	var mismatch snapshot.MismatchError
	require.ErrorAs(t, err, &mismatch)
	assert.Empty(t, fake.calls)
	assert.Nil(t, outcome.Store)
}

var errImportBoom = errors.New("boom from duckdb")

// The store refusal wraps the importer's own error (errors.Is) so a typed
// error such as *importer.UnmappableError still reaches a caller that asserts on it.
func Test_sync_and_import_frames_an_import_failure_as_a_store_refusal(t *testing.T) {
	bundle := v9fixture.OpenBundle(t, t.TempDir())
	home := t.TempDir()
	fake := &fakeImporter{err: errImportBoom}
	srv := newImportServer(t, home, fake)

	outcome, err := srv.SyncAndImport(t.Context(), bundle.Dir)

	require.Error(t, err)
	assert.ErrorIs(t, err, errImportBoom)
	assert.Nil(t, outcome.Store)
	id := snapshotIDFromPath(outcome.Manifest.Snapshot.Path)
	want := fmt.Sprintf("cannot build the store in %s: %s; run quarry sync --from %s",
		homepath.Abbreviate(home, filepath.Join(home, "quarry")), errImportBoom.Error(), id)
	assert.Equal(t, want, err.Error())
	assert.FileExists(t, outcome.Manifest.Snapshot.Path)
	assert.FileExists(t, outcome.Manifest.Snapshot.Manifest)
}

func Test_sync_and_import_does_not_import_when_the_snapshot_fails(t *testing.T) {
	home := t.TempDir()
	fake := &fakeImporter{}
	ref, err := v9.Reference(t.Context())
	require.NoError(t, err)
	srv := snapshot.NewServer(
		snapshot.WithSnapshotDir(filepath.Join(home, "snapshots")),
		snapshot.WithReference(v9.ReferenceLabel, ref),
		snapshot.WithSource(&fakeSource{openErr: errBoom}),
		snapshot.WithImporter(fake),
		snapshot.WithStorePath(filepath.Join(home, "quarry", "quarry.duckdb")),
	)

	outcome, err := srv.SyncAndImport(t.Context(), filepath.Join(home, "Home.quicken"))

	require.Error(t, err)
	var mismatch snapshot.MismatchError
	assert.False(t, errors.As(err, &mismatch))
	assert.Empty(t, fake.calls)
	assert.Nil(t, outcome.Store)
}

// Store must be the unbuilt result, not nil, so StdoutWriteRefusal already
// picks the --from --json form rather than the one naming the kept snapshot.
// Path is populated on the returned copy even though the importer's own
// Result contract leaves it empty on a failed build.
func Test_sync_and_import_keeps_the_store_result_when_validation_fails(t *testing.T) {
	bundle := v9fixture.OpenBundle(t, t.TempDir())
	home := t.TempDir()
	result := store.Result{Built: false, Validation: store.Validation{
		Balances: store.BalanceCheck{Checked: 1, Mismatched: []store.BalanceMismatch{{ID: "acct-1"}}},
	}}
	fake := &fakeImporter{result: result, err: store.ErrValidationFailed}
	srv := newImportServer(t, home, fake)

	outcome, err := srv.SyncAndImport(t.Context(), bundle.Dir)

	require.ErrorIs(t, err, store.ErrValidationFailed)
	require.NotNil(t, outcome.Store)
	want := result
	want.Path = filepath.Join(home, "quarry", "quarry.duckdb")
	assert.Equal(t, want, *outcome.Store)
	assert.Contains(t, err.Error(), "validation failed")

	writeErr := outcome.StdoutWriteRefusal(home, errBoom)
	assert.Contains(t, writeErr.Error(), "--from "+snapshotIDFromPath(outcome.Manifest.Snapshot.Path)+" --json")
}

// A previous store existing or not decides the V1 block's NOT REBUILT vs
// NOT BUILT line.
func Test_sync_and_import_reports_whether_a_previous_store_existed(t *testing.T) {
	cases := []struct {
		name        string
		createStore bool
		wantExisted bool
	}{
		{name: "no previous store", createStore: false, wantExisted: false},
		{name: "a previous store", createStore: true, wantExisted: true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			bundle := v9fixture.OpenBundle(t, t.TempDir())
			home := t.TempDir()
			storePath := filepath.Join(home, "quarry", "quarry.duckdb")
			if c.createStore {
				require.NoError(t, os.MkdirAll(filepath.Dir(storePath), 0o700))
				require.NoError(t, os.WriteFile(storePath, []byte("store"), 0o600))
			}
			fake := &fakeImporter{result: store.Result{Built: false}, err: store.ErrValidationFailed}
			srv := newImportServer(t, home, fake)

			outcome, err := srv.SyncAndImport(t.Context(), bundle.Dir)

			require.ErrorIs(t, err, store.ErrValidationFailed)
			assert.Equal(t, c.wantExisted, outcome.StoreExisted)
		})
	}
}

// A stat fault other than "not found" (here ENOTDIR, from a store path
// running through a regular file) is treated as a previous store existing:
// the build failed either way, so the file is unchanged either way.
func Test_sync_and_import_treats_a_stat_fault_as_a_previous_store(t *testing.T) {
	bundle := v9fixture.OpenBundle(t, t.TempDir())
	home := t.TempDir()
	blocker := filepath.Join(home, "quarry")
	require.NoError(t, os.WriteFile(blocker, []byte("not a directory"), 0o600))
	fake := &fakeImporter{result: store.Result{Built: false}, err: store.ErrValidationFailed}
	srv := newImportServer(t, home, fake)

	outcome, err := srv.SyncAndImport(t.Context(), bundle.Dir)

	require.ErrorIs(t, err, store.ErrValidationFailed)
	assert.True(t, outcome.StoreExisted)
}

// Count form is "X of Y <noun>": the noun agrees with Y, the verb with X.
func Test_sync_and_import_reports_the_v1_refusal_for_a_failed_check(t *testing.T) {
	cases := []struct {
		name       string
		validation store.Validation
		wantClause string
	}{
		{
			name: "one of one account mismatched",
			validation: store.Validation{Balances: store.BalanceCheck{
				Checked: 1, Mismatched: []store.BalanceMismatch{{ID: "acct-1"}},
			}},
			wantClause: "1 of 1 account does not match Quicken's last reconciled balance",
		},
		{
			name: "one balance mismatch",
			validation: store.Validation{Balances: store.BalanceCheck{
				Checked: 3, Mismatched: []store.BalanceMismatch{{ID: "acct-1"}},
			}},
			wantClause: "1 of 3 accounts does not match Quicken's last reconciled balance",
		},
		{
			name: "two balance mismatches",
			validation: store.Validation{Balances: store.BalanceCheck{
				Checked: 3, Mismatched: []store.BalanceMismatch{{ID: "acct-1"}, {ID: "acct-2"}},
			}},
			wantClause: "2 of 3 accounts do not match Quicken's last reconciled balance",
		},
		{
			name: "every checked account mismatched",
			validation: store.Validation{Balances: store.BalanceCheck{
				Checked: 3, Mismatched: []store.BalanceMismatch{{ID: "acct-1"}, {ID: "acct-2"}, {ID: "acct-3"}},
			}},
			wantClause: "3 of 3 accounts do not match Quicken's last reconciled balance",
		},
		{
			name:       "one split mismatch",
			validation: store.Validation{Splits: store.SplitCheck{Mismatched: []store.SplitMismatch{{ID: "txn-1"}}}},
			wantClause: "1 transaction does not equal the sum of its splits",
		},
		{
			name: "two split mismatches",
			validation: store.Validation{
				Splits: store.SplitCheck{Mismatched: []store.SplitMismatch{{ID: "txn-1"}, {ID: "txn-2"}}},
			},
			wantClause: "2 transactions do not equal the sum of their splits",
		},
		{
			name: "a balance and a split mismatch joined",
			validation: store.Validation{
				Balances: store.BalanceCheck{Checked: 3, Mismatched: []store.BalanceMismatch{{ID: "acct-1"}}},
				Splits:   store.SplitCheck{Mismatched: []store.SplitMismatch{{ID: "txn-1"}}},
			},
			wantClause: "1 of 3 accounts does not match Quicken's last reconciled balance and " +
				"1 transaction does not equal the sum of its splits",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			bundle := v9fixture.OpenBundle(t, t.TempDir())
			home := t.TempDir()
			result := store.Result{Built: false, Counts: store.Counts{Accounts: 3}, Validation: c.validation}
			fake := &fakeImporter{result: result, err: store.ErrValidationFailed}
			srv := newImportServer(t, home, fake)

			outcome, err := srv.SyncAndImport(t.Context(), bundle.Dir)

			require.ErrorIs(t, err, store.ErrValidationFailed)
			require.NotNil(t, outcome.Store)
			storePath := filepath.Join(home, "quarry", "quarry.duckdb")
			wantResult := result
			wantResult.Path = storePath
			assert.Equal(t, wantResult, *outcome.Store)
			want := fmt.Sprintf("validation failed: %s; %s was not changed; each difference is listed on stdout; "+
				"fix the account in Quicken and run quarry sync, or run quarry sync --from %s after updating quarry",
				c.wantClause, homepath.Abbreviate(home, storePath), snapshotIDFromPath(outcome.Manifest.Snapshot.Path))
			assert.Equal(t, want, err.Error())
		})
	}
}

func Test_sync_and_import_refuses_without_an_importer(t *testing.T) {
	bundle := v9fixture.OpenBundle(t, t.TempDir())
	home := t.TempDir()
	srv := newImportServer(t, home, nil)

	outcome, err := srv.SyncAndImport(t.Context(), bundle.Dir)

	require.Error(t, err)
	assert.Nil(t, outcome.Store)
}
