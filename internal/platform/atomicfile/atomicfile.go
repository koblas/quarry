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
// partial, which is atomic and fails without side effects when dest exists.
func Commit(partial, dest string) error {
	if err := os.Link(partial, dest); err != nil {
		return fmt.Errorf("commit %s: %w", dest, err)
	}
	if err := os.Remove(partial); err != nil {
		return fmt.Errorf("commit %s: remove partial: %w", dest, err)
	}
	return nil
}
