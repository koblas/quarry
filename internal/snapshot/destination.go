package snapshot

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/koblas/quarry/internal/platform/atomicfile"
)

// dirDestination is the production Destination adapter: a directory on disk
// holding committed snapshots and manifests plus their exclusively-created
// partials, named "<name>.sqlite"/"<name>.json" and
// ".<name>.sqlite.partial"/".<name>.json.partial".
type dirDestination struct {
	dir string
}

var _ Destination = (*dirDestination)(nil)

// NewDirDestination returns the production Destination adapter, writing
// into dir.
func NewDirDestination(dir string) Destination {
	return &dirDestination{dir: dir}
}

func (d *dirDestination) Prepare(ctx context.Context) error {
	if err := os.MkdirAll(d.dir, 0o700); err != nil {
		return fmt.Errorf("create snapshots directory %s: %w", d.dir, err)
	}
	return nil
}

func (d *dirDestination) Backup(ctx context.Context, src Source, name string) (string, error) {
	partial := d.partialPath(name, "sqlite")

	f, err := atomicfile.Create(partial, 0o600)
	if err != nil {
		return "", fmt.Errorf("create snapshot partial: %w", err)
	}
	_ = f.Close()

	if err := src.Backup(ctx, partial); err != nil {
		_ = os.Remove(partial)
		return "", fmt.Errorf("backup snapshot: %w", err)
	}
	return partial, nil
}

func (d *dirDestination) WriteManifest(ctx context.Context, name string, data []byte) (string, error) {
	partial := d.partialPath(name, "json")

	f, err := atomicfile.Create(partial, 0o600)
	if err != nil {
		return "", fmt.Errorf("create manifest partial: %w", err)
	}
	defer func() { _ = f.Close() }()

	if _, err := f.Write(data); err != nil {
		// unreachable: a write failing on a file this call just created needs disk exhaustion or an I/O fault; BR-11 assigns that fault's test to SCENARIO-13's fake Destination, not to this adapter.
		_ = os.Remove(partial)
		return "", fmt.Errorf("write manifest partial: %w", err)
	}
	return partial, nil
}

func (d *dirDestination) CommitSnapshot(ctx context.Context, partial string) (string, error) {
	final := d.finalPath(partial)
	if err := atomicfile.Commit(partial, final); err != nil {
		return "", fmt.Errorf("commit snapshot: %w", err)
	}
	return final, nil
}

func (d *dirDestination) CommitManifest(ctx context.Context, partial string) (string, error) {
	final := d.finalPath(partial)
	if err := atomicfile.Commit(partial, final); err != nil {
		return "", fmt.Errorf("commit manifest: %w", err)
	}
	return final, nil
}

// partialPath returns the exclusively-created partial's path for name and
// the final file's extension ("sqlite" or "json").
func (d *dirDestination) partialPath(name, ext string) string {
	return filepath.Join(d.dir, "."+name+"."+ext+".partial")
}

// finalPath derives a partial's committed name by stripping the leading
// "." and trailing ".partial" that partialPath added.
func (d *dirDestination) finalPath(partial string) string {
	base := strings.TrimSuffix(filepath.Base(partial), ".partial")
	base = strings.TrimPrefix(base, ".")
	return filepath.Join(filepath.Dir(partial), base)
}
