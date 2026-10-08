package replacefile

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/koblas/quarry/internal/platform/atomicfile"
)

// tempPattern names the temporary files Write creates beside its target.
const tempPattern = ".replace-*"

// tempFile is the part of *os.File that Write uses on its temporary file.
type tempFile interface {
	Name() string
	Chmod(mode fs.FileMode) error
	Write(p []byte) (int, error)
	Sync() error
	Close() error
}

// createTemp makes a new temporary file in dir.
type createTemp func(dir string) (tempFile, error)

// Write replaces the file at path with data and mode perm. It writes a
// temporary file in the same folder, syncs it and renames it over path, so a
// reader sees the old contents or the new, never a mix. On any failure it
// removes the temporary file and leaves path as it was; a folder-sync error
// after the rename is not a failure.
func Write(path string, data []byte, perm fs.FileMode) error {
	return write(path, data, perm, func(dir string) (tempFile, error) {
		return os.CreateTemp(dir, tempPattern)
	})
}

func write(path string, data []byte, perm fs.FileMode, create createTemp) (err error) {
	dir := filepath.Dir(path)
	tmp, err := create(dir)
	if err != nil {
		return fmt.Errorf("create a temporary file in %s: %w", dir, err)
	}
	name := tmp.Name()
	defer func() {
		if err != nil {
			_ = os.Remove(name)
		}
	}()

	err = fill(tmp, data, perm)
	closeErr := tmp.Close()
	if err == nil && closeErr != nil {
		err = fmt.Errorf("close %s: %w", name, closeErr)
	}
	if err != nil {
		return err
	}

	if err = os.Rename(name, path); err != nil {
		return fmt.Errorf("replace %s: %w", path, err)
	}
	atomicfile.SyncDir(dir)
	return nil
}

// fill sets tmp's mode explicitly, because the file starts at 0600 whatever perm is, then
// writes data and syncs it to disk.
func fill(tmp tempFile, data []byte, perm fs.FileMode) error {
	if err := tmp.Chmod(perm); err != nil {
		return fmt.Errorf("set the mode of %s: %w", tmp.Name(), err)
	}
	if _, err := tmp.Write(data); err != nil {
		return fmt.Errorf("write %s: %w", tmp.Name(), err)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("sync %s: %w", tmp.Name(), err)
	}
	return nil
}
