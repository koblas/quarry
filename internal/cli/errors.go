package cli

// UsageError is a final, exit-2 error message: Execute returns it verbatim,
// never appending cobra's own hint to it.
type UsageError struct {
	msg string
}

// Error returns the message verbatim.
func (e UsageError) Error() string { return e.msg }
