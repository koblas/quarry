package report

import "time"

// Month is one calendar month a summary covers: Start is its first day and End its last, both UTC midnights.
type Month struct {
	Start, End time.Time
}

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
func (e MonthError) Error() string { return "" }

// ParseMonth resolves value, YYYY-MM, into the month it names; a nil value is the calendar month before
// now's, read in now's own zone. It returns a MonthError for a value that is not a month and for a month
// that has not ended.
func ParseMonth(*string, time.Time) (Month, error) { return Month{}, nil }
