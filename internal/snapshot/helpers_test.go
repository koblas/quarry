package snapshot_test

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/koblas/quarry/internal/platform/sqlschema"
	v9 "github.com/koblas/quarry/internal/quicken/v9"
	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/koblas/quarry/internal/snapshot"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	idOldest = "20260927T143005Z"
	idMiddle = "20260929T090011Z"
	idNewest = "20260930T141502Z"
	// idOutside names no snapshot in the folders the list tests build.
	idOutside = "20260801T120000Z"
)

// Snapshot IDs given to links: newer than every snapshot threeSnapshots writes, between its middle
// and newest, and older than all of them.
const (
	idLinkNewer   = "20261001T000000Z"
	idLinkBetween = "20260930T000000Z"
	idLinkOlder   = "20260901T000000Z"
)

// snapshotsFolder creates and returns the snapshots folder under home.
func snapshotsFolder(tb testing.TB, home string) string {
	tb.Helper()
	dir := filepath.Join(home, "snapshots")
	require.NoError(tb, os.MkdirAll(dir, 0o700))
	return dir
}

// writeSnapshot writes id's .sqlite as a sparse file of size bytes.
func writeSnapshot(tb testing.TB, dir, id string, size int64) string {
	tb.Helper()
	path := filepath.Join(dir, id+".sqlite")
	require.NoError(tb, os.WriteFile(path, nil, 0o600))
	require.NoError(tb, os.Truncate(path, size))
	return path
}

// writeManifest writes id's manifest through Manifest.Encode.
func writeManifest(tb testing.TB, dir, id string, manifest snapshot.Manifest) {
	tb.Helper()
	data, err := manifest.Encode()
	require.NoError(tb, err)
	require.NoError(tb, os.WriteFile(filepath.Join(dir, id+".json"), data, 0o600))
}

// manifestTaken is a manifest whose taken_at reads takenAt, for a Home.quicken source.
func manifestTaken(takenAt string) snapshot.Manifest {
	return snapshot.Manifest{
		Snapshot: snapshot.Info{Source: "/Users/x/Documents/Home.quicken", TakenAt: takenAt},
		Schema:   snapshot.SchemaInfo{Verified: true},
	}
}

// newListServer builds a Server over home's snapshots folder; probe is the store, nil for none.
func newListServer(home string, probe snapshot.StoreProbe, extra ...snapshot.Option) *snapshot.Server {
	opts := append([]snapshot.Option{snapshot.WithSnapshotDir(filepath.Join(home, "snapshots")), snapshot.WithHome(home)}, extra...)
	if probe != nil {
		opts = append(opts, snapshot.WithStoreProbe(probe))
	}
	return snapshot.NewServer(opts...)
}

func entryIDs(l snapshot.Listing) []string {
	ids := make([]string, len(l.Entries))
	for i, e := range l.Entries {
		ids[i] = e.ID
	}
	return ids
}

// skipUnderRoot skips the test under root, whom file modes do not stop.
func skipUnderRoot(tb testing.TB) {
	tb.Helper()
	if os.Geteuid() == 0 {
		tb.Skip("root ignores file modes")
	}
}

var errStoreRead = errors.New("store read failed")

// unreadableRecorded is a recorded snapshot path that fails to stat for reason, named idMiddle so
// the entry of that ID in the folder is the one a name match would mark.
type unreadableRecorded struct {
	name   string
	reason string
	// arrange builds the fault under home and returns the recorded path.
	arrange func(tb testing.TB, home string) string
}

// unreadableRecordedPaths are the ways the recorded path fails to stat for a reason other than not existing.
func unreadableRecordedPaths() []unreadableRecorded {
	return []unreadableRecorded{
		{
			name: "permission denied", reason: "permission denied",
			arrange: func(tb testing.TB, home string) string {
				tb.Helper()
				skipUnderRoot(tb)
				backup := filepath.Join(home, "Backup")
				require.NoError(tb, os.Mkdir(backup, 0o700))
				restrictMode(tb, backup, 0o000)
				return filepath.Join(backup, idMiddle+".sqlite")
			},
		},
		{
			name: "a symlink loop", reason: "too many levels of symbolic links",
			arrange: func(tb testing.TB, home string) string {
				tb.Helper()
				loop := filepath.Join(home, idMiddle+".sqlite")
				return symlink(tb, loop, loop)
			},
		},
		{
			name: "a parent that is a file", reason: "not a directory",
			arrange: func(tb testing.TB, home string) string {
				tb.Helper()
				require.NoError(tb, os.WriteFile(filepath.Join(home, "Backup"), nil, 0o600))
				return filepath.Join(home, "Backup", idMiddle+".sqlite")
			},
		},
		{
			name: "a name too long", reason: "file name too long",
			arrange: func(_ testing.TB, home string) string {
				return filepath.Join(home, strings.Repeat("a", 256), idMiddle+".sqlite")
			},
		},
	}
}

// homeRelative shows path under home as ~/...
func homeRelative(home, path string) string { return "~" + strings.TrimPrefix(path, home) }

// skipOnCaseSensitiveVolume skips the test unless the volume under dir resolves a file name in any letter case.
func skipOnCaseSensitiveVolume(tb testing.TB, dir string) {
	tb.Helper()
	require.NoError(tb, os.WriteFile(filepath.Join(dir, "caseprobe"), nil, 0o600))
	if _, err := os.Stat(filepath.Join(dir, "CASEPROBE")); err != nil {
		tb.Skip("the volume is case-sensitive: a differently-cased name does not resolve")
	}
}

// snapshotFile is the path of id's snapshot in dir.
func snapshotFile(dir, id string) string { return filepath.Join(dir, id+".sqlite") }

// hardLink makes link a second name for the file at target and returns link.
func hardLink(tb testing.TB, target, link string) string {
	tb.Helper()
	require.NoError(tb, os.Link(target, link))
	return link
}

// symlink makes link a symbolic link to target, creating link's folder, and returns link.
func symlink(tb testing.TB, target, link string) string {
	tb.Helper()
	require.NoError(tb, os.MkdirAll(filepath.Dir(link), 0o700))
	require.NoError(tb, os.Symlink(target, link))
	return link
}

// renamedExtension renames id's file from extension from to extension to.
// A rename is needed because case-insensitive volumes keep the old name's case on a rewrite.
func renamedExtension(tb testing.TB, dir, id, from, to string) {
	tb.Helper()
	require.NoError(tb, os.Rename(filepath.Join(dir, id+"."+from), filepath.Join(dir, id+"."+to)))
}

// listed writes a snapshot per id into home's folder and lists them with probe as the store.
func listed(tb testing.TB, home string, probe snapshot.StoreProbe, ids ...string) snapshot.Listing {
	tb.Helper()
	dir := snapshotsFolder(tb, home)
	for _, id := range ids {
		writeSnapshot(tb, dir, id, 1000)
	}
	listing, err := newListServer(home, probe).List(tb.Context())
	require.NoError(tb, err)
	return listing
}

// keptID is the ID of the snapshot kept for the store, "" when none was.
func keptID(kept *snapshot.Entry) string {
	if kept == nil {
		return ""
	}
	return kept.ID
}

func doomedIDs(entries []snapshot.Entry) []string {
	ids := make([]string, len(entries))
	for i, e := range entries {
		ids[i] = e.ID
	}
	return ids
}

// fakeRemover records the base name of each file Prune removes, in order, and fails those
// named in faults; every other file is really removed.
type fakeRemover struct {
	calls  []string
	faults map[string]error
	// cancel, when set, ends the context once cancelAfter has been removed (or has failed).
	cancelAfter string
	cancel      context.CancelFunc
}

func (r *fakeRemover) remove(path string) error {
	name := filepath.Base(path)
	r.calls = append(r.calls, name)
	if r.cancel != nil && name == r.cancelAfter {
		defer r.cancel()
	}
	if err, ok := r.faults[name]; ok {
		return err
	}
	return os.Remove(path)
}

// failing is a fakeRemover that fails name with the *fs.PathError os.Remove returns for errno.
func failing(name string, errno syscall.Errno) *fakeRemover { return failingRemover(errno, name) }

// newPruneServer is newListServer whose removes go through rm.
func newPruneServer(home string, probe snapshot.StoreProbe, rm *fakeRemover, extra ...snapshot.Option) *snapshot.Server {
	return newListServer(home, probe, append([]snapshot.Option{snapshot.WithRemove(rm.remove)}, extra...)...)
}

// prunable writes a snapshot with a manifest for each id under home's folder and returns the folder.
func prunable(tb testing.TB, home string, ids ...string) string {
	tb.Helper()
	dir := snapshotsFolder(tb, home)
	for _, id := range ids {
		writeSnapshot(tb, dir, id, 1000)
		writeManifest(tb, dir, id, manifestTaken("2026-09-27T10:00:00Z"))
	}
	return dir
}

// countingProbe is a fakeStoreProbe that counts BuiltFrom calls and runs onRead inside each.
type countingProbe struct {
	fakeStoreProbe

	calls  int
	onRead func()
}

func (p *countingProbe) BuiltFrom(ctx context.Context) (string, error) {
	p.calls++
	if p.onRead != nil {
		p.onRead()
	}
	return p.fakeStoreProbe.BuiltFrom(ctx)
}

// upperCased renames id's .sqlite in dir to .SQLITE and returns the new path.
// A rename is needed because case-insensitive volumes keep the old name's case on a rewrite.
func upperCased(tb testing.TB, dir, id string) string {
	tb.Helper()
	upper := filepath.Join(dir, id+".SQLITE")
	require.NoError(tb, os.Rename(filepath.Join(dir, id+".sqlite"), upper))
	return upper
}

// dirNames lists the names os.ReadDir returns for dir.
func dirNames(tb testing.TB, dir string) []string {
	tb.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(tb, err)
	return namesOf(entries)
}

// namesOf is the name of each of entries, in order.
func namesOf(entries []os.DirEntry) []string {
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.Name()
	}
	return names
}

// variantEntry reports name and mode in place of those of the real entry it wraps, keeping that entry's Info.
type variantEntry struct {
	fs.DirEntry

	name string
	mode fs.FileMode
}

func (e variantEntry) Name() string { return e.name }

func (e variantEntry) Type() fs.FileMode { return e.mode.Type() }

func (e variantEntry) IsDir() bool { return e.mode.IsDir() }

// variant is an extra entry named name that wraps the real entry named like and has type mode.
// A case-insensitive volume cannot hold two letter cases of one name, so the second case exists only in the listing.
type variant struct {
	name, like string
	mode       fs.FileMode
}

// regularVariant is a regular-file variant named name, wrapping the real entry named like.
func regularVariant(name, like string) variant { return variant{name: name, like: like} }

// readDirWith is os.ReadDir plus one extra entry for each of variants, in the order given.
func readDirWith(tb testing.TB, variants ...variant) func(string) ([]fs.DirEntry, error) {
	tb.Helper()
	return func(dir string) ([]fs.DirEntry, error) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return nil, err
		}
		onDisk := entries
		for _, v := range variants {
			i := indexOfEntry(onDisk, v.like)
			require.GreaterOrEqual(tb, i, 0, "no real entry named %s to wrap", v.like)
			entries = append(entries, variantEntry{DirEntry: onDisk[i], name: v.name, mode: v.mode})
		}
		return entries, nil
	}
}

func indexOfEntry(entries []fs.DirEntry, name string) int {
	for i, e := range entries {
		if e.Name() == name {
			return i
		}
	}
	return -1
}

// refusalText is the text of the RefusalError err holds; it fails the test when err holds none.
func refusalText(tb testing.TB, err error) string {
	tb.Helper()
	var refusal snapshot.RefusalError
	require.ErrorAs(tb, err, &refusal)
	return refusal.Error()
}

// restrictMode sets path's mode for the rest of the test and puts back 0o700 when it ends, so TempDir can clean up.
func restrictMode(tb testing.TB, path string, mode fs.FileMode) {
	tb.Helper()
	require.NoError(tb, os.Chmod(path, mode))
	tb.Cleanup(func() { assert.NoError(tb, os.Chmod(path, 0o700)) })
}

func newServer(t *testing.T, snapshotsDir string, opts ...snapshot.Option) *snapshot.Server {
	t.Helper()
	ref := v9Reference(t)
	return snapshot.NewServer(append([]snapshot.Option{
		snapshot.WithSnapshotDir(snapshotsDir),
		snapshot.WithReference(v9.ReferenceLabel, ref),
	}, opts...)...)
}

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

// fakeStoreProbe is a hand-written StoreProbe fake reporting a fixed path and existence,
// and answering BuiltFrom with builtFrom, or builtFromErr when set.
type fakeStoreProbe struct {
	path         string
	exists       bool
	builtFrom    string
	builtFromErr error
}

func (f *fakeStoreProbe) Path() string { return f.path }
func (f *fakeStoreProbe) Exists() bool { return f.exists }

func (f *fakeStoreProbe) BuiltFrom(context.Context) (string, error) {
	return f.builtFrom, f.builtFromErr
}

// newImportServer builds a Server whose snapshots and store live under home; opts override those.
func newImportServer(t *testing.T, home string, imp snapshot.Importer, opts ...snapshot.Option) *snapshot.Server {
	t.Helper()
	ref := v9Reference(t)
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

func historyRestartLine(reason string) string {
	return "cannot carry import history forward from the previous store (" + reason + "); import_runs starts again with this sync"
}

func findingsRestartLine(reason string) string {
	return "cannot carry findings forward from the previous store (" + reason + "); findings history starts again with this sync"
}

func combinedCarryLine(reason string) string {
	return "cannot carry import history, findings or exchange rates forward from the previous store (" + reason + "); all three start again with this sync"
}

func ratesRestartLine(reason string) string {
	return "cannot carry exchange rates forward from the previous store (" + reason + "); fetching them all again"
}

// fakeSource is a hand-written Source fake: each call returns the
// configured error, or succeeds when it is nil.
type fakeSource struct {
	openErr, probeErr, backupErr error
}

func (f *fakeSource) Open(context.Context, string) error   { return f.openErr }
func (f *fakeSource) Probe(context.Context) error          { return f.probeErr }
func (f *fakeSource) Backup(context.Context, string) error { return f.backupErr }
func (f *fakeSource) Close() error                         { return nil }

var errBoom = errors.New("boom")

// fixedPathDestination hands Backup's caller a pre-built file instead of
// really backing anything up, so a test controls exactly what buildManifest
// reads.
type fixedPathDestination struct {
	snapshotPath string
}

func (f *fixedPathDestination) Prepare(context.Context) error { return nil }
func (f *fixedPathDestination) Backup(_ context.Context, _ snapshot.Source, name string) (string, string, error) {
	return f.snapshotPath, name, nil
}

func (f *fixedPathDestination) WriteManifest(context.Context, string, []byte) (string, error) {
	return "", nil
}

func (f *fixedPathDestination) CommitSnapshot(context.Context, string) (string, error) {
	return "", nil
}

func (f *fixedPathDestination) CommitManifest(context.Context, string) (string, error) {
	return "", nil
}
func (f *fixedPathDestination) FinalPaths(string) (string, string)    { return "", "" }
func (f *fixedPathDestination) Discard(context.Context, string) error { return nil }

// assertSnapshotsDirEmpty fails if dir holds anything at all: unlike
// assertNoPartialsLeftBehind, it also catches a committed final Discard
// should have removed.
func assertSnapshotsDirEmpty(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

// assertNoPartialsLeftBehind fails if dir holds any of Destination's
// exclusively-created ".partial" files: every Destination method that
// creates one removes it again on its own failure.
func assertNoPartialsLeftBehind(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return
	}
	require.NoError(t, err)
	for _, entry := range entries {
		assert.False(t, strings.HasSuffix(entry.Name(), ".partial"), "leftover partial: %s", entry.Name())
	}
}

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
	sqliteFileWith(t, path, ddl...)
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

// oldIDs returns n snapshot IDs from the year 2000, oldest first: older than any ID SyncAndImport mints.
func oldIDs(n int) []string {
	ids := make([]string, n)
	for i := range ids {
		ids[i] = fmt.Sprintf("2000%02d01T000000Z", i+1)
	}
	return ids
}

// futureIDs returns two snapshot IDs newer than any ID SyncAndImport mints, oldest first.
func futureIDs() []string { return []string{"20990101T000000Z", "20990201T000000Z"} }

// builtStore is the Importer result of a store that was built.
func builtStore() *fakeImporter { return &fakeImporter{result: store.Result{Built: true}} }

// syncBundle runs SyncAndImport over a fresh fixture bundle.
func syncBundle(t *testing.T, srv *snapshot.Server) (snapshot.Outcome, error) {
	t.Helper()
	return srv.SyncAndImport(t.Context(), v9fixture.OpenBundle(t, t.TempDir()).Dir)
}

// assertSnapshotPairs asserts each id's .sqlite and .json in dir exist (want) or are gone (!want).
func assertSnapshotPairs(t *testing.T, dir string, want bool, ids ...string) {
	t.Helper()
	for _, id := range ids {
		for _, ext := range []string{".sqlite", ".json"} {
			if want {
				assert.FileExists(t, filepath.Join(dir, id+ext))
			} else {
				assert.NoFileExists(t, filepath.Join(dir, id+ext))
			}
		}
	}
}

// deleteFailureLine is the warning for a snapshot that could not be deleted, for reason.
func deleteFailureLine(id, reason string) string {
	return "cannot delete snapshot " + id + ": " + reason + "; run quarry snapshots prune to try again"
}

// failingRemover returns a fakeRemover that fails each named file with errno as os.Remove reports it.
func failingRemover(errno syscall.Errno, names ...string) *fakeRemover {
	rm := &fakeRemover{faults: map[string]error{}}
	for _, name := range names {
		rm.faults[name] = &fs.PathError{Op: "remove", Path: name, Err: errno}
	}
	return rm
}

// v9Reference is the Quicken v9 schema reference, which a test may edit before handing it to the Server.
func v9Reference(tb testing.TB) sqlschema.Schema {
	tb.Helper()
	ref, err := v9.Reference(tb.Context())
	require.NoError(tb, err)
	return ref
}

// newDestinationServer builds a Server that syncs through dest, with the v9 schema reference.
func newDestinationServer(tb testing.TB, home, snapshotsDir string, dest snapshot.Destination) *snapshot.Server {
	tb.Helper()
	return snapshot.NewServer(
		snapshot.WithSnapshotDir(snapshotsDir),
		snapshot.WithReference(v9.ReferenceLabel, v9Reference(tb)),
		snapshot.WithDestination(dest),
		snapshot.WithHome(home),
	)
}

// syncSnapshotCopy syncs ~/Documents/Home.quicken under a fresh home through a source whose backup is the
// file at snapshotPath, and returns Sync's error.
func syncSnapshotCopy(tb testing.TB, snapshotPath string) error {
	tb.Helper()
	home := tb.TempDir()
	srv := snapshot.NewServer(
		snapshot.WithReference(v9.ReferenceLabel, sqlschema.Schema{}),
		snapshot.WithSource(&fakeSource{}),
		snapshot.WithDestination(&fixedPathDestination{snapshotPath: snapshotPath}),
		snapshot.WithHome(home),
	)
	_, err := srv.Sync(tb.Context(), filepath.Join(home, "Documents", "Home.quicken"))
	return err
}

// sqliteFileWith creates the SQLite file at path by running each of statements, in order, on one connection.
func sqliteFileWith(tb testing.TB, path string, statements ...string) {
	tb.Helper()
	conn, err := sql.Open("sqlite3", path)
	require.NoError(tb, err)
	for _, stmt := range statements {
		_, err := conn.ExecContext(tb.Context(), stmt)
		require.NoError(tb, err)
	}
	require.NoError(tb, conn.Close())
}
