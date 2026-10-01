package config

import (
	"errors"
	"strings"
)

// fixIt ends every refusal: the file is the interface, so the fix is an edit.
const fixIt = "; fix the file and run the command again"

// Problem is err's text without the closing instruction to fix the file, for a caller that words
// its own next step; an error that does not end with it is returned as text unchanged.
func Problem(err error) string {
	if refusal, ok := errors.AsType[*refusalError](err); ok {
		return strings.TrimSuffix(refusal.msg, fixIt)
	}
	return strings.TrimSuffix(err.Error(), fixIt)
}

// refusalError is a config refusal: its text is the complete line for the
// user, without the "quarry: " lead the command adds.
type refusalError struct {
	msg, absolute string
	cause         error
}

// Error returns the refusal text.
func (e *refusalError) Error() string { return e.msg }

// Unwrap returns the read error behind the refusal, nil for a bad value.
func (e *refusalError) Unwrap() error { return e.cause }

// refuse is a refusalError saying reason, then how to fix it. reason words the refusal around the
// file's path, which it is given ~-abbreviated for the message and absolute for ProblemAbsolute.
func (f file) refuse(reason func(path string) string, cause error) *refusalError {
	return &refusalError{msg: reason(f.shown) + fixIt, absolute: reason(f.path) + fixIt, cause: cause}
}

// cannotRead is the refusal for a file that cannot be read or parsed: detail says why.
func (f file) cannotRead(detail string, cause error) *refusalError {
	return f.refuse(func(path string) string { return "cannot read " + path + ": " + detail }, cause)
}

// badValue is the refusal for a setting whose value is wrong: the file's
// path, what the setting must be, and the value as the file wrote it.
func (f file) badValue(must, got string) *refusalError {
	return f.refuse(func(path string) string { return path + ": " + must + ", got " + got }, nil)
}

// ProblemAbsolute is Problem with the config file named by its absolute path, not ~-abbreviated;
// machine-readable output carries this form. An error that is not a config refusal is returned as Problem does.
func ProblemAbsolute(err error) string {
	if refusal, ok := errors.AsType[*refusalError](err); ok {
		return strings.TrimSuffix(refusal.absolute, fixIt)
	}
	return Problem(err)
}
