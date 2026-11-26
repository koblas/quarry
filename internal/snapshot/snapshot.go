package snapshot

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/koblas/quarry/internal/finding"
	"github.com/koblas/quarry/internal/platform/sqlite"
	"github.com/koblas/quarry/internal/platform/sqlschema"
	"github.com/koblas/quarry/internal/store"
)

// snapshotNameLayout is the UTC, colon-free timestamp each snapshot pair is named with.
const snapshotNameLayout = "20060102T150405Z"

// primaryKeyTable is the Core Data table that records each entity's number.
const primaryKeyTable = "Z_PRIMARYKEY"

// DefaultBusyTimeout is how long Sync waits for a busy or locked source before refusing.
const DefaultBusyTimeout = 5 * time.Second

// Server takes verified snapshots of a Quicken bundle into a snapshots
// directory, checking each one against a reference schema.
type Server struct {
	snapshotDir    string
	referenceLabel string
	reference      sqlschema.Schema
	home           string
	busyTimeout    time.Duration

	source      Source
	destination Destination
	importer    Importer
	storeProbe  StoreProbe
	locker      Locker
	remove      func(path string) error
	readDir     func(dir string) ([]fs.DirEntry, error)
	autoKeep    int
	ignore      []string
	readTime    func(store.FindingList) []finding.State
	resolver    EntityResolver
}

// Option configures a Server built by NewServer.
type Option func(*Server)

// WithSnapshotDir sets the directory snapshots and manifests are written
// into. It is created 0700 on first use.
func WithSnapshotDir(dir string) Option {
	return func(s *Server) { s.snapshotDir = dir }
}

// WithReference sets the schema Sync compares snapshots against and the
// label recorded for it in each manifest.
func WithReference(label string, schema sqlschema.Schema) Option {
	return func(s *Server) {
		s.referenceLabel = label
		s.reference = schema
	}
}

// EntityResolver renames a reference table or column name to the numbering of the file being checked, given
// each Z_NAME of the file's Z_PRIMARYKEY and its Z_ENT numbers; ok is false when it cannot.
type EntityResolver func(name string, entities map[string][]int64) (resolved string, ok bool)

// WithEntityResolver sets the EntityResolver the schema comparison renames reference names through.
func WithEntityResolver(resolver EntityResolver) Option {
	return func(s *Server) { s.resolver = resolver }
}

// WithSource overrides the Source Sync opens the bundle with. Production
// callers do not need it; tests use it to inject a fake.
func WithSource(source Source) Option {
	return func(s *Server) { s.source = source }
}

// WithDestination overrides the Destination Sync writes into. Production
// callers do not need it; tests use it to inject a fake.
func WithDestination(destination Destination) Option {
	return func(s *Server) { s.destination = destination }
}

// WithHome sets the home directory Sync abbreviates refusal messages
// raised from inside Sync itself against.
func WithHome(home string) Option {
	return func(s *Server) { s.home = home }
}

// WithBusyTimeout overrides DefaultBusyTimeout.
func WithBusyTimeout(d time.Duration) Option {
	return func(s *Server) { s.busyTimeout = d }
}

// WithImporter sets the Importer SyncAndImport builds the store with once a
// snapshot's schema verifies. Production callers wire the importer package
// through it; tests use it to inject a fake.
func WithImporter(imp Importer) Option {
	return func(s *Server) { s.importer = imp }
}

// WithStoreProbe sets the StoreProbe that locates the store the wired
// Importer writes; production callers pass that Importer's own store.
func WithStoreProbe(probe StoreProbe) Option {
	return func(s *Server) { s.storeProbe = probe }
}

// WithLocker sets the Locker LockForSync takes the writer lock with; unset,
// LockForSync takes no lock.
func WithLocker(locker Locker) Option {
	return func(s *Server) { s.locker = locker }
}

// WithRemove sets the function that deletes a file in the snapshots
// directory; it defaults to os.Remove. Tests use it to inject a failing delete.
func WithRemove(remove func(path string) error) Option {
	return func(s *Server) { s.remove = remove }
}

// WithReadDir sets the function that lists a snapshots folder, os.ReadDir by default. Tests use it
// to list two letter cases of one name, which a case-insensitive volume cannot hold.
func WithReadDir(readDir func(dir string) ([]fs.DirEntry, error)) Option {
	return func(s *Server) { s.readDir = readDir }
}

// WithAutoPrune makes SyncAndImport and ImportFrom delete the snapshots beyond the newest
// keep once the store is built; unset or below 1, they delete nothing.
func WithAutoPrune(keep int) Option {
	return func(s *Server) { s.autoKeep = keep }
}

// WithIgnore sets the finding ids of findings.ignore, which SyncAndImport and ImportFrom
// leave out of the open and new counts of the store they build and count as ignored.
func WithIgnore(ids []string) Option {
	return func(s *Server) { s.ignore = ids }
}

// WithReadTimeFindings sets the function that computes, from the accounts a build wrote, the findings
// no build records; SyncAndImport and ImportFrom count them with the stored ones, never as new.
func WithReadTimeFindings(states func(store.FindingList) []finding.State) Option {
	return func(s *Server) { s.readTime = states }
}

// NewServer builds a Server from opts.
func NewServer(opts ...Option) *Server {
	s := &Server{busyTimeout: DefaultBusyTimeout, remove: os.Remove, readDir: os.ReadDir, resolver: keepName}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Home returns the home directory Sync abbreviates refusal messages
// against, as set by WithHome.
func (s *Server) Home() string { return s.home }

// errNoReference is Sync's error when the Server has no reference schema.
var errNoReference = errors.New("no reference schema configured")

// Sync takes a verified snapshot of the Quicken bundle at bundlePath: it
// opens, probes and backs up the live file read-only, then checks the copy
// against the configured reference schema. It returns errNoReference with
// none configured, or the committed Manifest with a MismatchError when the
// schema check finds a missing table or column.
func (s *Server) Sync(ctx context.Context, bundlePath string) (Manifest, error) {
	if s.reference == nil {
		return Manifest{}, errNoReference
	}

	source := s.source
	if source == nil {
		source = newSQLiteSource(s.busyTimeout)
	}
	destination := s.destination
	if destination == nil {
		destination = newDirDestination(s.snapshotDir, s.readDir)
	}

	dataPath := filepath.Join(bundlePath, "data")
	if err := source.Open(ctx, dataPath); err != nil {
		_, refusal := sourceRefusal(s.home, bundlePath, err)
		return Manifest{}, FailureOutcome(ctx, refusal)
	}
	defer func() { _ = source.Close() }()

	if err := source.Probe(ctx); err != nil {
		_, refusal := sourceRefusal(s.home, bundlePath, err)
		return Manifest{}, FailureOutcome(ctx, refusal)
	}

	if err := destination.Prepare(ctx); err != nil {
		return Manifest{}, FailureOutcome(ctx, unwritableDirRefusal(s.home, s.snapshotDir, err))
	}

	takenAt := time.Now().UTC()
	name := takenAt.Format(snapshotNameLayout)

	snapshotPartial, resolvedName, err := destination.Backup(ctx, source, name)
	if err != nil {
		if ok, refusal := sourceRefusal(s.home, bundlePath, err); ok {
			return Manifest{}, FailureOutcome(ctx, refusal)
		}
		return Manifest{}, FailureOutcome(ctx, backupFailureRefusal(s.home, bundlePath, s.snapshotDir, err))
	}
	snapshotPath, manifestPath := destination.FinalPaths(resolvedName)

	manifest, err := s.buildManifest(ctx, snapshotPartial, bundlePath, takenAt)
	if err != nil {
		// Best-effort: a Discard failure never replaces the classified
		// refusal below, mirroring the deferred source.Close() above.
		_ = destination.Discard(ctx, snapshotPartial)
		return Manifest{}, FailureOutcome(ctx, contentRefusal(s.home, bundlePath, err))
	}
	// Set before Encode: the committed manifest must carry the paths Sync returns.
	manifest.Snapshot.Path = snapshotPath
	manifest.Snapshot.Manifest = manifestPath
	manifest.Warnings = schemaWarnings(bundlePath, manifestPath, manifest.Schema)

	manifestBytes, err := manifest.Encode()
	if err != nil {
		// uncovered: Manifest.Encode's own error path is unreachable for any value this package builds; see there.
		_ = destination.Discard(ctx, snapshotPartial)
		return Manifest{}, FailureOutcome(ctx, contentRefusal(s.home, bundlePath, fmt.Errorf("encode manifest: %w", err)))
	}
	manifestPartial, err := destination.WriteManifest(ctx, resolvedName, manifestBytes)
	if err != nil {
		_ = destination.Discard(ctx, snapshotPartial)
		return Manifest{}, FailureOutcome(ctx, writeFaultRefusal(s.home, s.snapshotDir, err))
	}

	if err := s.commit(ctx, destination, manifestPartial, snapshotPartial, manifestPath); err != nil {
		return Manifest{}, err
	}

	if !manifest.Schema.Verified {
		return manifest, mismatchError(s.home, bundlePath, snapshotPath, manifest.Schema)
	}
	return manifest, nil
}

// commit checks ctx once, then renames the manifest and snapshot partials
// into place in that order, so a crash never leaves a snapshot without a manifest.
func (s *Server) commit(ctx context.Context, destination Destination, manifestPartial, snapshotPartial, manifestPath string) error {
	if ctx.Err() != nil {
		_ = destination.Discard(ctx, manifestPartial)
		_ = destination.Discard(ctx, snapshotPartial)
		return InterruptedRefusal()
	}

	if _, err := destination.CommitManifest(ctx, manifestPartial); err != nil {
		_ = destination.Discard(ctx, manifestPartial)
		_ = destination.Discard(ctx, snapshotPartial)
		return writeFaultRefusal(s.home, s.snapshotDir, err)
	}
	if _, err := destination.CommitSnapshot(ctx, snapshotPartial); err != nil {
		// The manifest final is discarded first, before the snapshot
		// partial, so the reservation stays held until the last step.
		_ = destination.Discard(ctx, manifestPath)
		_ = destination.Discard(ctx, snapshotPartial)
		return writeFaultRefusal(s.home, s.snapshotDir, err)
	}
	return nil
}

// buildManifest reads the just-created snapshot at snapshotPath — every
// read past the probe runs on the snapshot, never the live file — and
// assembles the manifest: integrity check, account count, hash, and schema
// diff against the configured reference.
func (s *Server) buildManifest(ctx context.Context, snapshotPath, source string, takenAt time.Time) (Manifest, error) {
	content, err := inspectContent(ctx, snapshotPath, s.busyTimeout)
	if err != nil {
		return Manifest{}, err
	}

	size, sum, err := hashFile(snapshotPath)
	if err != nil {
		// uncovered: inspectContent already opened this same path above; a plain read cannot fail where SQLite's own read just succeeded.
		return Manifest{}, err
	}

	return Manifest{
		Snapshot: Info{
			Source:   source,
			TakenAt:  takenAt.Format(time.RFC3339),
			Bytes:    size,
			SHA256:   sum,
			Accounts: content.accounts,
		},
		Schema:   s.compareSchema(content.schema, content.entities),
		Warnings: []string{},
	}, nil
}

// fileContent is what inspectContent reads from a snapshot: its account count, its schema, and each Z_NAME of
// its Z_PRIMARYKEY with the Z_ENT numbers listed for it.
type fileContent struct {
	accounts int
	schema   sqlschema.Schema
	entities map[string][]int64
}

// inspectContent opens the snapshot at path read-only and returns its accounts, schema and entity numbers once its
// integrity and ZACCOUNT checks pass; a lock held past busyTimeout fails with a sqlite.IsBusy error.
func inspectContent(ctx context.Context, path string, busyTimeout time.Duration) (fileContent, error) {
	snap, err := sqlite.OpenReadOnlyBusy(ctx, path, busyTimeout)
	if err != nil {
		return fileContent{}, fmt.Errorf("open snapshot: %w", err)
	}
	defer func() { _ = snap.Close() }()

	if err := snap.IntegrityCheck(ctx); err != nil {
		return fileContent{}, fmt.Errorf("integrity check: %w", err)
	}

	exists, err := snap.QueryInt(ctx, "SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = 'ZACCOUNT'")
	if err != nil {
		// uncovered: only a ctx cancelled between IntegrityCheck and this query reaches here, a race no test can pin; the caller's FailureOutcome classifies it as interrupted.
		return fileContent{}, fmt.Errorf("check accounts table: %w", err)
	}
	if exists == 0 {
		return fileContent{}, errNoAccountsTable
	}

	accounts, err := snap.QueryInt(ctx, "SELECT count(*) FROM ZACCOUNT")
	if err != nil {
		// uncovered: only a ctx cancelled between the existence check and this query reaches here, a race no test can pin; the caller's FailureOutcome classifies it as interrupted.
		return fileContent{}, fmt.Errorf("count accounts: %w", err)
	}
	if accounts == 0 {
		return fileContent{}, errNoAccounts
	}

	actual, err := snap.Schema(ctx)
	if err != nil {
		return fileContent{}, fmt.Errorf("read snapshot schema: %w", err)
	}
	entities, err := readEntities(ctx, snap, actual)
	if err != nil {
		return fileContent{}, err
	}
	return fileContent{accounts: accounts, schema: actual, entities: entities}, nil
}

// readEntities returns each Z_NAME of snap's Z_PRIMARYKEY with every Z_ENT listed for it, leaving out rows
// with no Z_NAME. A file without the table has no entity numbers, so the result is empty.
func readEntities(ctx context.Context, snap *sqlite.DB, schema sqlschema.Schema) (map[string][]int64, error) {
	entities := map[string][]int64{}
	if _, ok := schema[primaryKeyTable]; !ok {
		return entities, nil
	}
	query := "SELECT Z_ENT, Z_NAME FROM " + primaryKeyTable + " WHERE Z_NAME IS NOT NULL"
	err := snap.QueryRows(ctx, query, nil, func(scan func(dest ...any) error) error {
		var number int64
		var name string
		if err := scan(&number, &name); err != nil {
			return err
		}
		entities[name] = append(entities[name], number)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("read entity numbers: %w", err)
	}
	return entities, nil
}

// hashFile streams the file at path through SHA-256, returning its size
// and hex digest.
func hashFile(path string) (int64, string, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, "", fmt.Errorf("read snapshot: %w", err)
	}
	defer func() { _ = f.Close() }()

	h := sha256.New()
	size, err := io.Copy(h, f)
	if err != nil {
		return 0, "", fmt.Errorf("read snapshot: %w", err)
	}
	return size, hex.EncodeToString(h.Sum(nil)), nil
}

// compareSchema diffs actual against the configured reference renamed to the numbering entities gives the
// file; the counts and the reference fingerprint stay those of the reference as written.
func (s *Server) compareSchema(actual sqlschema.Schema, entities map[string][]int64) SchemaInfo {
	resolved := resolveReference(scopeSchema(s.reference), s.nameResolver(entities))
	diff := sqlschema.Compare(resolved.schema, scopeSchema(actual))
	diff.MissingColumns = resolved.asWritten(diff.MissingColumns)
	return schemaInfoFromDiff(s.referenceLabel, s.reference, actual, diff)
}

// schemaWarnings returns a manifest's warnings for info: the extras warning
// when the schema has only extras, else none.
func schemaWarnings(bundlePath, manifestPath string, info SchemaInfo) []string {
	if hasOnlyExtras(info) {
		return []string{extrasWarningText(bundlePath, manifestPath, info)}
	}
	return []string{}
}

// schemaInfoFromDiff assembles SchemaInfo from a schema comparison.
func schemaInfoFromDiff(referenceLabel string, reference, actual sqlschema.Schema, diff sqlschema.Diff) SchemaInfo {
	scopedReference := scopeSchema(reference)
	referenceColumns := 0
	for _, cols := range scopedReference {
		referenceColumns += len(cols)
	}

	return SchemaInfo{
		Reference:            referenceLabel,
		Verified:             len(diff.MissingTables) == 0 && len(diff.MissingColumns) == 0,
		Fingerprint:          sqlschema.Fingerprint(scopeSchema(actual)),
		ReferenceFingerprint: sqlschema.Fingerprint(scopedReference),
		MissingTables:        diff.MissingTables,
		MissingColumns:       toManifestColumns(diff.MissingColumns),
		UnexpectedTables:     diff.UnexpectedTables,
		UnexpectedColumns:    toManifestColumns(diff.UnexpectedColumns),
		ReferenceTables:      len(scopedReference),
		ReferenceColumns:     referenceColumns,
	}
}

func toManifestColumns(refs []sqlschema.ColumnRef) []ColumnRef {
	cols := make([]ColumnRef, len(refs))
	for i, r := range refs {
		cols[i] = ColumnRef{Table: r.Table, Column: r.Column}
	}
	return cols
}
