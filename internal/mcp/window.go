package mcp

import (
	"errors"
	"strconv"

	"github.com/koblas/quarry/internal/report"
)

// windowRefusal is err, a window refusal, worded for the model: bounds are named since and until, and the
// stderr line is the class line, which never carries the caller's values. Any other error comes back as is.
func windowRefusal(err error) error {
	refusal, ok := errors.AsType[report.WindowError](err)
	if !ok {
		// unreachable: ParseWindow returns only what report's parseWindow and parseDateBound construct, and both construct only WindowError
		return err
	}
	return withLog(windowRefusedError(windowWording(refusal)), windowRefusedLog)
}

// windowRefusedError is the isError text of a refused window.
type windowRefusedError string

func (e windowRefusedError) Error() string { return string(e) }

// windowWording is the model's text for refusal; each kind's tail names the argument that fixes it.
func windowWording(refusal report.WindowError) string {
	bound := refusal.Bound
	switch refusal.Kind {
	case report.WindowNotADate:
		return bound + " " + strconv.Quote(refusal.Value) + " is not a date; use YYYY, YYYY-MM or YYYY-MM-DD"
	case report.WindowSinceAfterToday:
		return bound + " " + refusal.Value + " is after today; pass until to include future-dated transactions"
	case report.WindowChargeSinceAfterToday:
		return bound + " " + refusal.Value + " is after today; " + refusal.Command + " lists charges up to today only, so pass an earlier since"
	case report.WindowSinceAfterUntil:
		return bound + " " + refusal.Value + " is after until " + refusal.Other
	case report.WindowUntilBeforeDefault:
		return bound + " " + refusal.Value + " is before the default since " + refusal.DefaultSince + "; pass since too"
	}
	// unreachable: parseWindow and parseDateBound are the only WindowError constructors and each sets one of the kinds above
	return bound + " " + refusal.Value + " is refused"
}
