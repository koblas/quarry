package mcp

import (
	"errors"
	"fmt"

	"github.com/koblas/quarry/internal/platform/humanize"
	"github.com/koblas/quarry/internal/report"
)

// acbConfigShown is the config file in its ~ form, as the CLI's unclassified-accounts refusal names it.
const acbConfigShown = "~/Library/Application Support/quarry/config.toml"

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

// acbRefusal is err, a failed acb report call, worded for the model: unclassified accounts and a security that
// names none point to the tool call that lists them. Any other error comes back as is.
func acbRefusal(err error) error {
	refusal, ok := errors.AsType[report.RefusalError](err)
	if !ok {
		return err
	}
	switch refusal.Kind {
	case report.RefusalUnclassifiedAccounts:
		// The text carries nothing from the call, so stderr repeats it.
		return verbatim(namedRefusedError{text: unclassifiedAccountsText(refusal.Count), refusal: refusal})
	case report.RefusalUnknownSecurity:
		return namedRefusedError{
			text:    fmt.Sprintf("acb covers no security named %q; %s with no arguments lists every security it covers", refusal.Arg, toolACB),
			refusal: refusal,
		}
	case report.RefusalGeneric, report.RefusalUnknownAccount, report.RefusalAmbiguousAccount, report.RefusalStore, report.RefusalUnknownCategory:
	}

	return err
}

// unclassifiedAccountsText words acb's refusal while n investment accounts are in neither account list.
func unclassifiedAccountsText(n int) string {
	verb, pronoun := "are", "them"
	if n == 1 {
		verb, pronoun = "is", "it"
	}
	return fmt.Sprintf(
		"acb needs every brokerage and retirement account classified; %s %s in neither accounts.registered nor accounts.non-registered in %s; "+
			"%s with type unclassified-account and status all lists %s",
		humanize.Count(n, "account", "accounts"), verb, acbConfigShown, toolDataQuality, pronoun)
}

// yearRefusedError is the isError text of an acb year the model must fix.
type yearRefusedError string

func (e yearRefusedError) Error() string { return string(e) }

// acbYearRefusal is err, a refused acb year, worded for the model; its stderr line is the class line, which
// never carries the caller's value.
func acbYearRefusal(err error) error {
	refusal, ok := errors.AsType[report.ACBYearError](err)
	if ok && refusal.Kind == report.ACBYearAfterThisYear {
		return withLog(yearRefusedError("year "+refusal.Value+" is after this year; pass this year or an earlier one"), yearRefusedLog)
	}
	// unreachable: the schema bounds year to 1..9999 and "%04d" always yields four digits, so a year after this one is the only refusal
	return err
}
