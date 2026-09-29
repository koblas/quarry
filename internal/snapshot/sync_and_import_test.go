package snapshot_test

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
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

// fakeImporter is a hand-written Importer fake: it records every snapshot
// ref it was called with and returns the configured result, or err when
// set, after calling cancel when interrupt is set.
type fakeImporter struct {
	calls     []store.SnapshotRef
	result    store.Result
	err       error
	interrupt bool
	cancel    context.CancelFunc
}

func (f *fakeImporter) Import(_ context.Context, snap store.SnapshotRef) (store.Result, error) {
	f.calls = append(f.calls, snap)
	if f.interrupt {
		f.cancel()
	}
	return f.result, f.err
}

// taggedBuildError mirrors a store build error classified as sentinel:
// Unwrap reaches cause alone, Is also matches sentinel.
type taggedBuildError struct {
	sentinel error
	cause    error
}

func (e taggedBuildError) Error() string        { return "build store: " + e.cause.Error() }
func (e taggedBuildError) Unwrap() error        { return e.cause }
func (e taggedBuildError) Is(target error) bool { return target == e.sentinel }

// unmappableError mirrors the importer's unmappable-value error: its text is
// the reason, and it matches store.ErrUnmappable.
type unmappableError struct{ reason string }

func (e unmappableError) Error() string        { return e.reason }
func (e unmappableError) Is(target error) bool { return target == store.ErrUnmappable }

// fakeStoreProbe is a hand-written StoreProbe fake reporting a fixed path and existence.
type fakeStoreProbe struct {
	path   string
	exists bool
}

func (f *fakeStoreProbe) Path() string { return f.path }
func (f *fakeStoreProbe) Exists() bool { return f.exists }

// newImportServer builds a Server with a snapshots directory and a store
// path both under home, so refusal copy has something real to abbreviate;
// opts apply last, overriding those.
func newImportServer(t *testing.T, home string, imp snapshot.Importer, opts ...snapshot.Option) *snapshot.Server {
	t.Helper()
	ref, err := v9.Reference(t.Context())
	require.NoError(t, err)
	return snapshot.NewServer(append([]snapshot.Option{
		snapshot.WithSnapshotDir(filepath.Join(home, "snapshots")),
		snapshot.WithReference(v9.ReferenceLabel, ref),
		snapshot.WithHome(home),
		snapshot.WithImporter(imp),
		snapshot.WithStoreProbe(&fakeStoreProbe{path: filepath.Join(home, "quarry", "quarry.duckdb")}),
	}, opts...)...)
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
	assert.Equal(t, outcome.Manifest.Snapshot.Path, fake.calls[0].Path)
	require.NotNil(t, outcome.Store)
	assert.Equal(t, fake.result, *outcome.Store)
}

func Test_sync_and_import_passes_the_manifests_hash_and_fingerprint_to_the_importer(t *testing.T) {
	bundle := v9fixture.OpenBundle(t, t.TempDir())
	home := t.TempDir()
	fake := &fakeImporter{}
	srv := newImportServer(t, home, fake)

	outcome, err := srv.SyncAndImport(t.Context(), bundle.Dir)

	require.NoError(t, err)
	assert.Equal(t, []store.SnapshotRef{{
		Path: outcome.Manifest.Snapshot.Path, SHA256: outcome.Manifest.Snapshot.SHA256,
		SchemaFingerprint: outcome.Manifest.Schema.Fingerprint,
	}}, fake.calls)
	assert.NotEmpty(t, fake.calls[0].SHA256)
	assert.NotEmpty(t, fake.calls[0].SchemaFingerprint)
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

func Test_sync_and_import_reports_each_build_failure_with_its_refusal(t *testing.T) {
	permission := taggedBuildError{sentinel: store.ErrStoreNotWritable, cause: &fs.PathError{
		Op: "open", Path: "quarry.duckdb.partial", Err: fs.ErrPermission,
	}}
	diskFull := taggedBuildError{sentinel: store.ErrDiskFull, cause: errors.New(
		`IO Error: Could not write file "quarry.duckdb.partial": No space left on device`)}
	unmappable := unmappableError{reason: `account "Euro Savings" uses currency EUR; quarry supports CAD and USD accounts`}
	interrupted := func(_, storePath, id string) string {
		return "sync interrupted while building the store; " + storePath + " was not changed; run quarry sync --from " + id + " to rebuild it"
	}
	cases := []struct {
		name      string
		err       error
		interrupt bool
		want      func(storeDir, storePath, id string) string
	}{
		{
			name: "unwritable store directory",
			err:  fmt.Errorf("replace store: %w", permission),
			want: func(storeDir, _, _ string) string {
				return "cannot write to " + storeDir + ": permission denied; make the directory writable by your user"
			},
		},
		{
			name: "disk full",
			err:  fmt.Errorf("replace store: %w", diskFull),
			want: func(storeDir, _, id string) string {
				return "cannot write the store to " + storeDir + ": no space left on device; free disk space, then run quarry sync --from " + id
			},
		},
		{
			name: "other build failure",
			err:  fmt.Errorf("replace store: build store: %w", errImportBoom),
			want: func(storeDir, _, id string) string {
				return "cannot build the store in " + storeDir + ": " + errImportBoom.Error() + "; run quarry sync --from " + id
			},
		},
		{
			name: "unmappable value",
			err:  unmappable,
			want: func(_, storePath, id string) string {
				return "cannot import snapshot " + id + ": " + unmappable.reason + "; " + storePath +
					" was not changed; run quarry sync --from " + id + " once quarry supports it"
			},
		},
		{name: "interrupted", err: fmt.Errorf("replace store: build store: %w", errImportBoom), interrupt: true, want: interrupted},
		{name: "interrupted beats disk full", err: fmt.Errorf("replace store: %w", diskFull), interrupt: true, want: interrupted},
		{name: "interrupted beats an unmappable value", err: unmappable, interrupt: true, want: interrupted},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			bundle := v9fixture.OpenBundle(t, t.TempDir())
			home := t.TempDir()
			ctx, cancel := context.WithCancel(t.Context())
			t.Cleanup(cancel)
			fake := &fakeImporter{err: c.err, interrupt: c.interrupt, cancel: cancel}
			srv := newImportServer(t, home, fake)

			outcome, err := srv.SyncAndImport(ctx, bundle.Dir)

			require.ErrorIs(t, err, c.err)
			assert.Nil(t, outcome.Store)
			storePath := filepath.Join(home, "quarry", "quarry.duckdb")
			assert.Equal(t,
				c.want(homepath.Abbreviate(home, filepath.Dir(storePath)), homepath.Abbreviate(home, storePath),
					snapshotIDFromPath(outcome.Manifest.Snapshot.Path)),
				err.Error())
		})
	}
}

// A permission fault the store did not classify, such as one reading the
// snapshot, does not name the store directory as unwritable.
func Test_sync_and_import_reports_an_untagged_permission_fault_as_s3(t *testing.T) {
	bundle := v9fixture.OpenBundle(t, t.TempDir())
	home := t.TempDir()
	readErr := &fs.PathError{Op: "open", Path: "snapshot.sqlite", Err: fs.ErrPermission}
	fake := &fakeImporter{err: fmt.Errorf("open snapshot.sqlite: %w", readErr)}
	srv := newImportServer(t, home, fake)

	outcome, err := srv.SyncAndImport(t.Context(), bundle.Dir)

	require.ErrorIs(t, err, fs.ErrPermission)
	want := fmt.Sprintf("cannot build the store in %s: %s; run quarry sync --from %s",
		homepath.Abbreviate(home, filepath.Join(home, "quarry")), fs.ErrPermission.Error(),
		snapshotIDFromPath(outcome.Manifest.Snapshot.Path))
	assert.Equal(t, want, err.Error())
}

func Test_sync_and_import_completes_normally_when_the_context_ends_after_a_successful_import(t *testing.T) {
	bundle := v9fixture.OpenBundle(t, t.TempDir())
	home := t.TempDir()
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	fake := &fakeImporter{result: store.Result{Built: true}, interrupt: true, cancel: cancel}
	srv := newImportServer(t, home, fake)

	outcome, err := srv.SyncAndImport(ctx, bundle.Dir)

	require.NoError(t, err)
	require.NotNil(t, outcome.Store)
	assert.True(t, outcome.Store.Built)
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
		snapshot.WithStoreProbe(&fakeStoreProbe{path: filepath.Join(home, "quarry", "quarry.duckdb")}),
	)

	outcome, err := srv.SyncAndImport(t.Context(), filepath.Join(home, "Home.quicken"))

	require.Error(t, err)
	var mismatch snapshot.MismatchError
	assert.False(t, errors.As(err, &mismatch))
	assert.Empty(t, fake.calls)
	assert.Nil(t, outcome.Store)
}

// Store must be the unbuilt result, not nil, so StdoutWriteRefusal picks
// the --from --json form; Path is populated on this copy regardless.
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
		name    string
		existed bool
	}{
		{name: "no previous store", existed: false},
		{name: "a previous store", existed: true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			bundle := v9fixture.OpenBundle(t, t.TempDir())
			home := t.TempDir()
			fake := &fakeImporter{result: store.Result{Built: false}, err: store.ErrValidationFailed}
			probe := &fakeStoreProbe{path: filepath.Join(home, "quarry", "quarry.duckdb"), exists: c.existed}
			srv := newImportServer(t, home, fake, snapshot.WithStoreProbe(probe))

			outcome, err := srv.SyncAndImport(t.Context(), bundle.Dir)

			require.ErrorIs(t, err, store.ErrValidationFailed)
			assert.Equal(t, c.existed, outcome.StoreExisted)
		})
	}
}

func Test_sync_and_import_names_the_store_file_its_probe_reports(t *testing.T) {
	bundle := v9fixture.OpenBundle(t, t.TempDir())
	home := t.TempDir()
	fake := &fakeImporter{result: store.Result{Built: false}, err: store.ErrValidationFailed}
	probePath := filepath.Join(home, "elsewhere", "quarry.duckdb")
	srv := newImportServer(t, home, fake, snapshot.WithStoreProbe(&fakeStoreProbe{path: probePath}))

	outcome, err := srv.SyncAndImport(t.Context(), bundle.Dir)

	require.ErrorIs(t, err, store.ErrValidationFailed)
	require.NotNil(t, outcome.Store)
	assert.Equal(t, probePath, outcome.Store.Path)
}

func Test_sync_and_import_refuses_to_import_without_an_importer_or_store_probe(t *testing.T) {
	cases := []struct {
		name string
		opts []snapshot.Option
	}{
		{name: "no importer", opts: []snapshot.Option{snapshot.WithImporter(nil)}},
		{name: "no store probe", opts: []snapshot.Option{snapshot.WithStoreProbe(nil)}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			bundle := v9fixture.OpenBundle(t, t.TempDir())
			fake := &fakeImporter{}
			srv := newImportServer(t, t.TempDir(), fake, c.opts...)

			outcome, err := srv.SyncAndImport(t.Context(), bundle.Dir)

			require.EqualError(t, err, "no importer or store probe configured")
			assert.Empty(t, fake.calls)
			assert.Nil(t, outcome.Store)
		})
	}
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
			name: "balance counts grouped by thousands",
			validation: store.Validation{Balances: store.BalanceCheck{
				Checked: 1035, Mismatched: make([]store.BalanceMismatch, 1000),
			}},
			wantClause: "1,000 of 1,035 accounts do not match Quicken's last reconciled balance",
		},
		{
			name:       "split count grouped by thousands",
			validation: store.Validation{Splits: store.SplitCheck{Mismatched: make([]store.SplitMismatch, 1204)}},
			wantClause: "1,204 transactions do not equal the sum of their splits",
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
