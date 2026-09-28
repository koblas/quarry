package snapshot_test

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/koblas/quarry/internal/platform/sqlite"
	"github.com/koblas/quarry/internal/quicken/v9"
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
		_, err := conn.Exec(stmt)
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
	data, err := snapshot.Manifest{Snapshot: snapshot.SnapshotInfo{
		Source: "/Users/x/Documents/Home.quicken", SHA256: hex.EncodeToString(sum[:]),
	}}.Encode()
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(strings.TrimSuffix(snapshotPath, ".sqlite")+".json", data, 0o600))
}

func Test_import_from_passes_the_same_snapshot_ref_as_a_plain_sync(t *testing.T) {
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

func Test_import_from_returns_the_manifest_a_plain_sync_returned(t *testing.T) {
	home := t.TempDir()
	srv := newImportServer(t, home, &fakeImporter{})
	bundle := v9fixture.OpenBundle(t, t.TempDir())
	synced, err := srv.SyncAndImport(t.Context(), bundle.Dir)
	require.NoError(t, err)

	outcome, err := srv.ImportFrom(t.Context(), snapshotIDFromPath(synced.Manifest.Snapshot.Path))

	require.NoError(t, err)
	assert.Equal(t, synced.Manifest, outcome.Manifest)
}

func Test_import_from_imports_a_snapshot_whose_manifest_said_unverified_once_the_current_reference_matches(t *testing.T) {
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

func Test_import_from_reports_a_v1_failure_like_a_plain_sync(t *testing.T) {
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

func Test_import_from_refuses_without_importing_when_a_file_cannot_be_read(t *testing.T) {
	cases := []struct {
		name  string
		setup func(t *testing.T, snapshotPath string)
		cause error
	}{
		{name: "manifest missing", setup: func(t *testing.T, snapshotPath string) {
			require.NoError(t, os.WriteFile(snapshotPath, []byte("x"), 0o600))
		}, cause: fs.ErrNotExist},
		{name: "snapshot missing", setup: func(t *testing.T, snapshotPath string) {
			require.NoError(t, os.WriteFile(strings.TrimSuffix(snapshotPath, ".sqlite")+".json", []byte(`{}`), 0o600))
		}, cause: fs.ErrNotExist},
		{name: "snapshot is a directory", setup: func(t *testing.T, snapshotPath string) {
			require.NoError(t, os.Mkdir(snapshotPath, 0o700))
			require.NoError(t, os.WriteFile(strings.TrimSuffix(snapshotPath, ".sqlite")+".json", []byte(`{}`), 0o600))
		}, cause: syscall.EISDIR},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := t.TempDir()
			fake := &fakeImporter{}
			srv := newImportServer(t, home, fake)
			snapshotPath := filepath.Join(home, "snapshots", "20260927T143005Z.sqlite")
			require.NoError(t, os.MkdirAll(filepath.Dir(snapshotPath), 0o700))
			c.setup(t, snapshotPath)

			_, err := srv.ImportFrom(t.Context(), "20260927T143005Z")

			require.ErrorIs(t, err, c.cause)
			assert.Empty(t, fake.calls)
		})
	}
}

func Test_import_from_refuses_without_importing_when_the_manifest_is_not_json(t *testing.T) {
	home := t.TempDir()
	fake := &fakeImporter{}
	srv := newImportServer(t, home, fake)
	taken := takeSnapshot(t, srv)
	require.NoError(t, os.WriteFile(taken.Snapshot.Manifest, []byte("not json"), 0o600))

	_, err := srv.ImportFrom(t.Context(), snapshotIDFromPath(taken.Snapshot.Path))

	var syntaxErr *json.SyntaxError
	require.ErrorAs(t, err, &syntaxErr)
	assert.Empty(t, fake.calls)
}

func Test_import_from_refuses_without_importing_when_the_snapshot_is_not_sqlite(t *testing.T) {
	home := t.TempDir()
	fake := &fakeImporter{}
	srv := newImportServer(t, home, fake)
	snapshotPath := filepath.Join(home, "snapshots", "20260927T143005Z.sqlite")
	require.NoError(t, os.MkdirAll(filepath.Dir(snapshotPath), 0o700))
	require.NoError(t, os.WriteFile(snapshotPath, []byte("not a database, but long enough to have a header"), 0o600))
	writeManifestFor(t, snapshotPath)

	_, err := srv.ImportFrom(t.Context(), "20260927T143005Z")

	require.Error(t, err)
	assert.True(t, sqlite.IsNotADB(err), err.Error())
	assert.Empty(t, fake.calls)
}

func Test_import_from_names_the_missing_accounts_in_the_refusal(t *testing.T) {
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

func Test_import_from_refuses_without_a_reference_schema(t *testing.T) {
	fake := &fakeImporter{}
	srv := snapshot.NewServer(snapshot.WithImporter(fake), snapshot.WithSnapshotDir(t.TempDir()))

	_, err := srv.ImportFrom(t.Context(), "20260927T143005Z")

	require.EqualError(t, err, "no reference schema configured")
	assert.Empty(t, fake.calls)
}
