package mcp

import (
	"errors"
	"fmt"

	"github.com/koblas/quarry/internal/report"
)

// accountRefusedError is the isError text of an account the model must fix; it unwraps to the refusal that logLine classifies.
type accountRefusedError struct {
	text    string
	refusal report.RefusalError
}

func (e accountRefusedError) Error() string { return e.text }

func (e accountRefusedError) Unwrap() error { return e.refusal }

// accountRefusal is err, a failed report call with accounts, worded for the model: an account that names none
// points to describe_schema. Any other error, an ambiguous account's included, comes back as is.
func accountRefusal(err error) error {
	refusal, ok := errors.AsType[report.RefusalError](err)
	if !ok || refusal.Kind != report.RefusalUnknownAccount {
		return err
	}
	return accountRefusedError{
		text:    fmt.Sprintf("no account named %q; call %s to list the accounts", refusal.Arg, toolDescribe),
		refusal: refusal,
	}
}
