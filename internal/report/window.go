package report

import (
	"fmt"
	"time"

	"github.com/koblas/quarry/internal/store"
)

// WindowErrorKind is the reason a since/until pair was refused.
type WindowErrorKind int

// The reasons a window is refused.
const (
	// WindowNotADate: Value, the Bound's argument, names no year, month or day.
	WindowNotADate WindowErrorKind = iota
	// WindowSinceAfterToday: Value, a future since, was given without an until.
	WindowSinceAfterToday
	// WindowChargeSinceAfterToday is WindowSinceAfterToday for Command, which lists charges up to today only.
	WindowChargeSinceAfterToday
	// WindowSinceAfterUntil: Value, the since, is after Other, the until.
	WindowSinceAfterUntil
	// WindowUntilBeforeDefault: Value, the until, is before DefaultSince, the since taken when none was given.
	WindowUntilBeforeDefault
)

// WindowError is a refusal of a since/until pair, carried as parts so each surface words it in its own
// vocabulary. Bound is "since" or "until"; DefaultSince is a YYYY-MM-DD date. Error words it for the
// command line, with "--" before each bound and without the "quarry: " prefix a caller adds.
type WindowError struct {
	Kind         WindowErrorKind
	Bound        string
	Value        string
	Other        string
	DefaultSince string
	Command      string
}

// Error is the command-line wording of the refusal.
func (e WindowError) Error() string {
	flag := "--" + e.Bound
	switch e.Kind {
	case WindowNotADate:
		return fmt.Sprintf("%s %q is not a date; use YYYY, YYYY-MM or YYYY-MM-DD", flag, e.Value)
	case WindowSinceAfterToday:
		return fmt.Sprintf("%s %s is after today; pass --until to include future-dated transactions", flag, e.Value)
	case WindowChargeSinceAfterToday:
		return fmt.Sprintf("%s %s is after today; %s lists charges up to today only, so pass an earlier %s", flag, e.Value, e.Command, flag)
	case WindowSinceAfterUntil:
		return fmt.Sprintf("%s %s is after --%s %s", flag, e.Value, boundUntil, e.Other)
	case WindowUntilBeforeDefault:
		return fmt.Sprintf("%s %s is before the default --%s %s; pass --%s too", flag, e.Value, boundSince, e.DefaultSince, boundSince)
	}
	// unreachable: parseWindow and parseDateBound are the only WindowError constructors and each sets one of the kinds above
	return flag + " " + e.Value + " is refused"
}

// The bounds a WindowError names.
const (
	boundSince = "since"
	boundUntil = "until"
)

// dateForms are the accepted layouts, each with the step from its first day to its last.
var dateForms = []struct {
	layout string
	years  int
	months int
	days   int
}{
	{layout: "2006", years: 1, days: -1},
	{layout: "2006-01", months: 1, days: -1},
	{layout: time.DateOnly},
}

// DefaultWindow is January 1 of now's year through now's day, both read in
// now's own zone.
func DefaultWindow(now time.Time) store.Window {
	return store.Window{
		Since: time.Date(now.Year(), time.January, 1, 0, 0, 0, 0, time.UTC),
		Until: Today(now),
	}
}

// Today is now's calendar day, read in now's own zone, as UTC midnight.
func Today(now time.Time) time.Time {
	year, month, day := now.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

// ParseWindow resolves since and until into a window; a nil pointer is an argument not
// given and takes its default from now (see DefaultWindow). A bare year or month covers
// all of it. It returns a WindowError for a value that is not a date and for a period
// that is empty, or a future one when no until allows it.
func ParseWindow(since, until *string, now time.Time) (store.Window, error) {
	return parseWindow(since, until, now, WindowSinceAfterToday, "")
}

// ParseChargeWindow is ParseWindow for a command that lists charges up to today only: a
// future since given without an until is refused as WindowChargeSinceAfterToday, which carries command.
func ParseChargeWindow(command string, since, until *string, now time.Time) (store.Window, error) {
	return parseWindow(since, until, now, WindowChargeSinceAfterToday, command)
}

// parseWindow is ParseWindow with the refusal for a future since given by futureSince and command.
func parseWindow(since, until *string, now time.Time, futureSince WindowErrorKind, command string) (store.Window, error) {
	window := DefaultWindow(now)
	today := window.Until
	defaultSince := window.Since

	if since != nil {
		first, _, err := parseDateBound(boundSince, *since)
		if err != nil {
			return store.Window{}, err
		}
		window.Since = first
	}
	if until != nil {
		// An until covers through the last day of the period it names.
		_, last, err := parseDateBound(boundUntil, *until)
		if err != nil {
			return store.Window{}, err
		}
		window.Until = last
	}

	switch {
	case since != nil && until == nil && window.Since.After(today):
		return store.Window{}, WindowError{Kind: futureSince, Bound: boundSince, Value: *since, Command: command}
	case since != nil && until != nil && window.Since.After(window.Until):
		return store.Window{}, WindowError{Kind: WindowSinceAfterUntil, Bound: boundSince, Value: *since, Other: *until}
	case since == nil && until != nil && window.Until.Before(defaultSince):
		return store.Window{}, WindowError{
			Kind: WindowUntilBeforeDefault, Bound: boundUntil, Value: *until, DefaultSince: defaultSince.Format(time.DateOnly),
		}
	}
	return window, nil
}

// parseDateBound is the first and last day of the year, month or day that
// value names, or a WindowNotADate error for bound when it names none.
func parseDateBound(bound, value string) (time.Time, time.Time, error) {
	first, last, ok := datePeriod(value)
	if !ok {
		return time.Time{}, time.Time{}, WindowError{Kind: WindowNotADate, Bound: bound, Value: value}
	}
	return first, last, nil
}

// datePeriod is the first and last day of the year, month or day that value names; ok is false when it names none.
func datePeriod(value string) (time.Time, time.Time, bool) {
	for _, form := range dateForms {
		if first, err := time.Parse(form.layout, value); err == nil {
			return first, first.AddDate(form.years, form.months, form.days), true
		}
	}
	return time.Time{}, time.Time{}, false
}

// ParseSearchWindow resolves since and until into a search window; a nil pointer is an open bound,
// so no clock is read. A bare year or month covers all of it. It returns a WindowError for a value
// that is not a date and for a since after the until.
func ParseSearchWindow(since, until *string) (store.SearchWindow, error) {
	var window store.SearchWindow
	if since != nil {
		first, _, err := parseDateBound(boundSince, *since)
		if err != nil {
			return store.SearchWindow{}, err
		}
		window.Since = &first
	}
	if until != nil {
		_, last, err := parseDateBound(boundUntil, *until)
		if err != nil {
			return store.SearchWindow{}, err
		}
		window.Until = &last
	}
	if window.Since != nil && window.Until != nil && window.Since.After(*window.Until) {
		return store.SearchWindow{}, WindowError{Kind: WindowSinceAfterUntil, Bound: boundSince, Value: *since, Other: *until}
	}
	return window, nil
}
