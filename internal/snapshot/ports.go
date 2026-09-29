package snapshot

import (
	"context"

	"github.com/koblas/quarry/internal/store"
)

// Source is the live Quicken bundle's read-only port: open, probe for
// encryption, and back itself up. Every other read (integrity check,
// account count, hash, schema) runs against the snapshot Destination.Backup
// produces, never against the live file.
type Source interface {
	// Open opens path's data file read-only.
	Open(ctx context.Context, path string) error
	// Probe reads the open connection to detect an encrypted (closed)
	// Quicken file before anything is written to disk.
	Probe(ctx context.Context) error
	// Backup copies the open connection's database into destPath, which
	// must already exist, using SQLite's online backup API.
	Backup(ctx context.Context, destPath string) error
	// Close closes the open connection, if any.
	Close() error
}

// Destination is the snapshots-directory write port: every write that
// reaches disk during Sync goes through it, so a fake can inject a write
// failure without depending on real disk space.
type Destination interface {
	// Prepare creates the snapshots directory (0700) if it does not exist.
	Prepare(ctx context.Context) error
	// Backup creates a new, exclusively-created 0600 partial for name and
	// backs up src into it, returning the partial's path and the name
	// actually reserved ("<name>_2", ... on collision). It removes its own
	// partial on any failure.
	Backup(ctx context.Context, src Source, name string) (partial, resolvedName string, err error)
	// WriteManifest writes data to a new, exclusively-created 0600 partial
	// manifest file named for name, returning its path.
	WriteManifest(ctx context.Context, name string, data []byte) (partial string, err error)
	// CommitSnapshot renames a snapshot partial into its final name,
	// refusing to overwrite an existing file.
	CommitSnapshot(ctx context.Context, partial string) (final string, err error)
	// CommitManifest renames a manifest partial into its final name,
	// refusing to overwrite an existing file.
	CommitManifest(ctx context.Context, partial string) (final string, err error)
	// FinalPaths predicts the snapshot and manifest paths CommitSnapshot and
	// CommitManifest will produce for name.
	FinalPaths(name string) (snapshotPath, manifestPath string)
	// Discard removes a path Sync hands it — a snapshot or manifest
	// partial, or an already-committed final Sync must undo after a
	// later step fails — returning any removal error unchanged.
	Discard(ctx context.Context, partial string) error
}

// Importer builds quarry's store from a committed snapshot. It is shaped to
// (*importer.Server).Import so that type satisfies it with no adapter.
type Importer interface {
	// Import maps snap.Path's data into quarry's schema and writes it
	// through the importer's own Store, returning the store.Result it built.
	Import(ctx context.Context, snap store.SnapshotRef) (store.Result, error)
}

// StoreProbe locates the store the wired Importer writes, so refusal copy
// and the unbuilt-store result name the file the Importer actually uses.
type StoreProbe interface {
	// Path returns the store file's absolute path.
	Path() string
	// Exists reports whether a store file is already at Path.
	Exists() bool
}
