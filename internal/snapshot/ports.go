package snapshot

import "context"

// Source is the live Quicken bundle's read-only port: open, probe for
// encryption, and back itself up. BR-1 confines every other read (integrity
// check, account count, hash, schema) to the snapshot Destination.Backup
// produces, never to the live file.
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
// failure (R14) without depending on real disk space.
type Destination interface {
	// Prepare creates the snapshots directory (0700) if it does not exist.
	Prepare(ctx context.Context) error
	// Backup creates a new, exclusively-created 0600 partial snapshot file
	// for name and backs up src into it, returning the partial's path and
	// the name actually reserved: name, or "<name>_2", "<name>_3", ... if
	// name's final snapshot or manifest already exists. It removes its own
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
