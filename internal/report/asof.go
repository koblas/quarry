package report

import (
	"fmt"
	"time"
)

// AsOfErrorKind is the reason an as-of value was refused.
type AsOfErrorKind int

// The reasons an as-of value is refused.
const (
	// AsOfNotADate: Value names no year, month or day.
	AsOfNotADate AsOfErrorKind = iota
	// AsOfAfterToday: Value names a period that begins after today.
	AsOfAfterToday
)

// AsOfError is a refusal of an as-of value, carried as parts so each surface words it in its own
// vocabulary. Value is as typed. Error words it for the command line, without the "quarry: " prefix.
type AsOfError struct {
	Kind  AsOfErrorKind
	Value string
}

// Error is the command-line wording of the refusal.
func (e AsOfError) Error() string {
	if e.Kind == AsOfAfterToday {
		return fmt.Sprintf("--as-of %s is after today; holdings are valued up to today only, so pass an earlier --as-of", e.Value)
	}
	return fmt.Sprintf("--as-of %q is not a date; use YYYY, YYYY-MM or YYYY-MM-DD", e.Value)
}

// ResolveAsOf is the day holdings are valued on: today in now's own zone when value is nil, else ParseAsOf of
// *value. A value given as "" is given, and is not a date.
func ResolveAsOf(value *string, now time.Time) (time.Time, error) {
	if value == nil {
		return Today(now), nil
	}
	return ParseAsOf(*value, now)
}

// ParseAsOf resolves value, a year, month or day, into the day holdings are valued on: the last day
// of the period, or today when the period contains today. It returns an AsOfError for a value that
// names no date and for a period that begins after today, read in now's own zone.
func ParseAsOf(value string, now time.Time) (time.Time, error) {
	first, last, ok := datePeriod(value)
	if !ok {
		return time.Time{}, AsOfError{Kind: AsOfNotADate, Value: value}
	}
	today := Today(now)
	if first.After(today) {
		return time.Time{}, AsOfError{Kind: AsOfAfterToday, Value: value}
	}
	if last.After(today) {
		return today, nil
	}
	return last, nil
}
