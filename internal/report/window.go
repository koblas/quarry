package report

import (
	"fmt"
	"time"

	"github.com/koblas/quarry/internal/store"
)

// WindowError is a refusal of a --since/--until pair: its message excludes the
// "quarry: " prefix a caller adds before printing it to stderr.
type WindowError struct {
	msg string
}

// Error returns the refusal's message verbatim.
func (e WindowError) Error() string { return e.msg }

// dateForms are the accepted date layouts, each with the calendar step from
// the first day it names to the last.
var dateForms = []struct {
	layout string
	years  int
	months int
	days   int
}{
	{layout: "2006", years: 1, days: -1},
	{layout: "2006-01", months: 1, days: -1},
	{layout: layoutDay},
}

const layoutDay = "2006-01-02"

// ParseWindow resolves the --since and --until values into a window; a nil
// pointer is a flag not given and takes its default from now (see
// DefaultWindow). A bare year or month covers all of it. It returns a
// WindowError for a value that is not a date, for a since after until, for a
// since after today when no until is given, and for an until before the
// default since when no since is given.
func ParseWindow(since, until *string, now time.Time) (store.Window, error) {
	window := DefaultWindow(now)
	today := window.Until
	defaultSince := window.Since

	if since != nil {
		first, _, err := parseDateBound("--since", *since)
		if err != nil {
			return store.Window{}, err
		}
		window.Since = first
	}
	if until != nil {
		// An until covers through the last day of the period it names.
		_, last, err := parseDateBound("--until", *until)
		if err != nil {
			return store.Window{}, err
		}
		window.Until = last
	}

	switch {
	case since != nil && until == nil && window.Since.After(today):
		return store.Window{}, WindowError{msg: fmt.Sprintf(
			"--since %s is after today; pass --until to include future-dated transactions", *since)}
	case since != nil && until != nil && window.Since.After(window.Until):
		return store.Window{}, WindowError{msg: fmt.Sprintf("--since %s is after --until %s", *since, *until)}
	case since == nil && until != nil && window.Until.Before(defaultSince):
		return store.Window{}, WindowError{msg: fmt.Sprintf(
			"--until %s is before the default --since %s; pass --since too",
			*until, defaultSince.Format(layoutDay))}
	}
	return window, nil
}

// parseDateBound is the first and last day of the year, month or day that
// value names, or a WindowError naming flag when it names none.
func parseDateBound(flag, value string) (time.Time, time.Time, error) {
	for _, form := range dateForms {
		if first, err := time.Parse(form.layout, value); err == nil {
			return first, first.AddDate(form.years, form.months, form.days), nil
		}
	}
	return time.Time{}, time.Time{}, WindowError{msg: fmt.Sprintf(
		"%s %q is not a date; use YYYY, YYYY-MM or YYYY-MM-DD", flag, value)}
}
