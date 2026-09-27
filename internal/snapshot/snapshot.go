package snapshot

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/koblas/quarry/internal/platform/sqlite"
	"github.com/koblas/quarry/internal/platform/sqlschema"
)

// snapshotNameLayout is the UTC, colon-free timestamp Sync names each
// snapshot and manifest pair with.
const snapshotNameLayout = "20060102T150405Z"

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

// NewServer builds a Server from opts.
func NewServer(opts ...Option) *Server {
	s := &Server{busyTimeout: DefaultBusyTimeout}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// errNoReference is returned by Sync when the Server has no reference
// schema configured.
var errNoReference = fmt.Errorf("no reference schema configured")

// Sync takes a verified snapshot of the Quicken bundle at bundlePath (BR-8):
// it opens bundlePath's data file read-only and probes it, then, only once
// the probe succeeds, backs it up into the snapshots directory, checks the
// copy's integrity, account count, hash and schema against the configured
// reference, and commits the manifest before the snapshot. It returns
// errNoReference if no reference schema was configured.
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
		destination = newDirDestination(s.snapshotDir)
	}

	dataPath := filepath.Join(bundlePath, "data")
	if err := source.Open(ctx, dataPath); err != nil {
		return Manifest{}, sourceRefusal(s.home, bundlePath, err)
	}
	defer func() { _ = source.Close() }()

	if err := source.Probe(ctx); err != nil {
		return Manifest{}, sourceRefusal(s.home, bundlePath, err)
	}

	if err := destination.Prepare(ctx); err != nil {
		return Manifest{}, fmt.Errorf("sync %s: %w", bundlePath, err)
	}

	takenAt := time.Now().UTC()
	name := takenAt.Format(snapshotNameLayout)
	snapshotPath, manifestPath := destination.FinalPaths(name)

	snapshotPartial, err := destination.Backup(ctx, source, name)
	if err != nil {
		return Manifest{}, sourceRefusal(s.home, bundlePath, err)
	}

	manifest, err := s.buildManifest(ctx, snapshotPartial, bundlePath, takenAt)
	if err != nil {
		return Manifest{}, fmt.Errorf("sync %s: %w", bundlePath, err)
	}
	// Set before Encode: the committed manifest must carry the paths Sync returns.
	manifest.Snapshot.Path = snapshotPath
	manifest.Snapshot.Manifest = manifestPath

	manifestBytes, err := manifest.Encode()
	if err != nil {
		// unreachable: Manifest.Encode's own error path is unreachable for any value this package builds; see there.
		return Manifest{}, fmt.Errorf("sync %s: encode manifest: %w", bundlePath, err)
	}
	manifestPartial, err := destination.WriteManifest(ctx, name, manifestBytes)
	if err != nil {
		return Manifest{}, fmt.Errorf("sync %s: %w", bundlePath, err)
	}

	// Manifest commits first: BR-8 accepts a snapshot only if its manifest
	// exists, so a crash between the two commits must never leave a
	// snapshot without one.
	if _, err := destination.CommitManifest(ctx, manifestPartial); err != nil {
		return Manifest{}, fmt.Errorf("sync %s: %w", bundlePath, err)
	}
	if _, err := destination.CommitSnapshot(ctx, snapshotPartial); err != nil {
		return Manifest{}, fmt.Errorf("sync %s: %w", bundlePath, err)
	}

	return manifest, nil
}

// buildManifest reads the just-created snapshot at snapshotPath (BR-1: every
// read past the probe runs on the snapshot, never the live file) and
// assembles the manifest BR-8's remaining steps produce: integrity check,
// account count, hash, and schema diff against the configured reference.
func (s *Server) buildManifest(ctx context.Context, snapshotPath, source string, takenAt time.Time) (Manifest, error) {
	snap, err := sqlite.OpenReadOnly(ctx, snapshotPath)
	if err != nil {
		return Manifest{}, fmt.Errorf("open snapshot: %w", err)
	}
	defer func() { _ = snap.Close() }()

	if err := snap.IntegrityCheck(ctx); err != nil {
		return Manifest{}, fmt.Errorf("integrity check: %w", err)
	}

	accounts, err := snap.QueryInt(ctx, "SELECT count(*) FROM ZACCOUNT")
	if err != nil {
		return Manifest{}, fmt.Errorf("count accounts: %w", err)
	}

	raw, err := os.ReadFile(snapshotPath)
	if err != nil {
		// unreachable: sqlite.OpenReadOnly already opened this same path above; a plain read cannot fail where SQLite's own read just succeeded.
		return Manifest{}, fmt.Errorf("read snapshot: %w", err)
	}
	sum := sha256.Sum256(raw)

	actual, err := snap.Schema(ctx)
	if err != nil {
		// unreachable: this connection just ran IntegrityCheck and QueryInt above; Schema's own error paths on the same connection are unreachable, see platform/sqlite.
		return Manifest{}, fmt.Errorf("read snapshot schema: %w", err)
	}
	diff := sqlschema.Compare(scopeSchema(s.reference), scopeSchema(actual))

	return Manifest{
		Snapshot: SnapshotInfo{
			Source:   source,
			TakenAt:  takenAt.Format(time.RFC3339),
			Bytes:    int64(len(raw)),
			SHA256:   hex.EncodeToString(sum[:]),
			Accounts: accounts,
		},
		Schema:   schemaInfoFromDiff(s.referenceLabel, s.reference, actual, diff),
		Warnings: []string{},
	}, nil
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
