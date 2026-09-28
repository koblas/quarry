package atomicfile

import (
	"fmt"
	"io/fs"
	"os"
)

// Create opens a new file at path for exclusive writing. It returns an error
// wrapping fs.ErrExist when path already exists.
func Create(path string, perm fs.FileMode) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, perm)
	if err != nil {
		return nil, fmt.Errorf("create %s: %w", path, err)
	}
	return f, nil
}

// Commit moves partial into place at dest without overwriting an existing
// file. It returns an error wrapping fs.ErrExist when dest already exists,
// leaving partial in place. The move is a hard link followed by removing
// partial: the link is what makes dest exist, so the commit is done once it
// succeeds even if removing the now-redundant partial fails.
func Commit(partial, dest string) error {
	if err := os.Link(partial, dest); err != nil {
		return fmt.Errorf("commit %s: %w", dest, err)
	}
	_ = os.Remove(partial)
	return nil
}
