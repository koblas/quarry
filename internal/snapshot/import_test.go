package snapshot_test

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/finding"
	"github.com/koblas/quarry/internal/platform/homepath"
	v9 "github.com/koblas/quarry/internal/quicken/v9"
	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/koblas/quarry/internal/snapshot"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// errNoSpace stands in for a stdout write failing on a full disk.
var errNoSpace = errors.New("no space left on device")

func Test_stdout_write_refusal_points_to_from_only_once_the_build_was_reached(t *testing.T) {
	t.Parallel()
	writeErr := errNoSpace
	manifest := snapshot.Manifest{Snapshot: snapshot.Info{
		Path: "/Users/dave/Library/Application Support/quarry/snapshots/20260927T143005Z.sqlite",
	}}
	cases := []struct {
		name  string
		store *store.Result
		want  string
	}{
		{
			name:  "no build reached",
			store: nil,
			want: "cannot write the result to stdout: no space left on device; the snapshot is kept at " +
				"~/Library/Application Support/quarry/snapshots/20260927T143005Z.sqlite and its .json manifest holds the full result",
		},
		{
			name:  "build reached",
			store: &store.Result{Path: "/Users/dave/Library/Application Support/quarry/quarry.duckdb"},
			want: "cannot write the result to stdout: no space left on device; " +
				"run quarry sync --from 20260927T143005Z --json to see it again",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			outcome := snapshot.Outcome{Manifest: manifest, Store: c.store}

			got := outcome.StdoutWriteRefusal("/Users/dave", writeErr)

			assert.Equal(t, c.want, got.Error())
		})
	}
}

func Test_outcome_warnings_omit_one_sided_transfers_for_a_built_store(t *testing.T) {
	t.Parallel()
	built := &store.Result{Built: true, Validation: store.Validation{Transfers: store.TransferCheck{OneSided: make([]store.OneSidedTransfer, 2)}}}
	outcome := snapshot.Outcome{Manifest: snapshot.Manifest{Warnings: []string{"schema warning"}}, Store: built}

	got := outcome.Warnings()

	assert.Equal(t, []string{"schema warning"}, got)
}

func Test_outcome_warnings_are_the_manifests_when_no_import_ran(t *testing.T) {
	t.Parallel()
	outcome := snapshot.Outcome{Manifest: snapshot.Manifest{Warnings: []string{"schema warning"}}}

	got := outcome.Warnings()

	assert.Equal(t, []string{"schema warning"}, got)
}

func Test_id_strips_one_sqlite_extension_in_any_letter_case(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		path string
		want string
	}{
		{name: "lower case", path: "/snaps/20260927T143005Z.sqlite", want: "20260927T143005Z"},
		{name: "upper case", path: "/snaps/20260927T143005Z.SQLITE", want: "20260927T143005Z"},
		{name: "mixed case", path: "/snaps/20260927T143005Z.Sqlite", want: "20260927T143005Z"},
		{name: "the long s folds", path: "/snaps/20260927T143005Z.ſqlite", want: "20260927T143005Z"},
		{name: "another extension is kept", path: "/snaps/latest.db", want: "latest.db"},
		{name: "only the last extension goes", path: "/snaps/X.sqlite.SQLITE", want: "X.sqlite"},
		{name: "a manifest name is kept", path: "/snaps/X.json", want: "X.json"},
		{name: "no extension", path: "/snaps/X", want: "X"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.want, snapshot.ID(c.path))
		})
	}
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

func Test_sync_and_import_imports_the_committed_snapshot(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	bundle := v9fixture.OpenBundle(t, t.TempDir())
	home := t.TempDir()
	fake := &fakeImporter{}
	srv := newImportServer(t, home, fake)

	outcome, err := srv.SyncAndImport(t.Context(), bundle.Dir)

	require.NoError(t, err)
	require.Len(t, fake.calls, 1)
	assert.Equal(t, outcome.Manifest.Snapshot.Path, fake.calls[0].Path)
	assert.Equal(t, outcome.Manifest.Snapshot.SHA256, fake.calls[0].SHA256)
	assert.Equal(t, outcome.Manifest.Schema.Fingerprint, fake.calls[0].SchemaFingerprint)
	assert.NotEmpty(t, fake.calls[0].SHA256)
	assert.NotEmpty(t, fake.calls[0].SchemaFingerprint)
}

func Test_sync_and_import_passes_the_manifests_taken_at_and_source_to_the_importer(t *testing.T) {
	t.Parallel()
	bundle := v9fixture.OpenBundle(t, t.TempDir())
	home := t.TempDir()
	fake := &fakeImporter{}
	srv := newImportServer(t, home, fake)

	outcome, err := srv.SyncAndImport(t.Context(), bundle.Dir)

	require.NoError(t, err)
	require.Len(t, fake.calls, 1)
	manifestTakenAt, err := time.Parse(time.RFC3339, outcome.Manifest.Snapshot.TakenAt)
	require.NoError(t, err)
	assert.Equal(t, manifestTakenAt.UTC(), fake.calls[0].TakenAt)
	assert.NotEmpty(t, fake.calls[0].Source)
	assert.Equal(t, outcome.Manifest.Snapshot.Source, fake.calls[0].Source)
}

func Test_sync_and_import_skips_the_import_on_a_schema_mismatch(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	bundle := v9fixture.OpenBundle(t, t.TempDir())
	home := t.TempDir()
	fake := &fakeImporter{err: errImportBoom}
	srv := newImportServer(t, home, fake)

	outcome, err := srv.SyncAndImport(t.Context(), bundle.Dir)

	require.Error(t, err)
	require.ErrorIs(t, err, errImportBoom)
	assert.Nil(t, outcome.Store)
	id := snapshotIDFromPath(outcome.Manifest.Snapshot.Path)
	want := fmt.Sprintf("cannot build the store in %s: %s; run quarry sync --from %s",
		homepath.Abbreviate(home, filepath.Join(home, "quarry")), errImportBoom.Error(), id)
	assert.Equal(t, want, err.Error())
	assert.FileExists(t, outcome.Manifest.Snapshot.Path)
	assert.FileExists(t, outcome.Manifest.Snapshot.Manifest)
}

var errDuckDBDiskFull = errors.New(`IO Error: Could not write file "quarry.duckdb.partial": No space left on device`)

func Test_sync_and_import_reports_each_build_failure_with_its_refusal(t *testing.T) {
	t.Parallel()
	permission := taggedBuildError{sentinel: store.ErrStoreNotWritable, cause: &fs.PathError{
		Op: "open", Path: "quarry.duckdb.partial", Err: fs.ErrPermission,
	}}
	diskFull := taggedBuildError{sentinel: store.ErrDiskFull, cause: errDuckDBDiskFull}
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
			t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
	home := t.TempDir()
	fake := &fakeImporter{}
	ref := v9Reference(t)
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
	assert.NotErrorAs(t, err, &mismatch)
	assert.Empty(t, fake.calls)
	assert.Nil(t, outcome.Store)
}

// Store must be the unbuilt result, not nil, so StdoutWriteRefusal picks
// the --from --json form; Path is populated on this copy regardless.
func Test_sync_and_import_keeps_the_store_result_when_validation_fails(t *testing.T) {
	t.Parallel()
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

// A previous store existing or not decides the failed-validation block's NOT REBUILT vs
// NOT BUILT line.
func Test_sync_and_import_reports_whether_a_previous_store_existed(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		existed bool
	}{
		{name: "no previous store", existed: false},
		{name: "a previous store", existed: true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
	cases := []struct {
		name string
		opts []snapshot.Option
	}{
		{name: "no importer", opts: []snapshot.Option{snapshot.WithImporter(nil)}},
		{name: "no store probe", opts: []snapshot.Option{snapshot.WithStoreProbe(nil)}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
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
func Test_sync_and_import_reports_a_failed_check_in_the_validation_refusal(t *testing.T) {
	t.Parallel()
	const (
		cashTail        = "fix them in Quicken and run quarry sync, or run quarry sync --from %s after updating quarry"
		singleShareTail = "quarry read the holding's transactions differently from Quicken, so run quarry sync --from %s after updating quarry"
		pluralShareTail = "quarry read those holdings' transactions differently from Quicken, so run quarry sync --from %s after updating quarry"
	)
	cases := []struct {
		name       string
		validation store.Validation
		wantClause string
		wantTail   string
	}{
		{
			name: "one of one account mismatched",
			validation: store.Validation{Balances: store.BalanceCheck{
				Checked: 1, Mismatched: []store.BalanceMismatch{{ID: "acct-1"}},
			}},
			wantClause: "1 of 1 account does not match Quicken's last reconciled balance",
			wantTail:   cashTail,
		},
		{
			name: "one balance mismatch",
			validation: store.Validation{Balances: store.BalanceCheck{
				Checked: 3, Mismatched: []store.BalanceMismatch{{ID: "acct-1"}},
			}},
			wantClause: "1 of 3 accounts does not match Quicken's last reconciled balance",
			wantTail:   cashTail,
		},
		{
			name: "two balance mismatches",
			validation: store.Validation{Balances: store.BalanceCheck{
				Checked: 3, Mismatched: []store.BalanceMismatch{{ID: "acct-1"}, {ID: "acct-2"}},
			}},
			wantClause: "2 of 3 accounts do not match Quicken's last reconciled balance",
			wantTail:   cashTail,
		},
		{
			name: "every checked account mismatched",
			validation: store.Validation{Balances: store.BalanceCheck{
				Checked: 3, Mismatched: []store.BalanceMismatch{{ID: "acct-1"}, {ID: "acct-2"}, {ID: "acct-3"}},
			}},
			wantClause: "3 of 3 accounts do not match Quicken's last reconciled balance",
			wantTail:   cashTail,
		},
		{
			name:       "one split mismatch",
			validation: store.Validation{Splits: store.SplitCheck{Mismatched: []store.SplitMismatch{{ID: "txn-1"}}}},
			wantClause: "1 transaction does not equal the sum of its splits",
			wantTail:   cashTail,
		},
		{
			name: "two split mismatches",
			validation: store.Validation{
				Splits: store.SplitCheck{Mismatched: []store.SplitMismatch{{ID: "txn-1"}, {ID: "txn-2"}}},
			},
			wantClause: "2 transactions do not equal the sum of their splits",
			wantTail:   cashTail,
		},
		{
			name: "balance counts grouped by thousands",
			validation: store.Validation{Balances: store.BalanceCheck{
				Checked: 1035, Mismatched: make([]store.BalanceMismatch, 1000),
			}},
			wantClause: "1,000 of 1,035 accounts do not match Quicken's last reconciled balance",
			wantTail:   cashTail,
		},
		{
			name:       "split count grouped by thousands",
			validation: store.Validation{Splits: store.SplitCheck{Mismatched: make([]store.SplitMismatch, 1204)}},
			wantClause: "1,204 transactions do not equal the sum of their splits",
			wantTail:   cashTail,
		},
		{
			name: "a balance and a split mismatch joined",
			validation: store.Validation{
				Balances: store.BalanceCheck{Checked: 3, Mismatched: []store.BalanceMismatch{{ID: "acct-1"}}},
				Splits:   store.SplitCheck{Mismatched: []store.SplitMismatch{{ID: "txn-1"}}},
			},
			wantClause: "1 of 3 accounts does not match Quicken's last reconciled balance and " +
				"1 transaction does not equal the sum of its splits",
			wantTail: cashTail,
		},
		{
			name: "one share mismatch among many holdings",
			validation: store.Validation{Shares: store.ShareCheck{
				Checked: 145, Mismatched: []store.ShareMismatch{{AccountID: "acct-1"}},
			}},
			wantClause: "1 of 145 holdings does not match Quicken's share count",
			wantTail:   singleShareTail,
		},
		{
			name: "one of one holding mismatched",
			validation: store.Validation{Shares: store.ShareCheck{
				Checked: 1, Mismatched: []store.ShareMismatch{{AccountID: "acct-1"}},
			}},
			wantClause: "1 of 1 holding does not match Quicken's share count",
			wantTail:   singleShareTail,
		},
		{
			name: "several share mismatches",
			validation: store.Validation{Shares: store.ShareCheck{
				Checked: 145, Mismatched: []store.ShareMismatch{{AccountID: "acct-1"}, {AccountID: "acct-2"}},
			}},
			wantClause: "2 of 145 holdings do not match Quicken's share counts",
			wantTail:   pluralShareTail,
		},
		{
			name: "share counts grouped by thousands",
			validation: store.Validation{Shares: store.ShareCheck{
				Checked: 1035, Mismatched: make([]store.ShareMismatch, 1000),
			}},
			wantClause: "1,000 of 1,035 holdings do not match Quicken's share counts",
			wantTail:   pluralShareTail,
		},
		{
			name: "balances and shares joined",
			validation: store.Validation{
				Balances: store.BalanceCheck{Checked: 3, Mismatched: []store.BalanceMismatch{{ID: "acct-1"}}},
				Shares:   store.ShareCheck{Checked: 145, Mismatched: []store.ShareMismatch{{AccountID: "acct-1"}}},
			},
			wantClause: "1 of 3 accounts does not match Quicken's last reconciled balance and " +
				"1 of 145 holdings does not match Quicken's share count",
			wantTail: cashTail,
		},
		{
			name: "splits and shares joined",
			validation: store.Validation{
				Splits: store.SplitCheck{Mismatched: []store.SplitMismatch{{ID: "txn-1"}}},
				Shares: store.ShareCheck{Checked: 145, Mismatched: []store.ShareMismatch{{AccountID: "acct-1"}, {AccountID: "acct-2"}}},
			},
			wantClause: "1 transaction does not equal the sum of its splits and " +
				"2 of 145 holdings do not match Quicken's share counts",
			wantTail: cashTail,
		},
		{
			name: "balances, splits and shares in order",
			validation: store.Validation{
				Balances: store.BalanceCheck{Checked: 3, Mismatched: []store.BalanceMismatch{{ID: "acct-1"}}},
				Splits:   store.SplitCheck{Mismatched: []store.SplitMismatch{{ID: "txn-1"}}},
				Shares:   store.ShareCheck{Checked: 145, Mismatched: []store.ShareMismatch{{AccountID: "acct-1"}}},
			},
			wantClause: "1 of 3 accounts does not match Quicken's last reconciled balance and " +
				"1 transaction does not equal the sum of its splits and " +
				"1 of 145 holdings does not match Quicken's share count",
			wantTail: cashTail,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
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
			want := fmt.Sprintf("validation failed: %s; %s was not changed; each difference is listed on stdout; "+c.wantTail,
				c.wantClause, homepath.Abbreviate(home, storePath), snapshotIDFromPath(outcome.Manifest.Snapshot.Path))
			assert.Equal(t, want, err.Error())
		})
	}
}

func Test_sync_and_import_refuses_without_an_importer(t *testing.T) {
	t.Parallel()
	bundle := v9fixture.OpenBundle(t, t.TempDir())
	home := t.TempDir()
	srv := newImportServer(t, home, nil)

	outcome, err := srv.SyncAndImport(t.Context(), bundle.Dir)

	require.Error(t, err)
	assert.Nil(t, outcome.Store)
}

var errDriverText = errors.New("driver text")

func Test_sync_and_import_names_the_combined_reason_when_the_previous_store_cannot_be_read(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	storePath := filepath.Join(home, "quarry", "quarry.duckdb")
	cases := []struct {
		name  string
		fault *store.OpenError
		want  string
	}{
		{
			"not a DuckDB database", &store.OpenError{Fault: store.OpenFaultNotDuckDB, Path: storePath, Err: errDriverText},
			"the file is not a DuckDB database",
		},
		{
			"permission denied", &store.OpenError{Fault: store.OpenFaultPermission, Path: storePath, Err: errDriverText},
			"permission denied",
		},
		{
			"locked by another program", &store.OpenError{Fault: store.OpenFaultLocked, Path: storePath, Err: errDriverText},
			"another program has it open for writing",
		},
		{
			"other fault names the store by its display path", &store.OpenError{Fault: store.OpenFaultOther, Path: storePath, Reason: "cannot open " + storePath + ": broken"},
			"cannot open ~/quarry/quarry.duckdb: broken",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			fake := &fakeImporter{result: store.Result{Built: true, HistoryFault: c.fault, StoreUnreadable: true}}
			srv := newImportServer(t, home, fake)

			outcome, err := srv.SyncAndImport(t.Context(), v9fixture.OpenBundle(t, t.TempDir()).Dir)

			require.NoError(t, err)
			assert.Equal(t, []string{combinedCarryLine(c.want)}, outcome.Warnings())
		})
	}
}

func Test_sync_and_import_names_the_history_fault_reason_in_the_warning(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	storePath := filepath.Join(home, "quarry", "quarry.duckdb")
	reasons := []string{
		"its import_runs table repeats an id",
		"it has no import_runs table",
		"its import_runs table is incomplete",
		"its import_runs table has an id too large to follow",
	}

	for _, reason := range reasons {
		t.Run(reason, func(t *testing.T) {
			t.Parallel()
			fault := &store.OpenError{Fault: store.OpenFaultOther, Path: storePath, Reason: reason}
			srv := newImportServer(t, home, &fakeImporter{result: store.Result{Built: true, HistoryFault: fault}})

			outcome, err := srv.SyncAndImport(t.Context(), v9fixture.OpenBundle(t, t.TempDir()).Dir)

			require.NoError(t, err)
			assert.Equal(t, []string{historyRestartLine(reason)}, outcome.Warnings())
		})
	}
}

func Test_sync_and_import_names_the_findings_fault_reason_in_the_warning(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	storePath := filepath.Join(home, "quarry", "quarry.duckdb")
	reasons := []string{"its findings table repeats an id", "its findings table is incomplete"}

	for _, reason := range reasons {
		t.Run(reason, func(t *testing.T) {
			t.Parallel()
			fault := &store.OpenError{Fault: store.OpenFaultOther, Path: storePath, Reason: reason}
			srv := newImportServer(t, home, &fakeImporter{result: store.Result{Built: true, FindingsFault: fault}})

			outcome, err := srv.SyncAndImport(t.Context(), v9fixture.OpenBundle(t, t.TempDir()).Dir)

			require.NoError(t, err)
			assert.Equal(t, []string{findingsRestartLine(reason)}, outcome.Warnings())
		})
	}
}

func Test_sync_and_import_warns_of_a_findings_fault_alone_without_the_import_history_line(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	fault := &store.OpenError{Fault: store.OpenFaultOther, Path: filepath.Join(home, "quarry", "quarry.duckdb"), Reason: "its findings table is incomplete"}
	srv := newImportServer(t, home, &fakeImporter{result: store.Result{Built: true, FindingsFault: fault}})

	outcome, err := srv.SyncAndImport(t.Context(), v9fixture.OpenBundle(t, t.TempDir()).Dir)

	require.NoError(t, err)
	assert.Equal(t, []string{findingsRestartLine("its findings table is incomplete")}, outcome.Warnings())
}

func Test_sync_and_import_prints_the_history_line_then_the_findings_line_when_both_tables_are_faulty(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	storePath := filepath.Join(home, "quarry", "quarry.duckdb")
	history := &store.OpenError{Fault: store.OpenFaultOther, Path: storePath, Reason: "its import_runs table is incomplete"}
	findings := &store.OpenError{Fault: store.OpenFaultOther, Path: storePath, Reason: "its findings table repeats an id"}
	srv := newImportServer(t, home, &fakeImporter{result: store.Result{Built: true, HistoryFault: history, FindingsFault: findings}})

	outcome, err := srv.SyncAndImport(t.Context(), v9fixture.OpenBundle(t, t.TempDir()).Dir)

	require.NoError(t, err)
	assert.Equal(t, []string{
		historyRestartLine("its import_runs table is incomplete"),
		findingsRestartLine("its findings table repeats an id"),
	}, outcome.Warnings())
}

func Test_sync_and_import_warns_of_a_findings_fault_alone_when_the_store_is_flagged_unreadable(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	fault := &store.OpenError{Fault: store.OpenFaultOther, Path: filepath.Join(home, "quarry", "quarry.duckdb"), Reason: "its findings table is incomplete"}
	srv := newImportServer(t, home, &fakeImporter{result: store.Result{Built: true, StoreUnreadable: true, FindingsFault: fault}})

	outcome, err := srv.SyncAndImport(t.Context(), v9fixture.OpenBundle(t, t.TempDir()).Dir)

	require.NoError(t, err)
	assert.Equal(t, []string{findingsRestartLine("its findings table is incomplete")}, outcome.Warnings())
}

func Test_sync_and_import_names_the_rates_fault_reason_in_the_warning(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	storePath := filepath.Join(home, "quarry", "quarry.duckdb")
	reasons := []string{
		"its fx_rates table repeats a date",
		"its fx_rates table is incomplete",
		"its fx_rates table holds an impossible rate",
		"its fx_rates table names an unknown series",
	}

	for _, reason := range reasons {
		t.Run(reason, func(t *testing.T) {
			t.Parallel()
			fault := &store.OpenError{Fault: store.OpenFaultOther, Path: storePath, Reason: reason}
			srv := newImportServer(t, home, &fakeImporter{result: store.Result{Built: true, RatesFault: fault}})

			outcome, err := srv.SyncAndImport(t.Context(), v9fixture.OpenBundle(t, t.TempDir()).Dir)

			require.NoError(t, err)
			assert.Equal(t, []string{ratesRestartLine(reason)}, outcome.Warnings())
		})
	}
}

func Test_sync_and_import_prints_the_history_findings_and_rates_lines_in_that_order_when_all_three_tables_are_faulty(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	storePath := filepath.Join(home, "quarry", "quarry.duckdb")
	history := &store.OpenError{Fault: store.OpenFaultOther, Path: storePath, Reason: "its import_runs table is incomplete"}
	findings := &store.OpenError{Fault: store.OpenFaultOther, Path: storePath, Reason: "its findings table repeats an id"}
	rates := &store.OpenError{Fault: store.OpenFaultOther, Path: storePath, Reason: "its fx_rates table repeats a date"}
	srv := newImportServer(t, home, &fakeImporter{result: store.Result{Built: true, HistoryFault: history, FindingsFault: findings, RatesFault: rates}})

	outcome, err := srv.SyncAndImport(t.Context(), v9fixture.OpenBundle(t, t.TempDir()).Dir)

	require.NoError(t, err)
	assert.Equal(t, []string{
		historyRestartLine("its import_runs table is incomplete"),
		findingsRestartLine("its findings table repeats an id"),
		ratesRestartLine("its fx_rates table repeats a date"),
	}, outcome.Warnings())
}

func Test_sync_and_import_warns_of_a_rates_fault_alone_when_the_store_is_flagged_unreadable(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	fault := &store.OpenError{Fault: store.OpenFaultOther, Path: filepath.Join(home, "quarry", "quarry.duckdb"), Reason: "its fx_rates table is incomplete"}
	srv := newImportServer(t, home, &fakeImporter{result: store.Result{Built: true, StoreUnreadable: true, RatesFault: fault}})

	outcome, err := srv.SyncAndImport(t.Context(), v9fixture.OpenBundle(t, t.TempDir()).Dir)

	require.NoError(t, err)
	assert.Equal(t, []string{ratesRestartLine("its fx_rates table is incomplete")}, outcome.Warnings())
}

func Test_sync_and_import_prints_the_combined_line_alone_when_the_unreadable_store_also_carries_a_rates_fault(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	storePath := filepath.Join(home, "quarry", "quarry.duckdb")
	history := &store.OpenError{Fault: store.OpenFaultNotDuckDB, Path: storePath}
	rates := &store.OpenError{Fault: store.OpenFaultOther, Path: storePath, Reason: "its fx_rates table is incomplete"}
	srv := newImportServer(t, home, &fakeImporter{result: store.Result{Built: true, StoreUnreadable: true, HistoryFault: history, RatesFault: rates}})

	outcome, err := srv.SyncAndImport(t.Context(), v9fixture.OpenBundle(t, t.TempDir()).Dir)

	require.NoError(t, err)
	assert.Equal(t, []string{combinedCarryLine("the file is not a DuckDB database")}, outcome.Warnings())
}

func Test_outcome_adds_no_carry_warning_for_a_store_that_was_not_built(t *testing.T) {
	t.Parallel()
	fault := &store.OpenError{Fault: store.OpenFaultNotDuckDB, Path: "quarry.duckdb"}
	unbuilt := snapshot.Outcome{Store: &store.Result{HistoryFault: fault, FindingsFault: fault, RatesFault: fault, StoreUnreadable: true}}

	assert.Empty(t, unbuilt.Warnings())
}

func Test_sync_and_import_warns_of_nothing_when_the_history_was_carried(t *testing.T) {
	t.Parallel()
	fake := &fakeImporter{result: store.Result{Built: true}}
	srv := newImportServer(t, t.TempDir(), fake)

	outcome, err := srv.SyncAndImport(t.Context(), v9fixture.OpenBundle(t, t.TempDir()).Dir)

	require.NoError(t, err)
	assert.Empty(t, outcome.Warnings())
}

func Test_import_from_puts_the_history_warning_after_the_manifest_warning(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	ref := v9Reference(t)
	delete(ref, "ZALERT")
	fault := &store.OpenError{Fault: store.OpenFaultNotDuckDB, Path: filepath.Join(home, "quarry", "quarry.duckdb")}
	srv := newImportServer(t, home, &fakeImporter{result: store.Result{Built: true, HistoryFault: fault, StoreUnreadable: true}},
		snapshot.WithReference(v9.ReferenceLabel, ref))
	taken := takeSnapshot(t, srv)
	require.Len(t, taken.Warnings, 1)

	outcome, err := srv.ImportFrom(t.Context(), snapshotIDFromPath(taken.Snapshot.Path))

	require.NoError(t, err)
	assert.Equal(t, []string{taken.Warnings[0], combinedCarryLine("the file is not a DuckDB database")}, outcome.Warnings())
}

func Test_import_from_puts_the_findings_warning_after_the_manifest_warning(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	ref := v9Reference(t)
	delete(ref, "ZALERT")
	fault := &store.OpenError{Fault: store.OpenFaultOther, Path: filepath.Join(home, "quarry", "quarry.duckdb"), Reason: "its findings table repeats an id"}
	srv := newImportServer(t, home, &fakeImporter{result: store.Result{Built: true, FindingsFault: fault}},
		snapshot.WithReference(v9.ReferenceLabel, ref))
	taken := takeSnapshot(t, srv)
	require.Len(t, taken.Warnings, 1)

	outcome, err := srv.ImportFrom(t.Context(), snapshotIDFromPath(taken.Snapshot.Path))

	require.NoError(t, err)
	assert.Equal(t, []string{taken.Warnings[0], findingsRestartLine("its findings table repeats an id")}, outcome.Warnings())
}

func Test_import_from_puts_the_rates_warning_after_the_manifest_warning(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	ref := v9Reference(t)
	delete(ref, "ZALERT")
	fault := &store.OpenError{Fault: store.OpenFaultOther, Path: filepath.Join(home, "quarry", "quarry.duckdb"), Reason: "its fx_rates table names an unknown series"}
	srv := newImportServer(t, home, &fakeImporter{result: store.Result{Built: true, RatesFault: fault}},
		snapshot.WithReference(v9.ReferenceLabel, ref))
	taken := takeSnapshot(t, srv)
	require.Len(t, taken.Warnings, 1)

	outcome, err := srv.ImportFrom(t.Context(), snapshotIDFromPath(taken.Snapshot.Path))

	require.NoError(t, err)
	assert.Equal(t, []string{taken.Warnings[0], ratesRestartLine("its fx_rates table names an unknown series")}, outcome.Warnings())
}

// threeStates is two new findings and one carried, as a store build reports them with no ignore list.
func threeStates() store.Result {
	return store.Result{
		Built:    true,
		Findings: finding.Counts{Open: 3, New: 2},
		FindingStates: []finding.State{
			{ID: "uncategorized:payee-1", New: true},
			{ID: "uncategorized:payee-2", New: true},
			{ID: "uncategorized:payee-3"},
		},
	}
}

func Test_sync_and_import_counts_an_ignored_open_finding_as_ignored_not_open_or_new(t *testing.T) {
	t.Parallel()
	srv := newImportServer(t, t.TempDir(), &fakeImporter{result: threeStates()}, snapshot.WithIgnore([]string{"uncategorized:payee-1"}))

	outcome, err := srv.SyncAndImport(t.Context(), v9fixture.OpenBundle(t, t.TempDir()).Dir)

	require.NoError(t, err)
	assert.Equal(t, finding.Counts{Open: 2, Ignored: 1, New: 1}, outcome.Store.Findings)
}

func Test_sync_and_import_counts_every_finding_as_open_without_an_ignore_list(t *testing.T) {
	t.Parallel()
	srv := newImportServer(t, t.TempDir(), &fakeImporter{result: threeStates()})

	outcome, err := srv.SyncAndImport(t.Context(), v9fixture.OpenBundle(t, t.TempDir()).Dir)

	require.NoError(t, err)
	assert.Equal(t, finding.Counts{Open: 3, New: 2}, outcome.Store.Findings)
}

const unclassifiedID = "unclassified-account:acct-1"

// threeStatesWithAccount is threeStates with the one account a build wrote.
func threeStatesWithAccount() store.Result {
	result := threeStates()
	result.Accounts = []store.Account{{ID: "acct-1", Type: store.AccountTypeBrokerage}}
	return result
}

// readTimeStates is a read-time function computing one unclassified finding for each account it is given.
func readTimeStates(list store.FindingList) []finding.State {
	states := make([]finding.State, 0, len(list.Accounts))
	for _, a := range list.Accounts {
		states = append(states, finding.State{ID: "unclassified-account:" + a.ID})
	}
	return states
}

func Test_sync_and_import_counts_a_read_time_finding_open_and_never_new(t *testing.T) {
	t.Parallel()
	srv := newImportServer(t, t.TempDir(), &fakeImporter{result: threeStatesWithAccount()}, snapshot.WithReadTimeFindings(readTimeStates))

	outcome, err := srv.SyncAndImport(t.Context(), v9fixture.OpenBundle(t, t.TempDir()).Dir)

	require.NoError(t, err)
	assert.Equal(t, finding.Counts{Open: 4, New: 2}, outcome.Store.Findings)
}

func Test_sync_and_import_counts_the_same_without_a_read_time_function_whatever_accounts_the_build_wrote(t *testing.T) {
	t.Parallel()
	srv := newImportServer(t, t.TempDir(), &fakeImporter{result: threeStatesWithAccount()})

	outcome, err := srv.SyncAndImport(t.Context(), v9fixture.OpenBundle(t, t.TempDir()).Dir)

	require.NoError(t, err)
	assert.Equal(t, finding.Counts{Open: 3, New: 2}, outcome.Store.Findings)
}

func Test_sync_and_import_counts_an_ignored_read_time_finding_as_ignored(t *testing.T) {
	t.Parallel()
	srv := newImportServer(t, t.TempDir(), &fakeImporter{result: threeStatesWithAccount()},
		snapshot.WithReadTimeFindings(readTimeStates), snapshot.WithIgnore([]string{unclassifiedID}))

	outcome, err := srv.SyncAndImport(t.Context(), v9fixture.OpenBundle(t, t.TempDir()).Dir)

	require.NoError(t, err)
	assert.Equal(t, finding.Counts{Open: 3, Ignored: 1, New: 2}, outcome.Store.Findings)
}

func Test_import_from_counts_a_read_time_finding_open_and_never_new(t *testing.T) {
	t.Parallel()
	srv := newImportServer(t, t.TempDir(), &fakeImporter{result: threeStatesWithAccount()}, snapshot.WithReadTimeFindings(readTimeStates))
	manifest := takeSnapshot(t, srv)

	outcome, err := srv.ImportFrom(t.Context(), snapshotIDFromPath(manifest.Snapshot.Path))

	require.NoError(t, err)
	assert.Equal(t, finding.Counts{Open: 4, New: 2}, outcome.Store.Findings)
}

func Test_import_from_counts_an_ignored_open_finding_as_ignored_not_open_or_new(t *testing.T) {
	t.Parallel()
	srv := newImportServer(t, t.TempDir(), &fakeImporter{result: threeStates()}, snapshot.WithIgnore([]string{"uncategorized:payee-1"}))
	manifest := takeSnapshot(t, srv)

	outcome, err := srv.ImportFrom(t.Context(), snapshotIDFromPath(manifest.Snapshot.Path))

	require.NoError(t, err)
	assert.Equal(t, finding.Counts{Open: 2, Ignored: 1, New: 1}, outcome.Store.Findings)
}

func Test_sync_and_import_hands_the_read_time_function_the_investments_the_build_wrote(t *testing.T) {
	t.Parallel()
	result := threeStatesWithAccount()
	result.Investments = store.Investments{
		Securities:   []store.Security{{ID: "sec-1", Name: "Acme Corp"}},
		Transactions: []store.InvestmentTransaction{{ID: "itxn-1", AccountID: "acct-1", SecurityID: new("sec-1"), Action: store.ActionAddShares}},
	}
	var got store.FindingList
	srv := newImportServer(t, t.TempDir(), &fakeImporter{result: result},
		snapshot.WithReadTimeFindings(func(list store.FindingList) []finding.State { got = list; return nil }))

	_, err := srv.SyncAndImport(t.Context(), v9fixture.OpenBundle(t, t.TempDir()).Dir)

	require.NoError(t, err)
	assert.Equal(t, result.Investments, got.Investments)
}

const fetchReason = "www.bankofcanada.ca answered 503 Service Unavailable"

var (
	firstRate = time.Date(2017, time.January, 3, 0, 0, 0, 0, time.UTC)
	lastRate  = time.Date(2017, time.January, 4, 0, 0, 0, 0, time.UTC)
)

func nothingStoredLine(reason string) string {
	return "could not fetch exchange rates from the Bank of Canada: " + reason +
		"; the store has no rates, so reports list amounts in each account's own currency; run quarry sync again to retry"
}

func nothingNewLine(reason string) string {
	return "could not fetch exchange rates from the Bank of Canada: " + reason +
		"; the store has rates from 2017-01-03 to 2017-01-04, and later dates convert at the 2017-01-04 rate; run quarry sync again to retry"
}

func partialFetchLine(reason string) string {
	return "could not fetch every exchange rate from the Bank of Canada: " + reason +
		"; the store has rates from 2017-01-03 to 2017-01-04, and later dates convert at the 2017-01-04 rate; run quarry sync again to fetch the rest"
}

func fetchFailed() store.RatesSummary {
	return store.RatesSummary{FetchError: fetchReason}
}

func Test_sync_and_import_names_the_failed_rate_fetch_in_a_warning(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		rates store.RatesSummary
		want  string
	}{
		{"nothing stored", fetchFailed(), nothingStoredLine(fetchReason)},
		{
			"nothing new, rates stored",
			store.RatesSummary{First: firstRate, Last: lastRate, FetchError: fetchReason},
			nothingNewLine(fetchReason),
		},
		{
			"some rates fetched, the rest not",
			store.RatesSummary{First: firstRate, Last: lastRate, Added: 2, FetchError: fetchReason, Partial: true},
			partialFetchLine(fetchReason),
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			srv := newImportServer(t, t.TempDir(), &fakeImporter{result: store.Result{Built: true, Rates: c.rates}})

			outcome, err := syncBundle(t, srv)

			require.NoError(t, err)
			assert.Equal(t, []string{c.want}, outcome.Warnings())
			assert.Equal(t, []string{c.want}, outcome.WarningsAbsolute())
		})
	}
}

func Test_sync_and_import_names_the_fetch_reason_it_was_given(t *testing.T) {
	t.Parallel()
	rates := store.RatesSummary{FetchError: "cannot reach www.bankofcanada.ca"}
	srv := newImportServer(t, t.TempDir(), &fakeImporter{result: store.Result{Built: true, Rates: rates}})

	outcome, err := syncBundle(t, srv)

	require.NoError(t, err)
	assert.Equal(t, []string{nothingStoredLine("cannot reach www.bankofcanada.ca")}, outcome.Warnings())
}

func Test_sync_and_import_adds_no_fetch_warning_when_the_rate_fetch_succeeded(t *testing.T) {
	t.Parallel()
	rates := store.RatesSummary{First: firstRate, Last: lastRate, Added: 2}
	srv := newImportServer(t, t.TempDir(), &fakeImporter{result: store.Result{Built: true, Rates: rates}})

	outcome, err := syncBundle(t, srv)

	require.NoError(t, err)
	assert.Empty(t, outcome.Warnings())
}

func Test_outcome_adds_no_fetch_warning_for_a_store_that_was_not_built(t *testing.T) {
	t.Parallel()
	srv := newImportServer(t, t.TempDir(), &fakeImporter{result: store.Result{Rates: fetchFailed()}})

	outcome, err := syncBundle(t, srv)

	require.NoError(t, err)
	assert.Empty(t, outcome.Warnings())
}

func Test_outcome_lists_the_fetch_warning_before_the_prune_warning(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	ids := oldIDs(2)
	prunable(t, home, ids...)
	srv := newImportServer(t, home, &fakeImporter{result: store.Result{Built: true, Rates: fetchFailed()}},
		snapshot.WithAutoPrune(1), snapshot.WithRemove(failingRemover(syscall.EACCES, ids[0]+".sqlite").remove))

	outcome, err := syncBundle(t, srv)

	require.NoError(t, err)
	want := []string{nothingStoredLine(fetchReason), deleteFailureLine(ids[0], "permission denied")}
	assert.Equal(t, want, outcome.Warnings())
	assert.Equal(t, want, outcome.WarningsAbsolute())
}

func Test_outcome_lists_the_fetch_warning_after_the_rates_carry_warning(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	storePath := filepath.Join(home, "quarry", "quarry.duckdb")
	result := store.Result{
		Built:      true,
		RatesFault: &store.OpenError{Fault: store.OpenFaultOther, Path: storePath, Reason: "its fx_rates table repeats a date"},
		Rates:      fetchFailed(),
	}
	srv := newImportServer(t, home, &fakeImporter{result: result})

	outcome, err := syncBundle(t, srv)

	require.NoError(t, err)
	want := []string{ratesRestartLine("its fx_rates table repeats a date"), nothingStoredLine(fetchReason)}
	assert.Equal(t, want, outcome.Warnings())
	assert.Equal(t, want, outcome.WarningsAbsolute())
}

func Test_outcome_lists_the_fetch_warning_after_the_combined_carry_warning(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	storePath := filepath.Join(home, "quarry", "quarry.duckdb")
	result := store.Result{
		Built:           true,
		StoreUnreadable: true,
		HistoryFault:    &store.OpenError{Fault: store.OpenFaultNotDuckDB, Path: storePath},
		Rates:           fetchFailed(),
	}
	srv := newImportServer(t, home, &fakeImporter{result: result})

	outcome, err := syncBundle(t, srv)

	require.NoError(t, err)
	want := []string{combinedCarryLine("the file is not a DuckDB database"), nothingStoredLine(fetchReason)}
	assert.Equal(t, want, outcome.Warnings())
	assert.Equal(t, want, outcome.WarningsAbsolute())
}

func Test_outcome_lists_every_warning_in_the_ruled_order(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	ids := oldIDs(2)
	prunable(t, home, ids...)
	storePath := filepath.Join(home, "quarry", "quarry.duckdb")
	result := store.Result{
		Built:         true,
		HistoryFault:  &store.OpenError{Fault: store.OpenFaultOther, Path: storePath, Reason: "its import_runs table is incomplete"},
		FindingsFault: &store.OpenError{Fault: store.OpenFaultOther, Path: storePath, Reason: "its findings table repeats an id"},
		RatesFault:    &store.OpenError{Fault: store.OpenFaultOther, Path: storePath, Reason: "its fx_rates table repeats a date"},
		Rates:         store.RatesSummary{First: firstRate, Last: lastRate, Added: 2, FetchError: fetchReason, Partial: true},
	}
	srv := newImportServer(t, home, &fakeImporter{result: result},
		snapshot.WithAutoPrune(1), snapshot.WithRemove(failingRemover(syscall.EACCES, ids[0]+".sqlite").remove))

	outcome, err := srv.SyncAndImport(t.Context(), v9fixture.ExtraSchemaBundle(t, t.TempDir()).Dir)

	require.NoError(t, err)
	require.Len(t, outcome.Manifest.Warnings, 1)
	want := []string{
		outcome.Manifest.Warnings[0],
		historyRestartLine("its import_runs table is incomplete"),
		findingsRestartLine("its findings table repeats an id"),
		ratesRestartLine("its fx_rates table repeats a date"),
		partialFetchLine(fetchReason),
		deleteFailureLine(ids[0], "permission denied"),
	}
	assert.Equal(t, want, outcome.Warnings())
	assert.Equal(t, want, outcome.WarningsAbsolute())
}
