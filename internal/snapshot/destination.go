package snapshot

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/koblas/quarry/internal/platform/atomicfile"
)

// leftoverMaxAge is how old a leftover partial must be before Prepare's sweep removes it.
const leftoverMaxAge = time.Hour

// leftoverPartialPattern matches a quarry partial name and its -journal/-wal/-shm companions.
var leftoverPartialPattern = regexp.MustCompile(`^\.\d{8}T\d{6}Z(_\d+)?\.(sqlite|json)\.partial(-journal|-wal|-shm)?$`)

// dirDestination is the production Destination adapter: a directory on disk
// holding committed snapshots and manifests plus their exclusively-created
// partials, named "<name>.sqlite"/"<name>.json" and
// ".<name>.sqlite.partial"/".<name>.json.partial".
type dirDestination struct {
	dir string
}

var _ Destination = (*dirDestination)(nil)

// newDirDestination returns the production Destination adapter, writing
// into dir.
func newDirDestination(dir string) Destination {
	return &dirDestination{dir: dir}
}

func (d *dirDestination) Prepare(ctx context.Context) error {
	if err := os.MkdirAll(d.dir, 0o700); err != nil {
		return fmt.Errorf("create snapshots directory %s: %w", d.dir, err)
	}
	d.sweepLeftovers()
	return nil
}

// sweepLeftovers best-effort removes leftoverPartialPattern matches older
// than leftoverMaxAge. A ReadDir or Remove failure is swallowed: the sweep
// never fails Prepare.
func (d *dirDestination) sweepLeftovers() {
	entries, err := os.ReadDir(d.dir)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-leftoverMaxAge)
	for _, entry := range entries {
		if entry.IsDir() || !leftoverPartialPattern.MatchString(entry.Name()) {
			continue
		}
		info, err := entry.Info()
		if err != nil || info.ModTime().After(cutoff) {
			continue
		}
		_ = os.Remove(filepath.Join(d.dir, entry.Name()))
	}
}

// Backup reserves the first candidate name ("name", then "name_2", "name_3",
// ...) whose sqlite and json finals do not already exist, backing up src
// into that candidate's partial.
func (d *dirDestination) Backup(ctx context.Context, src Source, name string) (string, string, error) {
	candidate := name
	for suffix := 1; ; suffix++ {
		partial := d.partialPath(candidate, "sqlite")

		f, err := atomicfile.Create(partial, 0o600)
		if err != nil {
			if errors.Is(err, fs.ErrExist) {
				candidate = fmt.Sprintf("%s_%d", name, suffix+1)
				continue
			}
			return "", "", fmt.Errorf("create snapshot partial: %w", err)
		}
		_ = f.Close()

		snapshotFinal, manifestFinal := d.FinalPaths(candidate)
		if fileExists(snapshotFinal) || fileExists(manifestFinal) {
			_ = os.Remove(partial)
			candidate = fmt.Sprintf("%s_%d", name, suffix+1)
			continue
		}

		if err := src.Backup(ctx, partial); err != nil {
			_ = os.Remove(partial)
			return "", "", fmt.Errorf("backup snapshot: %w", err)
		}
		return partial, candidate, nil
	}
}

// fileExists reports whether path can be stat'ed.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// WriteManifest writes data to name's partial and fsyncs it before closing,
// so a crash right after this call cannot leave a truncated manifest on
// disk once it is later committed.
func (d *dirDestination) WriteManifest(ctx context.Context, name string, data []byte) (string, error) {
	partial := d.partialPath(name, "json")

	f, err := atomicfile.Create(partial, 0o600)
	if err != nil {
		return "", fmt.Errorf("create manifest partial: %w", err)
	}

	if _, err := f.Write(data); err != nil {
		// unreachable: reachable via a real ENOSPC mid-write; this repo has no portable disk-full fixture to trigger it.
		_ = f.Close()
		_ = os.Remove(partial)
		return "", fmt.Errorf("write manifest partial: %w", err)
	}
	if err := f.Sync(); err != nil {
		// unreachable: reachable via a real ENOSPC on fsync; this repo has no portable disk-full fixture to trigger it.
		_ = f.Close()
		_ = os.Remove(partial)
		return "", fmt.Errorf("sync manifest partial: %w", err)
	}
	if err := f.Close(); err != nil {
		// unreachable: reachable via a delayed write-back failure on close; this repo has no portable fixture to trigger it.
		_ = os.Remove(partial)
		return "", fmt.Errorf("close manifest partial: %w", err)
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

func (d *dirDestination) FinalPaths(name string) (string, string) {
	return d.finalPath(d.partialPath(name, "sqlite")), d.finalPath(d.partialPath(name, "json"))
}

func (d *dirDestination) Discard(ctx context.Context, partial string) error {
	if err := os.Remove(partial); err != nil {
		return fmt.Errorf("discard snapshot partial: %w", err)
	}
	return nil
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
