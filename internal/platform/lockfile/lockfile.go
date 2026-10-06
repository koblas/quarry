package lockfile

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"syscall"
)

const (
	folderPerm = 0o700
	filePerm   = 0o600
	// openFlags never follows a symlink and never blocks opening a fifo.
	openFlags = os.O_RDONLY | os.O_CREATE | syscall.O_NOFOLLOW | syscall.O_NONBLOCK
)

// Mode says whether Acquire may create the lock file's folder.
type Mode int

const (
	// ModeSync creates the folder (0700) when it is missing.
	ModeSync Mode = iota
	// ModePrune never creates the folder; a missing one is a KindFolderMissing error.
	ModePrune
)

// Kind classifies an Error.
type Kind int

const (
	// KindHeld means another open file description holds the lock.
	KindHeld Kind = iota + 1
	// KindFolderMissing means the folder is absent and the mode forbids creating it.
	KindFolderMissing
)

// Error is a classified lock failure Acquire returns.
type Error struct {
	Kind Kind
	Path string
	Err  error
}

// Error returns the failure's message, naming Path.
func (e *Error) Error() string {
	switch e.Kind {
	case KindHeld:
		return "lock " + e.Path + " is held by another process"
	case KindFolderMissing:
		return "folder of lock " + e.Path + " does not exist"
	}
	return "lock " + e.Path + " failed"
}

// Unwrap returns the underlying cause, if any.
func (e *Error) Unwrap() error { return e.Err }

// Option configures a Locker built by New.
type Option func(*Locker)

// WithFlock replaces the flock call; tests use it to inject a failing lock.
func WithFlock(flock func(fd, how int) error) Option {
	return func(l *Locker) { l.flock = flock }
}

// Locker takes the lock at one path in one Mode.
type Locker struct {
	path  string
	mode  Mode
	flock func(fd, how int) error
}

// New returns a Locker for the lock file at path.
func New(path string, mode Mode, opts ...Option) *Locker {
	l := &Locker{path: path, mode: mode, flock: syscall.Flock}
	for _, opt := range opts {
		opt(l)
	}
	return l
}

// Acquire takes the lock without waiting and returns the func that releases it.
// It returns a *Error of KindHeld while another open holds the lock, and of
// KindFolderMissing in ModePrune when the folder is absent; any other failure
// is returned wrapped with the path. The lock lasts until release runs, which
// is idempotent, or the process exits.
func (l *Locker) Acquire(_ context.Context) (func(), error) {
	if err := l.ensureFolder(); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(l.path, openFlags, filePerm)
	if err != nil {
		return nil, fmt.Errorf("open lock file %s: %w", l.path, err)
	}
	if err := l.lock(f); err != nil {
		_ = f.Close()
		return nil, err
	}
	var once sync.Once
	// The closure keeps f referenced: a garbage-collected os.File closes its fd and drops the lock.
	return func() { once.Do(func() { _ = f.Close() }) }, nil
}

// ensureFolder creates the lock file's folder in ModeSync; in ModePrune it only checks it exists.
func (l *Locker) ensureFolder() error {
	dir := filepath.Dir(l.path)
	if l.mode == ModeSync {
		if err := os.MkdirAll(dir, folderPerm); err != nil {
			return fmt.Errorf("create folder %s: %w", dir, err)
		}
		return nil
	}
	_, err := os.Stat(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return &Error{Kind: KindFolderMissing, Path: l.path, Err: err}
	}
	if err != nil {
		return fmt.Errorf("check folder %s: %w", dir, err)
	}
	return nil
}

// lock takes the exclusive non-blocking flock on f, classifying a would-block as KindHeld.
func (l *Locker) lock(f *os.File) error {
	flockErr := l.flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if errors.Is(flockErr, syscall.EWOULDBLOCK) {
		return &Error{Kind: KindHeld, Path: l.path, Err: flockErr}
	}
	if flockErr != nil {
		return fmt.Errorf("lock %s: %w", l.path, flockErr)
	}
	return nil
}
