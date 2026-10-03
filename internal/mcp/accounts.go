package mcp

import (
	"errors"
	"fmt"

	"github.com/koblas/quarry/internal/report"
)

// namedRefusedError is the isError text of an account or category the model must fix; it unwraps to the refusal that logLine classifies.
type namedRefusedError struct {
	text    string
	refusal report.RefusalError
}

func (e namedRefusedError) Error() string { return e.text }

func (e namedRefusedError) Unwrap() error { return e.refusal }

// accountRefusal is err, a failed report call with accounts, worded for the model: an account that names none
// points to describe_schema. Any other error, an ambiguous account's included, comes back as is.
func accountRefusal(err error) error {
	refusal, ok := errors.AsType[report.RefusalError](err)
	if !ok || refusal.Kind != report.RefusalUnknownAccount {
		return err
	}
	return namedRefusedError{
		text:    fmt.Sprintf("no account named %q; call %s to list the accounts", refusal.Arg, toolDescribe),
		refusal: refusal,
	}
}

// categoryRefusal is err, a failed search with a category, worded for the model: a category that names none
// points to describe_schema. Any other error comes back as is.
func categoryRefusal(err error) error {
	refusal, ok := errors.AsType[report.RefusalError](err)
	if !ok || refusal.Kind != report.RefusalUnknownCategory {
		return err
	}
	return namedRefusedError{
		text:    fmt.Sprintf("no category named %q; call %s to list the categories", refusal.Arg, toolDescribe),
		refusal: refusal,
	}
}
