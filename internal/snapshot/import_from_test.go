package snapshot_test

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/platform/homepath"
	"github.com/koblas/quarry/internal/platform/sqlite"
	v9 "github.com/koblas/quarry/internal/quicken/v9"
	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/koblas/quarry/internal/snapshot"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// takeSnapshot commits a snapshot of a fresh fixture bundle into srv's snapshots directory.
func takeSnapshot(t *testing.T, srv *snapshot.Server) snapshot.Manifest {
	t.Helper()
	bundle := v9fixture.OpenBundle(t, t.TempDir())
	manifest, err := srv.Sync(t.Context(), bundle.Dir)
	require.NoError(t, err)
	return manifest
}

// editManifest rewrites the manifest at path after applying edit to its decoded form.
func editManifest(t *testing.T, path string, edit func(*snapshot.Manifest)) {
	t.Helper()
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	var m snapshot.Manifest
	require.NoError(t, json.Unmarshal(raw, &m))
	edit(&m)
	data, err := m.Encode()
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0o600))
}

// writeSnapshotPair creates a SQLite file <id>.sqlite in dir by running ddl,
// plus an <id>.json manifest recording the file's real SHA-256.
func writeSnapshotPair(t *testing.T, dir, id string, ddl ...string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o700))
	path := filepath.Join(dir, id+".sqlite")
	conn, err := sql.Open("sqlite3", path)
	require.NoError(t, err)
	for _, stmt := range ddl {
		_, err := conn.ExecContext(t.Context(), stmt)
		require.NoError(t, err)
	}
	require.NoError(t, conn.Close())
	writeManifestFor(t, path)
}

// writeManifestFor writes the .json manifest next to snapshotPath, recording its real SHA-256.
func writeManifestFor(t *testing.T, snapshotPath string) {
	t.Helper()
	raw, err := os.ReadFile(snapshotPath)
	require.NoError(t, err)
	sum := sha256.Sum256(raw)
	data, err := snapshot.Manifest{Snapshot: snapshot.Info{
		Source: "/Users/x/Documents/Home.quicken", SHA256: hex.EncodeToString(sum[:]),
	}}.Encode()
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(strings.TrimSuffix(snapshotPath, ".sqlite")+".json", data, 0o600))
}

func Test_import_from_passes_the_same_snapshot_ref_as_a_plain_sync(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	fake := &fakeImporter{}
	srv := newImportServer(t, home, fake)
	bundle := v9fixture.OpenBundle(t, t.TempDir())
	synced, err := srv.SyncAndImport(t.Context(), bundle.Dir)
	require.NoError(t, err)
	raw, err := os.ReadFile(synced.Manifest.Snapshot.Path)
	require.NoError(t, err)
	sum := sha256.Sum256(raw)

	_, err = srv.ImportFrom(t.Context(), snapshotIDFromPath(synced.Manifest.Snapshot.Path))

	require.NoError(t, err)
	require.Len(t, fake.calls, 2)
	assert.Equal(t, fake.calls[0], fake.calls[1])
	assert.Equal(t, hex.EncodeToString(sum[:]), fake.calls[1].SHA256)
}

func Test_import_from_passes_the_recomputed_schema_fingerprint_not_the_recorded_one(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	fake := &fakeImporter{}
	srv := newImportServer(t, home, fake)
	bundle := v9fixture.OpenBundle(t, t.TempDir())
	synced, err := srv.SyncAndImport(t.Context(), bundle.Dir)
	require.NoError(t, err)
	const edited = "edited-fingerprint"
	editManifest(t, synced.Manifest.Snapshot.Manifest, func(m *snapshot.Manifest) { m.Schema.Fingerprint = edited })

	_, err = srv.ImportFrom(t.Context(), snapshotIDFromPath(synced.Manifest.Snapshot.Path))

	require.NoError(t, err)
	require.Len(t, fake.calls, 2)
	assert.Equal(t, synced.Manifest.Schema.Fingerprint, fake.calls[1].SchemaFingerprint)
	assert.NotEqual(t, edited, fake.calls[1].SchemaFingerprint)
}

func Test_import_from_never_rewrites_a_manifest_that_differs_from_the_recomputed_one(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	srv := newImportServer(t, home, &fakeImporter{})
	taken := takeSnapshot(t, srv)
	editManifest(t, taken.Snapshot.Manifest, func(m *snapshot.Manifest) {
		m.Schema.Verified = false
		m.Schema.MissingTables = []string{"ZALERT"}
	})
	before, err := os.ReadFile(taken.Snapshot.Manifest)
	require.NoError(t, err)

	_, err = srv.ImportFrom(t.Context(), snapshotIDFromPath(taken.Snapshot.Path))

	require.NoError(t, err)
	after, err := os.ReadFile(taken.Snapshot.Manifest)
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after))
}

func Test_import_from_a_path_outside_the_snapshots_directory_never_creates_it(t *testing.T) {
	t.Parallel()
	elsewhere := t.TempDir()
	fake := &fakeImporter{}
	ref, err := v9.Reference(t.Context())
	require.NoError(t, err)
	taken := takeSnapshot(t, snapshot.NewServer(
		snapshot.WithSnapshotDir(elsewhere), snapshot.WithReference(v9.ReferenceLabel, ref)))
	home := t.TempDir()
	srv := newImportServer(t, home, fake)

	_, err = srv.ImportFrom(t.Context(), taken.Snapshot.Path)

	require.NoError(t, err)
	require.Len(t, fake.calls, 1)
	assert.Equal(t, taken.Snapshot.Path, fake.calls[0].Path)
	assert.NoDirExists(t, filepath.Join(home, "snapshots"))
}

const unresolvableFolderRefusal = "cannot resolve x.sqlite against the current folder: %s; pass --from an absolute or ~/ path instead"

func Test_import_from_refuses_a_relative_path_when_the_working_directory_no_longer_exists(t *testing.T) {
	deletedDir := filepath.Join(t.TempDir(), "deleted")
	require.NoError(t, os.Mkdir(deletedDir, 0o700))
	t.Chdir(deletedDir)
	require.NoError(t, os.Remove(deletedDir))
	if _, err := os.Getwd(); err == nil {
		t.Skip("os.Getwd resolved despite the working directory being removed on this platform")
	}
	fake := &fakeImporter{}
	srv := newImportServer(t, t.TempDir(), fake)

	_, err := srv.ImportFrom(t.Context(), "x.sqlite")

	require.ErrorIs(t, err, fs.ErrNotExist)
	require.EqualError(t, err, fmt.Sprintf(unresolvableFolderRefusal, "no such file or directory"))
	assert.Empty(t, fake.calls)
}

func Test_import_from_refuses_a_relative_path_when_the_working_directory_cannot_be_searched(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory modes")
	}
	locked := t.TempDir()
	t.Chdir(locked)
	require.NoError(t, os.Chmod(locked, 0o000))
	t.Cleanup(func() { _ = os.Chmod(locked, 0o700) })
	fake := &fakeImporter{}
	srv := newImportServer(t, t.TempDir(), fake)

	_, err := srv.ImportFrom(t.Context(), "x.sqlite")

	require.ErrorIs(t, err, fs.ErrPermission)
	require.EqualError(t, err, fmt.Sprintf(unresolvableFolderRefusal, "permission denied"))
	assert.Empty(t, fake.calls)
}

func Test_import_from_an_absolute_or_home_path_succeeds_when_the_working_directory_cannot_be_searched(t *testing.T) {
	cases := []struct {
		name string
		from func(home, snapshotPath string) string
	}{
		{name: "an absolute path", from: func(_, snapshotPath string) string { return snapshotPath }},
		{name: "a ~/ path", from: homepath.Abbreviate},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if os.Geteuid() == 0 {
				t.Skip("root ignores directory modes")
			}
			home := t.TempDir()
			fake := &fakeImporter{}
			srv := newImportServer(t, home, fake)
			taken := takeSnapshot(t, srv)
			locked := t.TempDir()
			t.Chdir(locked)
			require.NoError(t, os.Chmod(locked, 0o000))
			t.Cleanup(func() { _ = os.Chmod(locked, 0o700) })

			_, err := srv.ImportFrom(t.Context(), c.from(home, taken.Snapshot.Path))

			require.NoError(t, err)
			require.Len(t, fake.calls, 1)
			assert.Equal(t, taken.Snapshot.Path, fake.calls[0].Path)
		})
	}
}

func Test_import_from_returns_the_manifest_a_plain_sync_returned(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	srv := newImportServer(t, home, &fakeImporter{})
	bundle := v9fixture.OpenBundle(t, t.TempDir())
	synced, err := srv.SyncAndImport(t.Context(), bundle.Dir)
	require.NoError(t, err)

	outcome, err := srv.ImportFrom(t.Context(), snapshotIDFromPath(synced.Manifest.Snapshot.Path))

	require.NoError(t, err)
	assert.Equal(t, synced.Manifest, outcome.Manifest)
}

func Test_import_from_returns_the_taken_at_time_the_manifest_recorded(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	srv := newImportServer(t, home, &fakeImporter{})
	taken := takeSnapshot(t, srv)
	const recordedTakenAt = "2001-02-03T04:05:06Z"
	editManifest(t, taken.Snapshot.Manifest, func(m *snapshot.Manifest) { m.Snapshot.TakenAt = recordedTakenAt })

	outcome, err := srv.ImportFrom(t.Context(), snapshotIDFromPath(taken.Snapshot.Path))

	require.NoError(t, err)
	assert.Equal(t, recordedTakenAt, outcome.Manifest.Snapshot.TakenAt)
}

func Test_import_from_passes_the_recorded_taken_at_and_source_to_the_importer(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	fake := &fakeImporter{}
	srv := newImportServer(t, home, fake)
	taken := takeSnapshot(t, srv)
	editManifest(t, taken.Snapshot.Manifest, func(m *snapshot.Manifest) {
		m.Snapshot.TakenAt = "2001-02-03T04:05:06-05:00"
		m.Snapshot.Source = "/Users/alex/Documents/Old.quicken"
	})

	_, err := srv.ImportFrom(t.Context(), snapshotIDFromPath(taken.Snapshot.Path))

	require.NoError(t, err)
	require.Len(t, fake.calls, 1)
	assert.Equal(t, time.Date(2001, 2, 3, 9, 5, 6, 0, time.UTC), fake.calls[0].TakenAt)
	assert.Equal(t, time.UTC, fake.calls[0].TakenAt.Location())
	assert.Equal(t, "/Users/alex/Documents/Old.quicken", fake.calls[0].Source)
}

func Test_import_from_imports_with_a_zero_taken_at_when_the_manifest_time_is_unparseable(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	fake := &fakeImporter{}
	srv := newImportServer(t, home, fake)
	taken := takeSnapshot(t, srv)
	editManifest(t, taken.Snapshot.Manifest, func(m *snapshot.Manifest) { m.Snapshot.TakenAt = "yesterday-ish" })

	_, err := srv.ImportFrom(t.Context(), snapshotIDFromPath(taken.Snapshot.Path))

	require.NoError(t, err)
	require.Len(t, fake.calls, 1)
	assert.True(t, fake.calls[0].TakenAt.IsZero())
}

func Test_import_from_imports_a_snapshot_whose_manifest_said_unverified_once_the_current_reference_matches(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	fake := &fakeImporter{}
	srv := newImportServer(t, home, fake)
	taken := takeSnapshot(t, srv)
	editManifest(t, taken.Snapshot.Manifest, func(m *snapshot.Manifest) {
		m.Schema.Verified = false
		m.Schema.MissingTables = []string{"ZALERT"}
	})

	outcome, err := srv.ImportFrom(t.Context(), snapshotIDFromPath(taken.Snapshot.Path))

	require.NoError(t, err)
	assert.Len(t, fake.calls, 1)
	assert.True(t, outcome.Manifest.Schema.Verified)
	assert.Empty(t, outcome.Manifest.Schema.MissingTables)
}

func Test_import_from_reports_a_failed_validation_like_a_plain_sync(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	fake := &fakeImporter{}
	srv := newImportServer(t, home, fake)
	taken := takeSnapshot(t, srv)
	fake.err = store.ErrValidationFailed

	outcome, err := srv.ImportFrom(t.Context(), snapshotIDFromPath(taken.Snapshot.Path))

	require.ErrorIs(t, err, store.ErrValidationFailed)
	require.NotNil(t, outcome.Store)
	assert.False(t, outcome.Store.Built)
	assert.Equal(t, filepath.Join(home, "quarry", "quarry.duckdb"), outcome.Store.Path)
}

func Test_import_from_never_imports_a_snapshot_whose_hash_changed(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	fake := &fakeImporter{}
	srv := newImportServer(t, home, fake)
	taken := takeSnapshot(t, srv)
	editManifest(t, taken.Snapshot.Manifest, func(m *snapshot.Manifest) {
		m.Snapshot.SHA256 = "0000000000000000000000000000000000000000000000000000000000000000"
	})

	outcome, err := srv.ImportFrom(t.Context(), snapshotIDFromPath(taken.Snapshot.Path))

	require.EqualError(t, err, "~/snapshots/"+filepath.Base(taken.Snapshot.Path)+
		" has changed since quarry took it (its SHA-256 does not match its manifest); take a new snapshot with quarry sync")
	assert.Empty(t, fake.calls)
	assert.Nil(t, outcome.Store)
}

func Test_import_from_reports_a_changed_snapshot_that_no_longer_opens_as_changed(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	fake := &fakeImporter{}
	srv := newImportServer(t, home, fake)
	taken := takeSnapshot(t, srv)
	f, err := os.OpenFile(taken.Snapshot.Path, os.O_WRONLY, 0)
	require.NoError(t, err)
	_, err = f.WriteAt([]byte("not a sqlite db!"), 0)
	require.NoError(t, err)
	require.NoError(t, f.Close())

	_, err = srv.ImportFrom(t.Context(), snapshotIDFromPath(taken.Snapshot.Path))

	require.EqualError(t, err, "~/snapshots/"+filepath.Base(taken.Snapshot.Path)+
		" has changed since quarry took it (its SHA-256 does not match its manifest); take a new snapshot with quarry sync")
	assert.Empty(t, fake.calls)
}

func Test_import_from_skips_the_import_when_the_current_reference_no_longer_matches(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	fake := &fakeImporter{}
	taken := takeSnapshot(t, newImportServer(t, home, fake))
	ref, err := v9.Reference(t.Context())
	require.NoError(t, err)
	ref["ZQUARRYNEWTABLE"] = []string{"Z_PK"}
	srv := snapshot.NewServer(
		snapshot.WithSnapshotDir(filepath.Join(home, "snapshots")),
		snapshot.WithReference(v9.ReferenceLabel, ref),
		snapshot.WithHome(home),
		snapshot.WithImporter(fake),
	)
	id := snapshotIDFromPath(taken.Snapshot.Path)

	outcome, err := srv.ImportFrom(t.Context(), id)

	var mismatch snapshot.MismatchError
	require.ErrorAs(t, err, &mismatch)
	assert.Equal(t, "schema check failed: snapshot "+id+" of Home.quicken is missing 1 table that the schema reference expects; "+
		"quarry cannot import it until its schema reference is updated", err.Error())
	assert.Empty(t, fake.calls)
	assert.Nil(t, outcome.Store)
	assert.Equal(t, []string{"ZQUARRYNEWTABLE"}, outcome.Manifest.Schema.MissingTables)
}

// No manifest file exists in any of this group's fixtures: that absence
// proves ImportFrom's path pre-check runs before any manifest read.
func Test_import_from_refuses_a_path_form_value_that_does_not_exist(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	fake := &fakeImporter{}
	srv := newImportServer(t, home, fake)
	snapshotPath := filepath.Join(home, "elsewhere", "20260927T143005Z.sqlite")

	_, err := srv.ImportFrom(t.Context(), snapshotPath)

	require.EqualError(t, err, homepath.Abbreviate(home, snapshotPath)+
		" does not exist; check the path passed to --from")
	assert.Empty(t, fake.calls)
}

// os.Stat needs no read permission on its target, only execute on its
// ancestor directories, so only a chmod'd directory reaches this branch.
func Test_import_from_a_path_names_the_snapshot_when_its_directory_cannot_be_searched(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		t.Skip("root ignores file permissions")
	}
	home := t.TempDir()
	fake := &fakeImporter{}
	srv := newImportServer(t, home, fake)
	taken := takeSnapshot(t, srv)
	snapshotsDir := filepath.Dir(taken.Snapshot.Path)
	require.NoError(t, os.Chmod(snapshotsDir, 0o000))
	t.Cleanup(func() { _ = os.Chmod(snapshotsDir, 0o700) })

	_, err := srv.ImportFrom(t.Context(), taken.Snapshot.Path)

	require.EqualError(t, err, "cannot read "+homepath.Abbreviate(home, taken.Snapshot.Path)+
		": permission denied; check the file's permissions")
	assert.Empty(t, fake.calls)
}

func Test_import_from_refuses_an_id_form_value_with_no_matching_snapshot(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	fake := &fakeImporter{}
	srv := newImportServer(t, home, fake)

	_, err := srv.ImportFrom(t.Context(), "20260927T143005Z")

	require.EqualError(t, err, "no snapshot 20260927T143005Z in ~/snapshots; run quarry snapshots to list the ones kept")
	assert.Empty(t, fake.calls)
}

func Test_import_from_refuses_a_path_form_directory_that_is_not_a_quicken_bundle(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	fake := &fakeImporter{}
	srv := newImportServer(t, home, fake)
	snapshotPath := filepath.Join(home, "snapshots", "20260927T143005Z.sqlite")
	require.NoError(t, os.MkdirAll(snapshotPath, 0o700))

	_, err := srv.ImportFrom(t.Context(), snapshotPath)

	require.EqualError(t, err, "~/snapshots/20260927T143005Z.sqlite is not a snapshot file; "+
		"pass a .sqlite snapshot from ~/snapshots with --from <snapshot>")
	assert.Empty(t, fake.calls)
}

func Test_import_from_refuses_a_quicken_bundle_passed_as_from(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	fake := &fakeImporter{}
	srv := newImportServer(t, home, fake)
	bundle := v9fixture.OpenBundle(t, t.TempDir())

	_, err := srv.ImportFrom(t.Context(), bundle.Dir)

	require.EqualError(t, err, bundle.Dir+" is a Quicken file, not a snapshot; "+
		"pass it with --quicken <path>, or pass a snapshot with --from <snapshot>")
	assert.Empty(t, fake.calls)
}

func Test_import_from_reports_no_manifest_as_not_a_quarry_snapshot(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	fake := &fakeImporter{}
	srv := newImportServer(t, home, fake)
	snapshotPath := filepath.Join(home, "snapshots", "20260927T143005Z.sqlite")
	require.NoError(t, os.MkdirAll(filepath.Dir(snapshotPath), 0o700))
	require.NoError(t, os.WriteFile(snapshotPath, []byte("x"), 0o600))

	_, err := srv.ImportFrom(t.Context(), "20260927T143005Z")

	require.EqualError(t, err, "~/snapshots/20260927T143005Z.sqlite is not a quarry snapshot "+
		"(no .json manifest next to it); pass a snapshot taken by quarry sync with --from <snapshot>")
	assert.Empty(t, fake.calls)
}

func Test_import_from_refuses_without_importing_when_the_manifest_is_not_json(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	fake := &fakeImporter{}
	srv := newImportServer(t, home, fake)
	taken := takeSnapshot(t, srv)
	require.NoError(t, os.WriteFile(taken.Snapshot.Manifest, []byte("not json"), 0o600))

	_, err := srv.ImportFrom(t.Context(), snapshotIDFromPath(taken.Snapshot.Path))

	require.EqualError(t, err, homepath.Abbreviate(home, taken.Snapshot.Path)+
		" is not a quarry snapshot (its manifest is not readable JSON); pass a snapshot taken by quarry sync with --from <snapshot>")
	var syntaxErr *json.SyntaxError
	require.ErrorAs(t, err, &syntaxErr)
	assert.Empty(t, fake.calls)
}

func Test_import_from_refuses_without_importing_when_the_manifest_has_a_wrong_typed_field(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	fake := &fakeImporter{}
	srv := newImportServer(t, home, fake)
	taken := takeSnapshot(t, srv)
	require.NoError(t, os.WriteFile(taken.Snapshot.Manifest, []byte(`{"snapshot":{"sha256":5}}`), 0o600))

	_, err := srv.ImportFrom(t.Context(), snapshotIDFromPath(taken.Snapshot.Path))

	require.EqualError(t, err, homepath.Abbreviate(home, taken.Snapshot.Path)+
		" is not a quarry snapshot (its manifest is not readable JSON); pass a snapshot taken by quarry sync with --from <snapshot>")
	var typeErr *json.UnmarshalTypeError
	require.ErrorAs(t, err, &typeErr)
	assert.Empty(t, fake.calls)
}

// Only the manifest is chmod'd, not the snapshot: proves the refusal names
// whichever file's own read actually failed.
func Test_import_from_names_the_manifest_when_it_cannot_be_read(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		t.Skip("root ignores file permissions")
	}
	home := t.TempDir()
	fake := &fakeImporter{}
	srv := newImportServer(t, home, fake)
	taken := takeSnapshot(t, srv)
	require.NoError(t, os.Chmod(taken.Snapshot.Manifest, 0o000))
	t.Cleanup(func() { _ = os.Chmod(taken.Snapshot.Manifest, 0o600) })

	_, err := srv.ImportFrom(t.Context(), snapshotIDFromPath(taken.Snapshot.Path))

	require.EqualError(t, err, "cannot read "+homepath.Abbreviate(home, taken.Snapshot.Manifest)+
		": permission denied; check the file's permissions")
	assert.Empty(t, fake.calls)
}

// Only the snapshot is chmod'd, not the manifest: the mirror of the case above.
func Test_import_from_names_the_snapshot_when_it_cannot_be_read(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		t.Skip("root ignores file permissions")
	}
	home := t.TempDir()
	fake := &fakeImporter{}
	srv := newImportServer(t, home, fake)
	taken := takeSnapshot(t, srv)
	require.NoError(t, os.Chmod(taken.Snapshot.Path, 0o000))
	t.Cleanup(func() { _ = os.Chmod(taken.Snapshot.Path, 0o600) })

	_, err := srv.ImportFrom(t.Context(), snapshotIDFromPath(taken.Snapshot.Path))

	require.EqualError(t, err, "cannot read "+homepath.Abbreviate(home, taken.Snapshot.Path)+
		": permission denied; check the file's permissions")
	assert.Empty(t, fake.calls)
}

func Test_import_from_refuses_without_importing_when_the_snapshot_is_not_sqlite(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	fake := &fakeImporter{}
	srv := newImportServer(t, home, fake)
	snapshotPath := filepath.Join(home, "snapshots", "20260927T143005Z.sqlite")
	require.NoError(t, os.MkdirAll(filepath.Dir(snapshotPath), 0o700))
	require.NoError(t, os.WriteFile(snapshotPath, []byte("not a database, but long enough to have a header"), 0o600))
	writeManifestFor(t, snapshotPath)

	_, err := srv.ImportFrom(t.Context(), "20260927T143005Z")

	require.EqualError(t, err, "~/snapshots/20260927T143005Z.sqlite is not a quarry snapshot "+
		"(not a SQLite database); pass a snapshot taken by quarry sync with --from <snapshot>")
	assert.True(t, sqlite.IsNotADB(err), err.Error())
	assert.Empty(t, fake.calls)
}

// CorruptDataFile opens fine as SQLite (so sqlite.IsNotADB is false) and
// fails only PRAGMA integrity_check, isolating that arm from the not-a-database one above.
func Test_import_from_refuses_a_snapshot_that_fails_integrity_check(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	fake := &fakeImporter{}
	srv := newImportServer(t, home, fake)
	snapshotPath := filepath.Join(home, "snapshots", "20260927T143005Z.sqlite")
	require.NoError(t, os.MkdirAll(filepath.Dir(snapshotPath), 0o700))
	v9fixture.CorruptDataFile(t, snapshotPath)
	writeManifestFor(t, snapshotPath)

	_, err := srv.ImportFrom(t.Context(), "20260927T143005Z")

	require.EqualError(t, err, "~/snapshots/20260927T143005Z.sqlite is not a quarry snapshot "+
		"(not a SQLite database); pass a snapshot taken by quarry sync with --from <snapshot>")
	var integrityErr sqlite.IntegrityError
	require.ErrorAs(t, err, &integrityErr)
	assert.Empty(t, fake.calls)
}

// A pre-cancelled ctx makes inspectContent's own OpenReadOnly fail on its
// first query, before any content classification: ImportFrom must report
// the interruption, never a content refusal built from a cancellation.
func Test_import_from_reports_interrupted_when_ctx_is_already_cancelled_before_inspecting_content(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	fake := &fakeImporter{}
	srv := newImportServer(t, home, fake)
	taken := takeSnapshot(t, srv)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := srv.ImportFrom(ctx, snapshotIDFromPath(taken.Snapshot.Path))

	require.EqualError(t, err, "sync interrupted; nothing was kept; run quarry sync again")
	assert.Empty(t, fake.calls)
}

func Test_import_from_names_the_missing_accounts_in_the_refusal(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		ddl    []string
		reason string
	}{
		{name: "no accounts table", ddl: []string{"CREATE TABLE OTHER (id INTEGER PRIMARY KEY)"}, reason: "it has no accounts table"},
		{name: "no account rows", ddl: []string{"CREATE TABLE ZACCOUNT (Z_PK INTEGER PRIMARY KEY)"}, reason: "it has no accounts"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			fake := &fakeImporter{}
			srv := newImportServer(t, home, fake)
			writeSnapshotPair(t, filepath.Join(home, "snapshots"), "20260927T143005Z", c.ddl...)

			_, err := srv.ImportFrom(t.Context(), "20260927T143005Z")

			require.EqualError(t, err, "~/snapshots/20260927T143005Z.sqlite is not a quarry snapshot ("+c.reason+
				"); pass a snapshot taken by quarry sync with --from <snapshot>")
			assert.Empty(t, fake.calls)
		})
	}
}

// A rollback-journal file, so the held exclusive lock blocks readers; a
// short busy timeout keeps the wait for the read to give up brief.
func Test_import_from_refuses_a_locked_snapshot_as_unreadable(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	fake := &fakeImporter{}
	srv := newImportServer(t, home, fake, snapshot.WithBusyTimeout(50*time.Millisecond))
	snapshots := filepath.Join(home, "snapshots")
	writeSnapshotPair(t, snapshots, "20260927T143005Z",
		"CREATE TABLE ZACCOUNT (Z_PK INTEGER PRIMARY KEY)", "INSERT INTO ZACCOUNT (Z_PK) VALUES (1)")
	holdExclusiveLock(t, filepath.Join(snapshots, "20260927T143005Z.sqlite"))

	_, err := srv.ImportFrom(t.Context(), "20260927T143005Z")

	require.EqualError(t, err,
		"cannot read ~/snapshots/20260927T143005Z.sqlite: database is locked; take a new snapshot with quarry sync")
	assert.Empty(t, fake.calls)
}

// holdExclusiveLock holds an EXCLUSIVE transaction on the SQLite file at path until the test ends.
func holdExclusiveLock(t *testing.T, path string) {
	t.Helper()
	db, err := sql.Open("sqlite3", path)
	require.NoError(t, err)
	conn, err := db.Conn(t.Context())
	require.NoError(t, err)
	_, err = conn.ExecContext(t.Context(), "BEGIN EXCLUSIVE")
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = conn.Close()
		_ = db.Close()
	})
}

func Test_import_from_refuses_without_a_reference_schema(t *testing.T) {
	t.Parallel()
	fake := &fakeImporter{}
	srv := snapshot.NewServer(snapshot.WithImporter(fake), snapshot.WithSnapshotDir(t.TempDir()))

	_, err := srv.ImportFrom(t.Context(), "20260927T143005Z")

	require.EqualError(t, err, "no reference schema configured")
	assert.Empty(t, fake.calls)
}
