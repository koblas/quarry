package lockfile

import (
	"context"
	"syscall"
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
func (e *Error) Error() string { return "" }

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
func (l *Locker) Acquire(_ context.Context) (release func(), err error) {
	return func() {}, nil
}
