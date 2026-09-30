package config

import "strings"

// fixIt ends every refusal: the file is the interface, so the fix is an edit.
const fixIt = "; fix the file and run the command again"

// Problem is err's text without the closing instruction to fix the file, for a caller that words
// its own next step; an error that does not end with it is returned as text unchanged.
func Problem(err error) string { return strings.TrimSuffix(err.Error(), fixIt) }

// refusalError is a config refusal: its text is the complete line for the
// user, without the "quarry: " lead the command adds.
type refusalError struct {
	msg   string
	cause error
}

// Error returns the refusal text.
func (e *refusalError) Error() string { return e.msg }

// Unwrap returns the read error behind the refusal, nil for a bad value.
func (e *refusalError) Unwrap() error { return e.cause }

// refuse is a refusalError saying reason, then how to fix it.
func (f file) refuse(reason string, cause error) *refusalError {
	return &refusalError{msg: reason + fixIt, cause: cause}
}

// badValue is the refusal for a setting whose value is wrong: the file's
// path, what the setting must be, and the value as the file wrote it.
func (f file) badValue(must, got string) *refusalError {
	return f.refuse(f.shown+": "+must+", got "+got, nil)
}
