package main

import (
	"io"
	"os"

	"golang.org/x/term"
)

// isTerminal reports whether r is an interactive terminal: an *os.File whose
// descriptor answers the termios query. /dev/null is a character device but not a terminal.
func isTerminal(r io.Reader) bool {
	f, ok := r.(*os.File)
	return ok && term.IsTerminal(int(f.Fd()))
}
