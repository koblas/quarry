package report

import (
	"fmt"
	"time"

	"github.com/koblas/quarry/internal/store"
)

// Month is one calendar month a summary covers: Start is its first day and End its last, both UTC midnights.
type Month struct {
	Start, End time.Time
}

// Window is the month as the window of days it covers.
func (m Month) Window() store.Window { return store.Window{Since: m.Start, Until: m.End} }

// String is the month as YYYY-MM, the form --month takes.
func (m Month) String() string { return m.Start.Format(monthLayout) }

// Name is the month as English month name and year, such as "September 2026".
func (m Month) Name() string { return m.Start.Format("January 2006") }

// MonthErrorKind is the reason a --month value was refused.
type MonthErrorKind int

// The reasons a month is refused.
const (
	// MonthNotAMonth: Value is not exactly YYYY-MM naming a month from year 0001 on.
	MonthNotAMonth MonthErrorKind = iota
	// MonthNotEnded: Value names a month that has not ended.
	MonthNotEnded
)

// MonthError is a refusal of a month, carried as parts so each surface words it in its own vocabulary.
// Example is the default month, YYYY-MM, the wording suggests. Error words it for the command line, without
// the "quarry: " prefix a caller adds.
type MonthError struct {
	Kind    MonthErrorKind
	Value   string
	Example string
}

// Error is the command-line wording of the refusal.
func (e MonthError) Error() string {
	if e.Kind == MonthNotEnded {
		return fmt.Sprintf("--month %s has not ended; summary covers whole months, so pass %s or earlier", e.Value, e.Example)
	}
	return fmt.Sprintf("--month %q is not a month; use YYYY-MM, such as %s", e.Value, e.Example)
}

// monthLayout is the YYYY-MM form of a month.
const monthLayout = "2006-01"

// ParseMonth resolves value, YYYY-MM, into the month it names; a nil value is the calendar month before
// now's, read in now's own zone. It returns a MonthError for a value that is not a month and for a month
// that has not ended.
func ParseMonth(value *string, now time.Time) (Month, error) {
	today := Today(now)
	last := monthOf(time.Date(today.Year(), today.Month()-1, 1, 0, 0, 0, 0, time.UTC))
	if value == nil {
		return last, nil
	}
	start, err := time.Parse(monthLayout, *value)
	if err != nil || start.Year() < 1 {
		return Month{}, MonthError{Kind: MonthNotAMonth, Value: *value, Example: last.String()}
	}
	month := monthOf(start)
	if !month.End.Before(today) {
		return Month{}, MonthError{Kind: MonthNotEnded, Value: *value, Example: last.String()}
	}
	return month, nil
}

// monthOf is the month whose first day is start.
func monthOf(start time.Time) Month { return Month{Start: start, End: monthEnd(start)} }
