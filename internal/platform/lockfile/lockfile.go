package lockfile

import (
	"context"
	"errors"
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
	// KindNotRegular means the lock path exists but is not a regular file (directory, symlink, fifo).
	KindNotRegular
	// KindFolderCreate means ModeSync could not create the folder.
	KindFolderCreate
	// KindCreate means the lock file was absent and could not be created.
	KindCreate
	// KindOpen means the lock file exists (or could not be looked up) and could not be opened.
	KindOpen
	// KindLock means flock failed for a reason other than another holder.
	KindLock
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
	case KindNotRegular:
		return "lock " + e.Path + " is not a regular file"
	case KindFolderCreate:
		return "folder of lock " + e.Path + " cannot be created"
	case KindCreate:
		return "lock " + e.Path + " cannot be created"
	case KindOpen:
		return "lock " + e.Path + " cannot be opened"
	case KindLock:
		return "lock " + e.Path + " cannot be taken"
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

// Acquire takes the lock without waiting and returns its idempotent release func.
// Every failure is a *Error: KindHeld while another open holds the lock,
// KindFolderMissing in ModePrune when the folder is absent, KindNotRegular for a
// path that is not a regular file, and KindFolderCreate, KindCreate, KindOpen or
// KindLock for the OS step that failed.
func (l *Locker) Acquire(_ context.Context) (func(), error) {
	if err := l.ensureFolder(); err != nil {
		return nil, err
	}
	existed, err := l.checkRegular()
	if err != nil {
		return nil, err
	}
	f, err := os.OpenFile(l.path, openFlags, filePerm)
	if err != nil {
		kind := KindCreate
		if existed {
			kind = KindOpen
		}
		return nil, &Error{Kind: kind, Path: l.path, Err: err}
	}
	if err := l.lock(f); err != nil {
		_ = f.Close()
		return nil, err
	}
	var once sync.Once
	// The closure keeps f referenced: a garbage-collected os.File closes its fd and drops the lock.
	return func() { once.Do(func() { _ = f.Close() }) }, nil
}

// ensureFolder creates the lock file's folder in ModeSync; in ModePrune it only checks it is not missing.
func (l *Locker) ensureFolder() error {
	dir := filepath.Dir(l.path)
	if l.mode == ModeSync {
		if err := os.MkdirAll(dir, folderPerm); err != nil {
			return &Error{Kind: KindFolderCreate, Path: l.path, Err: err}
		}
		return nil
	}
	// Any other Stat fault is left for checkRegular's Lstat to classify.
	if _, err := os.Stat(dir); errors.Is(err, fs.ErrNotExist) {
		return &Error{Kind: KindFolderMissing, Path: l.path, Err: err}
	}
	return nil
}

// checkRegular reports whether the lock file exists and refuses a path that is not a
// regular file. Lstat means a symlink is refused, never followed.
func (l *Locker) checkRegular() (bool, error) {
	info, err := os.Lstat(l.path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, &Error{Kind: KindOpen, Path: l.path, Err: err}
	}
	if !info.Mode().IsRegular() {
		return true, &Error{Kind: KindNotRegular, Path: l.path}
	}
	return true, nil
}

// lock takes the exclusive non-blocking flock on f, classifying a would-block as KindHeld.
func (l *Locker) lock(f *os.File) error {
	flockErr := l.flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if errors.Is(flockErr, syscall.EWOULDBLOCK) {
		return &Error{Kind: KindHeld, Path: l.path, Err: flockErr}
	}
	if flockErr != nil {
		return &Error{Kind: KindLock, Path: l.path, Err: flockErr}
	}
	return nil
}
