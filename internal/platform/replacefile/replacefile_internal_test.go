// White-box: the temp-file faults (chmod, write, sync, close) cannot be
// provoked through Write's public surface, so these rows drive the unexported
// write with a temp-file seam that fails one call.
package replacefile

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errInjected = errors.New("injected fault")

// faultyTemp is a real temp file whose chosen call fails.
type faultyTemp struct {
	*os.File

	failChmod, failWrite, failSync, failClose bool
}

func (f *faultyTemp) Chmod(m os.FileMode) error {
	if f.failChmod {
		return &os.PathError{Op: "chmod", Path: f.Name(), Err: errInjected}
	}
	return f.File.Chmod(m)
}

func (f *faultyTemp) Write(p []byte) (int, error) {
	if f.failWrite {
		return 0, &os.PathError{Op: "write", Path: f.Name(), Err: errInjected}
	}
	return f.File.Write(p)
}

func (f *faultyTemp) Sync() error {
	if f.failSync {
		return &os.PathError{Op: "sync", Path: f.Name(), Err: errInjected}
	}
	return f.File.Sync()
}

func (f *faultyTemp) Close() error {
	err := f.File.Close()
	if f.failClose {
		return &os.PathError{Op: "close", Path: f.Name(), Err: errInjected}
	}
	return err
}

func Test_write_leaves_the_old_file_and_no_temp_when_a_temp_file_call_fails(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		fault faultyTemp
	}{
		{name: "chmod fails", fault: faultyTemp{failChmod: true}},
		{name: "write fails", fault: faultyTemp{failWrite: true}},
		{name: "sync fails", fault: faultyTemp{failSync: true}},
		{name: "close fails", fault: faultyTemp{failClose: true}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			path := filepath.Join(dir, "target")
			require.NoError(t, os.WriteFile(path, []byte("old"), 0o600))
			create := func(d string) (tempFile, error) {
				f, err := os.CreateTemp(d, ".replace-*")
				if err != nil {
					return nil, err
				}
				fault := c.fault
				fault.File = f
				return &fault, nil
			}

			err := write(path, []byte("new"), 0o600, create)

			require.ErrorIs(t, err, errInjected)
			got, readErr := os.ReadFile(path)
			require.NoError(t, readErr)
			assert.Equal(t, "old", string(got))
			entries, listErr := os.ReadDir(dir)
			require.NoError(t, listErr)
			require.Len(t, entries, 1)
			assert.Equal(t, "target", entries[0].Name())
		})
	}
}
