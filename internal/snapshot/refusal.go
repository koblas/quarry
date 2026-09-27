package snapshot

// RefusalError is a final, one-line refusal: its message excludes the
// "quarry: " prefix a caller adds before printing it to stderr.
type RefusalError struct {
	msg string
}

// Error returns the refusal's message verbatim.
func (e RefusalError) Error() string { return e.msg }
